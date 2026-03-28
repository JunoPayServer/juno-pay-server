#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  recover-native-staging.sh [options]

Options:
  --root <path>          Deployment root. Default: /opt/juno-pay
  --backup-dir <path>    SQLite backup directory. Default: <root>/backups/scan-reset
  --wait-seconds <sec>   Per-service startup wait budget. Default: 300
EOF
}

ROOT="/opt/juno-pay"
BACKUP_DIR=""
WAIT_SECONDS=300

while [[ $# -gt 0 ]]; do
  case "$1" in
    --root)
      ROOT="${2:-}"
      shift 2
      ;;
    --backup-dir)
      BACKUP_DIR="${2:-}"
      shift 2
      ;;
    --wait-seconds)
      WAIT_SECONDS="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

COMPOSE_PATH="${ROOT}/docker-compose.yml"
RUNTIME_ENV_PATH="${ROOT}/runtime.env"
DATA_DIR="${ROOT}/data"
JUNOCASHD_DIR="${DATA_DIR}/junocashd"
SCAN_DIR="${DATA_DIR}/juno-scan"
SCAN_DB_PATH="${SCAN_DIR}/db"
PAYSERVER_DIR="${DATA_DIR}/juno-pay-server"
STATE_DB_PATH="${PAYSERVER_DIR}/state.sqlite"
BACKUP_DIR="${BACKUP_DIR:-${ROOT}/backups/scan-reset}"

if [[ ! -f "$COMPOSE_PATH" ]]; then
  echo "compose file not found: $COMPOSE_PATH" >&2
  exit 1
fi

if [[ ! -f "$RUNTIME_ENV_PATH" ]]; then
  echo "runtime env file not found: $RUNTIME_ENV_PATH" >&2
  exit 1
fi

compose() {
  docker compose --env-file "$RUNTIME_ENV_PATH" -f "$COMPOSE_PATH" "$@"
}

service_container_id() {
  compose ps -q "$1"
}

wait_service_ready() {
  local service cid status deadline
  service="$1"
  deadline=$((SECONDS + WAIT_SECONDS))

  while (( SECONDS < deadline )); do
    cid="$(service_container_id "$service" || true)"
    if [[ -n "$cid" ]]; then
      status="$(
        docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$cid" 2>/dev/null || true
      )"
      case "$status" in
        healthy|running)
          return 0
          ;;
      esac
    fi
    sleep 2
  done

  echo "service did not become ready within ${WAIT_SECONDS}s: $service" >&2
  if [[ -n "${cid:-}" ]]; then
    docker logs --tail 80 "$cid" 2>&1 || true
  fi
  exit 1
}

wait_scanner_tip_ready() {
  local cid deadline raw
  deadline=$((SECONDS + WAIT_SECONDS))

  while (( SECONDS < deadline )); do
    cid="$(service_container_id juno-scan || true)"
    if [[ -n "$cid" ]]; then
      raw="$(docker exec "$cid" curl -fsS http://127.0.0.1:8080/v1/health 2>/dev/null || true)"
      if [[ -n "$raw" ]] && python3 - "$raw" <<'PY'
import json
import sys

doc = json.loads(sys.argv[1])
if doc.get("scanned_height") is None:
    raise SystemExit(1)
PY
      then
        return 0
      fi
    fi
    sleep 2
  done

  echo "juno-scan did not report scanned_height within ${WAIT_SECONDS}s" >&2
  if [[ -n "${cid:-}" ]]; then
    docker logs --tail 80 "$cid" 2>&1 || true
  fi
  exit 1
}

sqlite_integrity_ok() {
  if [[ ! -f "$STATE_DB_PATH" ]]; then
    return 1
  fi

  python3 - "$STATE_DB_PATH" <<'PY'
import sqlite3
import sys

db_path = sys.argv[1]
conn = sqlite3.connect(db_path)
try:
    result = conn.execute("PRAGMA integrity_check").fetchone()
finally:
    conn.close()

if not result or result[0] != "ok":
    raise SystemExit(1)
PY
}

restore_latest_backup() {
  local latest_backup timestamp broken_copy

  latest_backup="$(
    BACKUP_DIR="$BACKUP_DIR" python3 - <<'PY'
import glob
import os

pattern = os.path.join(os.environ["BACKUP_DIR"], "state.sqlite.*.bak")
matches = sorted(glob.glob(pattern))
print(matches[-1] if matches else "")
PY
  )"

  if [[ -z "$latest_backup" ]]; then
    echo "state sqlite failed integrity check and no backup was found in $BACKUP_DIR" >&2
    exit 1
  fi

  mkdir -p "$PAYSERVER_DIR" "$BACKUP_DIR"
  if [[ -f "$STATE_DB_PATH" ]]; then
    timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
    broken_copy="${BACKUP_DIR}/state.sqlite.invalid.${timestamp}.bak"
    cp "$STATE_DB_PATH" "$broken_copy"
    echo "saved_invalid_state=$broken_copy"
  fi

  cp "$latest_backup" "$STATE_DB_PATH"
  echo "restored_state_backup=$latest_backup"

  if ! sqlite_integrity_ok; then
    echo "restored backup is not a valid sqlite database: $latest_backup" >&2
    exit 1
  fi
}

