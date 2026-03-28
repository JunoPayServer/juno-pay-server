#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  check-cutover-readiness.sh [options]

Options:
  --mode <bootstrap|warm|final>  Validation mode. Default: final
  --production-url <url>         Default: https://junopayserver.com
  --staging-url <url>            Default: https://staging.junopayserver.com
  --service-token-file <path>    JSON file with client_id/client_secret
  --access-client-id <id>        Cloudflare Access service token client id
  --access-client-secret <sec>   Cloudflare Access service token client secret
  --merchant-api-key <key>       Optional merchant API key for synthetic invoice create/fetch
  --progress-seconds <sec>       Progress window for bootstrap/warm. Default: 60
  --height-lag-tolerance <n>     Allowed block-height difference for parity checks. Default: 1
  --target-host <host>           DO target host. Default: 159.203.150.96
  --target-user <user>           DO target user. Default: root
  --target-ssh-key <path>        SSH key for DO target checks
  --target-deploy-root <path>    DO deploy root. Default: /opt/juno-pay
EOF
}

MODE="final"
PRODUCTION_URL="https://junopayserver.com"
STAGING_URL="https://staging.junopayserver.com"
SERVICE_TOKEN_FILE=""
ACCESS_CLIENT_ID="${ACCESS_CLIENT_ID:-}"
ACCESS_CLIENT_SECRET="${ACCESS_CLIENT_SECRET:-}"
MERCHANT_API_KEY=""
PROGRESS_SECONDS=60
HEIGHT_LAG_TOLERANCE=1
TARGET_HOST="159.203.150.96"
TARGET_USER="root"
TARGET_SSH_KEY="${TARGET_SSH_KEY:-}"
TARGET_DEPLOY_ROOT="/opt/juno-pay"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mode)
      MODE="${2:-}"
      shift 2
      ;;
    --production-url)
      PRODUCTION_URL="${2:-}"
      shift 2
      ;;
    --staging-url)
      STAGING_URL="${2:-}"
      shift 2
      ;;
    --service-token-file)
      SERVICE_TOKEN_FILE="${2:-}"
      shift 2
      ;;
    --access-client-id)
      ACCESS_CLIENT_ID="${2:-}"
      shift 2
      ;;
    --access-client-secret)
      ACCESS_CLIENT_SECRET="${2:-}"
      shift 2
      ;;
    --merchant-api-key)
      MERCHANT_API_KEY="${2:-}"
      shift 2
      ;;
    --progress-seconds)
      PROGRESS_SECONDS="${2:-}"
      shift 2
      ;;
    --height-lag-tolerance)
      HEIGHT_LAG_TOLERANCE="${2:-}"
      shift 2
      ;;
    --target-host)
      TARGET_HOST="${2:-}"
      shift 2
      ;;
    --target-user)
      TARGET_USER="${2:-}"
      shift 2
      ;;
    --target-ssh-key)
      TARGET_SSH_KEY="${2:-}"
      shift 2
      ;;
    --target-deploy-root)
      TARGET_DEPLOY_ROOT="${2:-}"
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

if [[ "$MODE" != "bootstrap" && "$MODE" != "warm" && "$MODE" != "final" ]]; then
  echo "--mode must be bootstrap, warm, or final" >&2
  exit 2
fi

if [[ -n "$SERVICE_TOKEN_FILE" ]]; then
  if [[ ! -f "$SERVICE_TOKEN_FILE" ]]; then
    echo "service token file not found: $SERVICE_TOKEN_FILE" >&2
    exit 2
  fi
  IFS=$'\t' read -r ACCESS_CLIENT_ID ACCESS_CLIENT_SECRET < <(
    SERVICE_TOKEN_FILE="$SERVICE_TOKEN_FILE" python3 - <<'PY'
import json
import os

with open(os.environ["SERVICE_TOKEN_FILE"], "r", encoding="utf-8") as f:
    doc = json.load(f)

print(f"{doc.get('client_id', '')}\t{doc.get('client_secret', '')}")
PY
  )
fi

if [[ -z "$ACCESS_CLIENT_ID" || -z "$ACCESS_CLIENT_SECRET" ]]; then
  echo "Cloudflare Access service token credentials are required for staging checks" >&2
  exit 2
fi

