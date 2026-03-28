#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  check-cutover-readiness.sh [options]

Options:
  --mode <warm|final>         Validation mode. Default: final
  --production-url <url>      Default: https://junopayserver.com
  --staging-url <url>         Default: https://staging.junopayserver.com
  --service-token-file <path> JSON file with client_id/client_secret
  --access-client-id <id>     Cloudflare Access service token client id
  --access-client-secret <s>  Cloudflare Access service token client secret
  --merchant-api-key <key>    Optional merchant API key for synthetic invoice create/fetch
  --progress-seconds <sec>    Warm-mode cursor progress window. Default: 60
  --scan-log-since <dur>      Docker log window for scanner checks. Default: 15m
  --target-host <host>        DO target host for juno-scan log checks. Default: 159.203.150.96
  --target-user <user>        DO target user. Default: root
  --target-ssh-key <path>     Optional SSH key for DO target log checks
  --target-deploy-root <path> DO deploy root. Default: /opt/juno-pay
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
SCAN_LOG_SINCE="15m"
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
    --scan-log-since)
      SCAN_LOG_SINCE="${2:-}"
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

if [[ "$MODE" != "warm" && "$MODE" != "final" ]]; then
  echo "--mode must be warm or final" >&2
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

if [[ -n "$TARGET_SSH_KEY" && ! -f "$TARGET_SSH_KEY" ]]; then
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

fetch_status() {
  local url output headers=()
  url="$1"
  output="$2"
  shift 2
  headers=("$@")
  curl "${CURL_OPTS[@]}" "${headers[@]}" "$url/v1/status" >"$output"
}