backup_state_db() {
  local timestamp backup_path

  mkdir -p "$BACKUP_DIR"
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  backup_path="${BACKUP_DIR}/state.sqlite.${timestamp}.bak"
  cp "$STATE_DB_PATH" "$backup_path"
  echo "backup_path=$backup_path"
}

fix_payserver_ownership() {
  chown -R 10001:65534 "$PAYSERVER_DIR" || true
}

reset_scan_cursors() {
  python3 - "$STATE_DB_PATH" <<'PY'
import sqlite3
import sys

db_path = sys.argv[1]
conn = sqlite3.connect(db_path)
cur = conn.cursor()
cur.execute("DELETE FROM scan_cursors")
deleted = cur.rowcount
conn.commit()
remaining = cur.execute("SELECT COUNT(*) FROM scan_cursors").fetchone()[0]
print(f"deleted_scan_cursors={deleted}")
print(f"remaining_scan_cursors={remaining}")
PY
}

register_wallets_and_backfill() {
  python3 - "$STATE_DB_PATH" "$COMPOSE_PATH" "$RUNTIME_ENV_PATH" <<'PY'
import json
import sqlite3
import subprocess
import sys
from urllib.parse import quote

state_db_path, compose_path, runtime_env_path = sys.argv[1:4]
conn = sqlite3.connect(state_db_path)
wallets = conn.execute(
    "SELECT wallet_id, ufvk FROM merchant_wallets ORDER BY wallet_id"
).fetchall()

if not wallets:
    print("wallet_backfill=skipped")
    raise SystemExit(0)

curl_base = [
    "docker", "compose",
    "--env-file", runtime_env_path,
    "-f", compose_path,
    "exec", "-T", "juno-scan",
    "curl", "-fsS",
]

def post_json(url, payload):
    cmd = curl_base + [
        "-X", "POST",
        "-H", "content-type: application/json",
        "-d", json.dumps(payload, separators=(",", ":")),
        url,
    ]
    out = subprocess.check_output(cmd, text=True)
    return json.loads(out)

for wallet_id, ufvk in wallets:
    post_json(
        "http://127.0.0.1:8080/v1/wallets",
        {"wallet_id": wallet_id, "ufvk": ufvk},
    )

    from_height = 0
    batch_size = 10000
    while True:
        resp = post_json(
            f"http://127.0.0.1:8080/v1/wallets/{quote(wallet_id, safe='')}/backfill",
            {"from_height": from_height, "batch_size": batch_size},
        )
        next_height = resp.get("next_height")
        to_height = resp.get("to_height")
        scanned_to = resp.get("scanned_to")
        inserted_events = resp.get("inserted_events")
        inserted_notes = resp.get("inserted_notes")
        print(
            "wallet_backfill "
            f"wallet_id={wallet_id} "
            f"from_height={from_height} "
            f"scanned_to={scanned_to} "
            f"next_height={next_height} "
            f"inserted_events={inserted_events} "
            f"inserted_notes={inserted_notes}"
        )
        if next_height is None or to_height is None or next_height > to_height:
            break
        if next_height <= from_height:
            raise RuntimeError(
                f"backfill for {wallet_id} did not advance: from_height={from_height} next_height={next_height}"
            )
        from_height = int(next_height)
PY
}

if ! sqlite_integrity_ok; then
  restore_latest_backup
fi

backup_state_db
compose stop juno-pay-server juno-scan junocashd >/dev/null || true

rm -rf "$JUNOCASHD_DIR" "$SCAN_DB_PATH"
mkdir -p "$JUNOCASHD_DIR" "$SCAN_DIR" "$PAYSERVER_DIR"
fix_payserver_ownership
reset_scan_cursors

compose up -d junocashd >/dev/null
wait_service_ready junocashd

compose up -d juno-scan >/dev/null
wait_service_ready juno-scan
wait_scanner_tip_ready

register_wallets_and_backfill

compose up -d juno-pay-server >/dev/null
wait_service_ready juno-pay-server

echo "mode=bootstrap"
echo "recovered_root=$ROOT"
echo "junocashd_dir=$JUNOCASHD_DIR"
echo "scan_db_path=$SCAN_DB_PATH"
echo "payserver_dir=$PAYSERVER_DIR"
echo "state_db_path=$STATE_DB_PATH"