if [[ -z "$TARGET_SSH_KEY" ]]; then
  echo "target ssh key is required for DO readiness checks" >&2
  exit 2
fi

if [[ ! -f "$TARGET_SSH_KEY" ]]; then
  echo "target ssh key not found: $TARGET_SSH_KEY" >&2
  exit 2
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

CURL_OPTS=(
  -k
  -sS
  --connect-timeout 10
  --max-time 30
)

target_ssh() {
  ssh \
    -i "$TARGET_SSH_KEY" \
    -o BatchMode=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "${TARGET_USER}@${TARGET_HOST}" \
    "$@"
}

fetch_status() {
  local url output headers=()
  url="$1"
  output="$2"
  shift 2
  headers=("$@")
  curl "${CURL_OPTS[@]}" "${headers[@]}" "$url/v1/status" >"$output" || true
}

fetch_target_sample() {
  local meta_out node_out scanner_out payserver_out log_out raw_out
  meta_out="$1"
  node_out="$2"
  scanner_out="$3"
  payserver_out="$4"
  log_out="$5"
  raw_out="$tmpdir/$(basename "$meta_out" .txt).raw"

  target_ssh bash -se -- "$TARGET_DEPLOY_ROOT" >"$raw_out" <<'EOF'
set -euo pipefail

ROOT="$1"
COMPOSE_PATH="${ROOT}/docker-compose.yml"
RUNTIME_ENV_PATH="${ROOT}/runtime.env"

service_id() {
  docker compose --env-file "$RUNTIME_ENV_PATH" -f "$COMPOSE_PATH" ps -q "$1"
}

service_field() {
  local cid field
  cid="$1"
  field="$2"
  if [[ -z "$cid" ]]; then
    return
  fi
  docker inspect -f "$field" "$cid" 2>/dev/null || true
}

JCD_CID="$(service_id junocashd)"
SCAN_CID="$(service_id juno-scan)"
PAY_CID="$(service_id juno-pay-server)"

JCD_STARTED_AT="$(service_field "$JCD_CID" '{{.State.StartedAt}}')"
SCAN_STARTED_AT="$(service_field "$SCAN_CID" '{{.State.StartedAt}}')"
PAY_STARTED_AT="$(service_field "$PAY_CID" '{{.State.StartedAt}}')"

JCD_STATE="$(service_field "$JCD_CID" '{{.State.Status}}')"
SCAN_STATE="$(service_field "$SCAN_CID" '{{.State.Status}}')"
PAY_STATE="$(service_field "$PAY_CID" '{{.State.Status}}')"

JCD_HEALTH="$(service_field "$JCD_CID" '{{if .State.Health}}{{.State.Health.Status}}{{end}}')"
SCAN_HEALTH="$(service_field "$SCAN_CID" '{{if .State.Health}}{{.State.Health.Status}}{{end}}')"
PAY_HEALTH="$(service_field "$PAY_CID" '{{if .State.Health}}{{.State.Health.Status}}{{end}}')"

JCD_HEIGHT=""
if [[ -n "$JCD_CID" && "$JCD_STATE" == "running" ]]; then
  JCD_HEIGHT="$(docker exec "$JCD_CID" juno-cli getblockcount 2>/dev/null || true)"
fi

SCAN_HEALTH_JSON="{}"
if [[ -n "$SCAN_CID" && "$SCAN_STATE" == "running" ]]; then
  SCAN_HEALTH_JSON="$(docker exec "$SCAN_CID" curl -fsS http://127.0.0.1:8080/v1/health 2>/dev/null || printf '{}')"
fi

PAY_STATUS_JSON="{}"
if [[ -n "$PAY_CID" && "$PAY_STATE" == "running" ]]; then
  PAY_IP="$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' "$PAY_CID" 2>/dev/null || true)"
  if [[ -n "$PAY_IP" ]]; then
    PAY_STATUS_JSON="$(curl -fsS "http://${PAY_IP}:8080/v1/status" 2>/dev/null || printf '{}')"
  fi
fi

printf '__META__\n'
printf 'junocashd_container_id=%s\n' "$JCD_CID"
printf 'junocashd_started_at=%s\n' "$JCD_STARTED_AT"
printf 'junocashd_state_status=%s\n' "$JCD_STATE"
printf 'junocashd_health_status=%s\n' "$JCD_HEALTH"
printf 'juno_scan_container_id=%s\n' "$SCAN_CID"
printf 'juno_scan_started_at=%s\n' "$SCAN_STARTED_AT"
printf 'juno_scan_state_status=%s\n' "$SCAN_STATE"
printf 'juno_scan_health_status=%s\n' "$SCAN_HEALTH"
printf 'juno_pay_server_container_id=%s\n' "$PAY_CID"
printf 'juno_pay_server_started_at=%s\n' "$PAY_STARTED_AT"
printf 'juno_pay_server_state_status=%s\n' "$PAY_STATE"
printf 'juno_pay_server_health_status=%s\n' "$PAY_HEALTH"
printf '__NODE_HEIGHT__\n'
printf '%s\n' "$JCD_HEIGHT"
printf '__SCANNER_HEALTH__\n'
printf '%s\n' "$SCAN_HEALTH_JSON"
printf '__PAYSERVER_STATUS__\n'
printf '%s\n' "$PAY_STATUS_JSON"
printf '__SCAN_LOG__\n'
if [[ -n "$SCAN_CID" && -n "$SCAN_STARTED_AT" ]]; then
  docker logs --since "$SCAN_STARTED_AT" "$SCAN_CID" 2>&1 || true
fi
EOF

  python3 - <<'PY' "$raw_out" "$meta_out" "$node_out" "$scanner_out" "$payserver_out" "$log_out"
import sys

raw_path, meta_path, node_path, scanner_path, payserver_path, log_path = sys.argv[1:7]
with open(raw_path, "r", encoding="utf-8") as f:
    raw = f.read()

markers = [
    "__META__\n",
    "__NODE_HEIGHT__\n",
    "__SCANNER_HEALTH__\n",
    "__PAYSERVER_STATUS__\n",
    "__SCAN_LOG__\n",
]
if not raw.startswith(markers[0]):
    raise SystemExit("unexpected target sample output")

body = raw[len(markers[0]):]
meta_text, rest = body.split(markers[1], 1)
node_text, rest = rest.split(markers[2], 1)
scanner_text, rest = rest.split(markers[3], 1)
payserver_text, log_text = rest.split(markers[4], 1)

with open(meta_path, "w", encoding="utf-8") as f:
    f.write(meta_text)
with open(node_path, "w", encoding="utf-8") as f:
    f.write(node_text)
with open(scanner_path, "w", encoding="utf-8") as f:
    f.write(scanner_text)
with open(payserver_path, "w", encoding="utf-8") as f:
    f.write(payserver_text)
with open(log_path, "w", encoding="utf-8") as f:
    f.write(log_text)
PY
}