target_ssh() {
  ssh \
    -i "$TARGET_SSH_KEY" \
    -o BatchMode=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "${TARGET_USER}@${TARGET_HOST}" \
    "$@"
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

prod_status_2="$prod_status_1"
staging_status_2="$staging_status_1"
if [[ "$MODE" == "warm" && "$PROGRESS_SECONDS" -gt 0 ]]; then
  sleep "$PROGRESS_SECONDS"
  prod_status_2="$tmpdir/prod-status-2.json"
  staging_status_2="$tmpdir/staging-status-2.json"
  fetch_status "$PRODUCTION_URL" "$prod_status_2"
  fetch_status "$STAGING_URL" "$staging_status_2" \
    -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
    -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET"
fi

scan_log_status="skipped"
scan_log_file="$tmpdir/juno-scan.log"
if [[ -n "$TARGET_SSH_KEY" ]]; then
  target_ssh bash -se -- "$TARGET_DEPLOY_ROOT" "$SCAN_LOG_SINCE" >"$scan_log_file" <<'EOF'
set -euo pipefail

ROOT="$1"
LOG_SINCE="$2"
COMPOSE_PATH="${ROOT}/docker-compose.yml"
RUNTIME_ENV_PATH="${ROOT}/runtime.env"
cid="$(docker compose --env-file "$RUNTIME_ENV_PATH" -f "$COMPOSE_PATH" ps -q juno-scan)"
if [[ -z "$cid" ]]; then
  exit 1
fi
docker logs --since "$LOG_SINCE" "$cid" 2>&1 || true
EOF
  if grep -Eq "unknown to the objstorage provider|object size mismatch|Can't read block from disk|db connect: rocksdb: open:|panic: pebble: closed" "$scan_log_file"; then
    scan_log_status="corrupt"
  else
    scan_log_status="clean"
  fi
fi

python3 - <<'PY' \
  "$MODE" \
  "$prod_status_1" "$staging_status_1" "$prod_status_2" "$staging_status_2" \
  "$prod_health_code" "$staging_redirect_code" "$staging_health_code" \
  "$prod_admin_redirect_code" "$prod_admin_token_code" "$staging_admin_token_code" \
  "$invoice_status" "$invoice_public_status" "$scan_log_status"
import json
import sys

(
    mode,
    prod_status_1_path,
    staging_status_1_path,
    prod_status_2_path,
    staging_status_2_path,
    prod_health_code,
    staging_redirect_code,
    staging_health_code,
    prod_admin_redirect_code,
    prod_admin_token_code,
    staging_admin_token_code,
    invoice_status,
    invoice_public_status,
    scan_log_status,
) = sys.argv[1:]

with open(prod_status_1_path, "r", encoding="utf-8") as f:
    prod_1 = (json.load(f).get("data") or {})
with open(staging_status_1_path, "r", encoding="utf-8") as f:
    staging_1 = (json.load(f).get("data") or {})
with open(prod_status_2_path, "r", encoding="utf-8") as f:
    prod_2 = (json.load(f).get("data") or {})
with open(staging_status_2_path, "r", encoding="utf-8") as f:
    staging_2 = (json.load(f).get("data") or {})

def pick_height(doc):
    chain = doc.get("chain") or {}
    return chain.get("best_height", doc.get("chain_height"))

def pick_cursor(doc):
    scanner = doc.get("scanner") or {}
    return scanner.get("last_cursor_applied")

def pick_pending(doc):
    delivery = doc.get("event_delivery") or {}
    return delivery.get("pending_deliveries")

def pick_scanner_connected(doc):
    scanner = doc.get("scanner") or {}
    return scanner.get("connected", doc.get("scanner_connected"))

prod_height_1 = pick_height(prod_1)
staging_height_1 = pick_height(staging_1)
prod_height_2 = pick_height(prod_2)
staging_height_2 = pick_height(staging_2)
prod_cursor_1 = pick_cursor(prod_1)
staging_cursor_1 = pick_cursor(staging_1)
prod_cursor_2 = pick_cursor(prod_2)
staging_cursor_2 = pick_cursor(staging_2)
prod_pending_2 = pick_pending(prod_2)
staging_pending_2 = pick_pending(staging_2)
prod_scanner_connected_2 = pick_scanner_connected(prod_2)
staging_scanner_connected_2 = pick_scanner_connected(staging_2)

print(f"mode={mode}")
print(f"prod_health_code={prod_health_code}")
print(f"staging_health_redirect_without_token={staging_redirect_code}")
print(f"staging_health_code_with_token={staging_health_code}")
print(f"prod_admin_redirect_without_token={prod_admin_redirect_code}")
print(f"prod_admin_code_with_token={prod_admin_token_code}")
print(f"staging_admin_code_with_token={staging_admin_token_code}")
print(f"initial_prod_height={prod_height_1}")
print(f"initial_staging_height={staging_height_1}")
print(f"final_prod_height={prod_height_2}")
print(f"final_staging_height={staging_height_2}")
print(f"initial_height_lag={(prod_height_1 or 0) - (staging_height_1 or 0)}")
print(f"final_height_lag={(prod_height_2 or 0) - (staging_height_2 or 0)}")
print(f"initial_prod_last_cursor_applied={prod_cursor_1}")
print(f"initial_staging_last_cursor_applied={staging_cursor_1}")
print(f"final_prod_last_cursor_applied={prod_cursor_2}")
print(f"final_staging_last_cursor_applied={staging_cursor_2}")
print(f"initial_cursor_lag={(prod_cursor_1 or 0) - (staging_cursor_1 or 0)}")
print(f"final_cursor_lag={(prod_cursor_2 or 0) - (staging_cursor_2 or 0)}")
print(f"final_prod_pending_deliveries={prod_pending_2}")
print(f"final_staging_pending_deliveries={staging_pending_2}")
print(f"final_prod_scanner_connected={prod_scanner_connected_2}")
print(f"final_staging_scanner_connected={staging_scanner_connected_2}")
print(f"scan_log_status={scan_log_status}")
print(f"synthetic_invoice_create_status={invoice_status}")
print(f"synthetic_invoice_public_status={invoice_public_status}")

errors = []

if prod_health_code != "200":
    errors.append("production health check failed")
if staging_redirect_code not in {"302", "303"}:
    errors.append("staging did not redirect unauthenticated requests to Access")
if staging_health_code != "200":
    errors.append("staging health check with Access token failed")
if prod_admin_redirect_code not in {"302", "303"}:
    errors.append("production admin path is not Access-protected")
if prod_admin_token_code not in {"200", "401"}:
    errors.append("production admin path with Access token returned an unexpected status")
if staging_admin_token_code not in {"200", "401"}:
    errors.append("staging admin path with Access token returned an unexpected status")
if prod_scanner_connected_2 is not True:
    errors.append("production scanner is not connected")
if staging_scanner_connected_2 is not True:
    errors.append("staging scanner is not connected")
if scan_log_status == "corrupt":
    errors.append("juno-scan logs show RocksDB corruption or block-read errors")
if invoice_status != "not-run" and invoice_status not in {"200", "201"}:
    errors.append("synthetic invoice creation failed")
if invoice_public_status != "not-run" and invoice_public_status != "200":
    errors.append("synthetic public invoice fetch failed")

if mode == "warm":
    if (prod_height_2 or 0) != (staging_height_2 or 0):
        errors.append("warm validation requires staging height parity")
    if staging_cursor_2 is None:
        errors.append("warm validation requires a staging scan cursor")
    else:
        progressed = False
        if staging_cursor_1 is not None and staging_cursor_2 > staging_cursor_1:
            progressed = True
        if prod_cursor_2 is not None and staging_cursor_2 == prod_cursor_2:
            progressed = True
        if not progressed:
            errors.append("warm validation requires staging cursor progress or full parity")
else:
    if (prod_height_2 or 0) != (staging_height_2 or 0):
        errors.append("final validation requires height parity")
    if (prod_cursor_2 or 0) != (staging_cursor_2 or 0):
        errors.append("final validation requires cursor parity")

if errors:
    for err in errors:
        print(err, file=sys.stderr)
    sys.exit(1)
PY
