#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
READINESS_SCRIPT="${SCRIPT_DIR}/check-cutover-readiness.sh"

usage() {
  cat <<'EOF'
Usage:
  wait-bootstrap-parity.sh [options]

Options:
  --required-consecutive <n>    Required consecutive parity samples. Default: 2
  --interval-seconds <sec>      Delay between samples. Default: 900
  --progress-seconds <sec>      Inner readiness progress window. Default: 15
  --max-samples <n>             Stop after N samples. Default: 0 (no limit)
  --production-url <url>        Default: https://junopayserver.com
  --staging-url <url>           Default: https://staging.junopayserver.com
  --service-token-file <path>   JSON file with client_id/client_secret
  --access-client-id <id>       Cloudflare Access service token client id
  --access-client-secret <sec>  Cloudflare Access service token client secret
  --target-host <host>          DO target host. Default: 159.203.150.96
  --target-user <user>          DO target user. Default: root
  --target-ssh-key <path>       SSH key for DO target checks
  --target-deploy-root <path>   DO deploy root. Default: /opt/juno-pay
EOF
}

REQUIRED_CONSECUTIVE=2
INTERVAL_SECONDS=900
PROGRESS_SECONDS=15
MAX_SAMPLES=0
PRODUCTION_URL="https://junopayserver.com"
STAGING_URL="https://staging.junopayserver.com"
SERVICE_TOKEN_FILE=""
ACCESS_CLIENT_ID="${ACCESS_CLIENT_ID:-}"
ACCESS_CLIENT_SECRET="${ACCESS_CLIENT_SECRET:-}"
TARGET_HOST="159.203.150.96"
TARGET_USER="root"
TARGET_SSH_KEY="${TARGET_SSH_KEY:-}"
TARGET_DEPLOY_ROOT="/opt/juno-pay"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --required-consecutive)
      REQUIRED_CONSECUTIVE="${2:-}"
      shift 2
      ;;
    --interval-seconds)
      INTERVAL_SECONDS="${2:-}"
      shift 2
      ;;
    --progress-seconds)
      PROGRESS_SECONDS="${2:-}"
      shift 2
      ;;
    --max-samples)
      MAX_SAMPLES="${2:-}"
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

if [[ ! -x "$READINESS_SCRIPT" ]]; then
  echo "readiness script missing or not executable: $READINESS_SCRIPT" >&2
  exit 1
fi

consecutive=0
sample=0

while true; do
  sample=$((sample + 1))
  timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

  cmd=(
    "$READINESS_SCRIPT"
    --mode bootstrap
    --production-url "$PRODUCTION_URL"
    --staging-url "$STAGING_URL"
    --progress-seconds "$PROGRESS_SECONDS"
    --target-host "$TARGET_HOST"
    --target-user "$TARGET_USER"
    --target-ssh-key "$TARGET_SSH_KEY"
    --target-deploy-root "$TARGET_DEPLOY_ROOT"
  )
  if [[ -n "$SERVICE_TOKEN_FILE" ]]; then
    cmd+=(--service-token-file "$SERVICE_TOKEN_FILE")
  else
    cmd+=(--access-client-id "$ACCESS_CLIENT_ID" --access-client-secret "$ACCESS_CLIENT_SECRET")
  fi

  set +e
  output="$("${cmd[@]}" 2>&1)"
  status=$?
  set -e

  summary="$(
    SUMMARY_INPUT="$output" python3 - "$status" "$timestamp" "$sample" "$consecutive" <<'PY'
import os
import sys

status = int(sys.argv[1])
timestamp = sys.argv[2]
sample = int(sys.argv[3])
prior_consecutive = int(sys.argv[4])

data = {}
for line in os.environ.get("SUMMARY_INPUT", "").splitlines():
    line = line.rstrip("\n")
    if "=" not in line:
        continue
    key, value = line.split("=", 1)
    data[key] = value

prod = data.get("final_prod_height")
node = data.get("final_target_node_height")
scan = data.get("final_target_scanner_tip")
cursor = data.get("final_target_cursor")

parity_ok = (
    status == 0
    and prod not in {None, "", "None"}
    and node == prod
    and scan == node
)

print(f"sample={sample}")
print(f"timestamp={timestamp}")
print(f"bootstrap_status={status}")
print(f"prod_height={prod}")
print(f"target_node_height={node}")
print(f"target_scanner_tip={scan}")
print(f"target_cursor={cursor}")
print(f"parity_ok={'yes' if parity_ok else 'no'}")
print(f"prior_consecutive={prior_consecutive}")
PY
  )"

  parity_ok="$(printf '%s\n' "$summary" | awk -F= '$1=="parity_ok"{print $2}')"
  prod_height="$(printf '%s\n' "$summary" | awk -F= '$1=="prod_height"{print $2}')"
  node_height="$(printf '%s\n' "$summary" | awk -F= '$1=="target_node_height"{print $2}')"
  scanner_tip="$(printf '%s\n' "$summary" | awk -F= '$1=="target_scanner_tip"{print $2}')"
  cursor="$(printf '%s\n' "$summary" | awk -F= '$1=="target_cursor"{print $2}')"

  if [[ "$parity_ok" == "yes" ]]; then
    consecutive=$((consecutive + 1))
  else
    consecutive=0
  fi

  printf 'bootstrap_sample=%s timestamp=%s prod_height=%s target_node_height=%s target_scanner_tip=%s target_cursor=%s parity_ok=%s consecutive=%s/%s\n' \
    "$sample" "$timestamp" "$prod_height" "$node_height" "$scanner_tip" "$cursor" "$parity_ok" "$consecutive" "$REQUIRED_CONSECUTIVE"

  if [[ "$status" -ne 0 ]]; then
    printf '%s\n' "$output" >&2
  fi

  if (( consecutive >= REQUIRED_CONSECUTIVE )); then
    echo "bootstrap parity reached for ${REQUIRED_CONSECUTIVE} consecutive samples"
    exit 0
  fi

  if (( MAX_SAMPLES > 0 && sample >= MAX_SAMPLES )); then
    echo "bootstrap parity not reached within ${MAX_SAMPLES} samples" >&2
    exit 1
  fi

  sleep "$INTERVAL_SECONDS"
done