prod_health_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' "$PRODUCTION_URL/v1/health"
)"
prod_status_1="$tmpdir/prod-status-1.json"
fetch_status "$PRODUCTION_URL" "$prod_status_1"

staging_redirect_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' "$STAGING_URL/v1/health"
)"
staging_health_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' \
    -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
    -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET" \
    "$STAGING_URL/v1/health"
)"
staging_status_1="$tmpdir/staging-status-1.json"
fetch_status "$STAGING_URL" "$staging_status_1" \
  -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
  -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET"

prod_admin_redirect_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' "$PRODUCTION_URL/v1/admin/merchants"
)"
prod_admin_token_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' \
    -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
    -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET" \
    "$PRODUCTION_URL/v1/admin/merchants"
)"
staging_admin_token_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' \
    -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
    -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET" \
    "$STAGING_URL/v1/admin/merchants"
)"

invoice_status="not-run"
invoice_public_status="not-run"
if [[ -n "$MERCHANT_API_KEY" ]]; then
  invoice_create_file="$tmpdir/invoice-create.json"
  external_order_id="cutover-check-$(date +%s)"
  invoice_http_code="$(
    curl "${CURL_OPTS[@]}" -o "$invoice_create_file" -w '%{http_code}' \
      -H "Authorization: Bearer $MERCHANT_API_KEY" \
      -H "Content-Type: application/json" \
      -d "{\"external_order_id\":\"$external_order_id\",\"amount_zat\":1}" \
      "$STAGING_URL/v1/invoices"
  )"
  invoice_status="$invoice_http_code"
  if [[ "$invoice_http_code" == "200" || "$invoice_http_code" == "201" ]]; then
    read -r public_invoice_url < <(
      INVOICE_CREATE_FILE="$invoice_create_file" \
      STAGING_URL="$STAGING_URL" \
      python3 - <<'PY'
import json
import os
import sys
from urllib.parse import quote

with open(os.environ["INVOICE_CREATE_FILE"], "r", encoding="utf-8") as f:
    doc = json.load(f)

data = doc.get("data") or {}
invoice = data.get("invoice") or {}
invoice_id = invoice.get("invoice_id")
token = data.get("invoice_token")
if not invoice_id or not token:
    sys.exit(1)

print(f"{os.environ['STAGING_URL']}/v1/public/invoices/{quote(invoice_id)}?token={quote(token)}")
PY
    )
    invoice_public_status="$(
      curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' "$public_invoice_url"
    )"
  fi
fi

target_meta_1="$tmpdir/target-meta-1.txt"
target_node_1="$tmpdir/target-node-1.txt"
target_scanner_1="$tmpdir/target-scanner-1.json"
target_payserver_1="$tmpdir/target-payserver-1.json"
target_scan_log_1="$tmpdir/target-scan-log-1.txt"
fetch_target_sample "$target_meta_1" "$target_node_1" "$target_scanner_1" "$target_payserver_1" "$target_scan_log_1"

prod_status_2="$prod_status_1"
staging_status_2="$staging_status_1"
target_meta_2="$target_meta_1"
target_node_2="$target_node_1"
target_scanner_2="$target_scanner_1"
target_payserver_2="$target_payserver_1"
target_scan_log_2="$target_scan_log_1"

if [[ "$MODE" != "final" && "$PROGRESS_SECONDS" -gt 0 ]]; then
  sleep "$PROGRESS_SECONDS"
  prod_status_2="$tmpdir/prod-status-2.json"
  staging_status_2="$tmpdir/staging-status-2.json"
  fetch_status "$PRODUCTION_URL" "$prod_status_2"
  fetch_status "$STAGING_URL" "$staging_status_2" \
    -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
    -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET"
  target_meta_2="$tmpdir/target-meta-2.txt"
  target_node_2="$tmpdir/target-node-2.txt"
  target_scanner_2="$tmpdir/target-scanner-2.json"
  target_payserver_2="$tmpdir/target-payserver-2.json"
  target_scan_log_2="$tmpdir/target-scan-log-2.txt"
  fetch_target_sample "$target_meta_2" "$target_node_2" "$target_scanner_2" "$target_payserver_2" "$target_scan_log_2"
fi

scan_log_status="clean"
if grep -Eq "unknown to the objstorage provider|object size mismatch|Can't read block from disk|db connect: rocksdb: open:|panic: pebble: closed" "$target_scan_log_2"; then
  scan_log_status="corrupt"
fi

python3 - <<'PY' \
  "$MODE" \
  "$prod_status_1" "$staging_status_1" "$prod_status_2" "$staging_status_2" \
  "$target_meta_1" "$target_meta_2" "$target_node_1" "$target_node_2" \
  "$target_scanner_1" "$target_scanner_2" "$target_payserver_1" "$target_payserver_2" \
  "$prod_health_code" "$staging_redirect_code" "$staging_health_code" \
  "$prod_admin_redirect_code" "$prod_admin_token_code" "$staging_admin_token_code" \
  "$invoice_status" "$invoice_public_status" "$scan_log_status" "$HEIGHT_LAG_TOLERANCE"
import json
import sys

(
    mode,
    prod_status_1_path,
    staging_status_1_path,
    prod_status_2_path,
    staging_status_2_path,
    target_meta_1_path,
    target_meta_2_path,
    target_node_1_path,
    target_node_2_path,
    target_scanner_1_path,
    target_scanner_2_path,
    target_payserver_1_path,
    target_payserver_2_path,
    prod_health_code,
    staging_redirect_code,
    staging_health_code,
    prod_admin_redirect_code,
    prod_admin_token_code,
    staging_admin_token_code,
    invoice_status,
    invoice_public_status,
    scan_log_status,
    height_lag_tolerance,
) = sys.argv[1:]
height_lag_tolerance = int(height_lag_tolerance)


def load_json(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            raw = f.read().strip()
        if not raw:
            return {}
        return json.loads(raw)
    except Exception:
        return {}


def load_meta(path):
    out = {}
    with open(path, "r", encoding="utf-8") as f:
        for line in f:
            if "=" not in line:
                continue
            key, value = line.rstrip("\n").split("=", 1)
            out[key] = value
    return out


def load_height(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            raw = f.read().strip()
        if raw == "":
            return None
        return int(raw)
    except Exception:
        return None


def pick_status_data(doc):
    return (doc or {}).get("data") or {}


def pick_chain_height(doc):
    return ((doc.get("chain") or {}).get("best_height"))


def pick_cursor(doc):
    return ((doc.get("scanner") or {}).get("last_cursor_applied"))


def pick_pending(doc):
    return ((doc.get("event_delivery") or {}).get("pending_deliveries"))


def pick_scanner_connected(doc):
    return ((doc.get("scanner") or {}).get("connected"))


def pick_scanned_height(doc):
    return doc.get("scanned_height")


def within_tolerance(a, b):
    if a is None or b is None:
        return False
    return abs(int(a) - int(b)) <= height_lag_tolerance


prod_1 = pick_status_data(load_json(prod_status_1_path))
prod_2 = pick_status_data(load_json(prod_status_2_path))
staging_1 = pick_status_data(load_json(staging_status_1_path))
staging_2 = pick_status_data(load_json(staging_status_2_path))
target_pay_1 = pick_status_data(load_json(target_payserver_1_path))
target_pay_2 = pick_status_data(load_json(target_payserver_2_path))
target_scan_1 = load_json(target_scanner_1_path)
target_scan_2 = load_json(target_scanner_2_path)
target_meta_1 = load_meta(target_meta_1_path)
target_meta_2 = load_meta(target_meta_2_path)
target_node_1 = load_height(target_node_1_path)
target_node_2 = load_height(target_node_2_path)

prod_height_1 = pick_chain_height(prod_1)
prod_height_2 = pick_chain_height(prod_2)
staging_public_height_1 = pick_chain_height(staging_1)
staging_public_height_2 = pick_chain_height(staging_2)
target_cursor_1 = pick_cursor(target_pay_1)
target_cursor_2 = pick_cursor(target_pay_2)
target_pending_2 = pick_pending(target_pay_2)
target_scanner_connected_2 = pick_scanner_connected(target_pay_2)
target_scanner_tip_1 = pick_scanned_height(target_scan_1)
target_scanner_tip_2 = pick_scanned_height(target_scan_2)

print(f"mode={mode}")
print(f"prod_health_code={prod_health_code}")
print(f"staging_health_redirect_without_token={staging_redirect_code}")
print(f"staging_health_code_with_token={staging_health_code}")
print(f"prod_admin_redirect_without_token={prod_admin_redirect_code}")
print(f"prod_admin_code_with_token={prod_admin_token_code}")
print(f"staging_admin_code_with_token={staging_admin_token_code}")
print(f"initial_prod_height={prod_height_1}")
print(f"final_prod_height={prod_height_2}")
print(f"initial_staging_public_height={staging_public_height_1}")
print(f"final_staging_public_height={staging_public_height_2}")
print(f"initial_target_node_height={target_node_1}")
print(f"final_target_node_height={target_node_2}")
print(f"initial_target_scanner_tip={target_scanner_tip_1}")
print(f"final_target_scanner_tip={target_scanner_tip_2}")
print(f"initial_target_cursor={target_cursor_1}")
print(f"final_target_cursor={target_cursor_2}")
print(f"final_target_pending_deliveries={target_pending_2}")
print(f"final_target_scanner_connected={target_scanner_connected_2}")
print(f"height_lag_tolerance={height_lag_tolerance}")
print(f"junocashd_state_status={target_meta_2.get('junocashd_state_status')}")
print(f"junocashd_health_status={target_meta_2.get('junocashd_health_status')}")
print(f"juno_scan_state_status={target_meta_2.get('juno_scan_state_status')}")
print(f"juno_scan_health_status={target_meta_2.get('juno_scan_health_status')}")
print(f"juno_pay_server_state_status={target_meta_2.get('juno_pay_server_state_status')}")
print(f"juno_pay_server_health_status={target_meta_2.get('juno_pay_server_health_status')}")
print(f"scan_log_status={scan_log_status}")
print(f"synthetic_invoice_create_status={invoice_status}")
print(f"synthetic_invoice_public_status={invoice_public_status}")

errors = []

if prod_health_code != "200":
    errors.append("production health check failed")
if staging_redirect_code not in {"302", "303"}:
    errors.append("staging did not redirect unauthenticated requests to Access")
if prod_admin_redirect_code not in {"302", "303"}:
    errors.append("production admin path is not Access-protected")
if prod_admin_token_code not in {"200", "401"}:
    errors.append("production admin path with Access token returned an unexpected status")
if staging_admin_token_code not in {"200", "401"}:
    errors.append("staging admin path with Access token returned an unexpected status")
if staging_health_code != "200":
    errors.append("staging health check with Access token failed")
if target_meta_2.get("junocashd_health_status") != "healthy":
    errors.append("target junocashd is not healthy")
if target_node_2 is None:
    errors.append("target junocashd height is unavailable")
if target_meta_2.get("juno_scan_state_status") != "running":
    errors.append("target juno-scan container is not running")
if target_scanner_tip_2 is None:
    errors.append("target juno-scan health did not report scanned_height")
if target_scanner_connected_2 is not True:
    errors.append("target pay-server does not report scanner connected")
if target_pending_2 not in {0, None}:
    errors.append("target pay-server has pending deliveries")
if scan_log_status == "corrupt":
    errors.append("juno-scan logs show RocksDB corruption, block-read errors, or a fresh pebble panic")
if invoice_status != "not-run" and invoice_status not in {"200", "201"}:
    errors.append("synthetic invoice creation failed")
if invoice_public_status != "not-run" and invoice_public_status != "200":
    errors.append("synthetic public invoice fetch failed")

if mode == "bootstrap":
    prod_tip = prod_height_2 or 0
    node_progressed = False
    if target_node_1 is not None and target_node_2 is not None and target_node_2 > target_node_1:
        node_progressed = True
    if within_tolerance(target_node_2, prod_tip):
        node_progressed = True
    if not node_progressed:
        errors.append("bootstrap validation requires target node height progress or production parity")

    scan_progressed = False
    if target_scanner_tip_1 is not None and target_scanner_tip_2 is not None and target_scanner_tip_2 > target_scanner_tip_1:
        scan_progressed = True
    if within_tolerance(target_scanner_tip_2, target_node_2):
        scan_progressed = True
    if not scan_progressed:
        errors.append("bootstrap validation requires scanner tip progress or parity with the target node")

    cursor_progressed = False
    if target_cursor_1 is not None and target_cursor_2 is not None and target_cursor_2 > target_cursor_1:
        cursor_progressed = True
    if target_cursor_2 is not None and target_cursor_2 > 0 and target_scanner_tip_2 is not None:
        cursor_progressed = True
    cursor_started = (target_cursor_1 or 0) > 0 or (target_cursor_2 or 0) > 0
    if cursor_started and not cursor_progressed:
        errors.append("bootstrap validation requires pay-server cursor progress or a stable non-zero cursor")
elif mode == "warm":
    if not within_tolerance(target_node_2, prod_height_2):
        errors.append("warm validation requires target node height parity with production")
    if not within_tolerance(target_scanner_tip_2, target_node_2):
        errors.append("warm validation requires scanner tip parity with the target node")
    cursor_progressed = False
    if target_cursor_1 is not None and target_cursor_2 is not None and target_cursor_2 > target_cursor_1:
        cursor_progressed = True
    if target_cursor_2 is not None and target_cursor_2 > 0 and within_tolerance(target_scanner_tip_2, target_node_2):
        cursor_progressed = True
    if not cursor_progressed:
        errors.append("warm validation requires replay cursor progress or a stable non-zero cursor after scanner parity")
else:
    if not within_tolerance(target_node_2, prod_height_2):
        errors.append("final validation requires target node height parity with production")
    if not within_tolerance(target_scanner_tip_2, target_node_2):
        errors.append("final validation requires scanner tip parity with the target node")
    if target_cursor_2 is None or target_cursor_2 <= 0:
        errors.append("final validation requires a non-zero replay cursor")

if errors:
    for err in errors:
        print(err, file=sys.stderr)
    sys.exit(1)
PY
