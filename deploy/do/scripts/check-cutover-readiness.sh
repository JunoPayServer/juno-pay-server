#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  check-cutover-readiness.sh [options]

Options:
  --production-url <url>       Default: https://junopayserver.com
  --staging-url <url>          Default: https://staging.junopayserver.com
  --service-token-file <path>  JSON file with client_id/client_secret
  --access-client-id <id>      Cloudflare Access service token client id
  --access-client-secret <s>   Cloudflare Access service token client secret
  --merchant-api-key <key>     Optional merchant API key for synthetic invoice create/fetch
EOF
}

PRODUCTION_URL="https://junopayserver.com"
STAGING_URL="https://staging.junopayserver.com"
SERVICE_TOKEN_FILE=""
ACCESS_CLIENT_ID="${ACCESS_CLIENT_ID:-}"
ACCESS_CLIENT_SECRET="${ACCESS_CLIENT_SECRET:-}"
MERCHANT_API_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
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

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

CURL_OPTS=(
  -k
  -sS
  --connect-timeout 10
  --max-time 30
)

prod_health_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' "$PRODUCTION_URL/v1/health"
)"
prod_status_file="$tmpdir/prod-status.json"
curl "${CURL_OPTS[@]}" "$PRODUCTION_URL/v1/status" >"$prod_status_file"

staging_redirect_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' "$STAGING_URL/v1/health"
)"
staging_health_code="$(
  curl "${CURL_OPTS[@]}" -o /dev/null -w '%{http_code}' \
    -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
    -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET" \
    "$STAGING_URL/v1/health"
)"
staging_status_file="$tmpdir/staging-status.json"
curl "${CURL_OPTS[@]}" \
  -H "CF-Access-Client-Id: $ACCESS_CLIENT_ID" \
  -H "CF-Access-Client-Secret: $ACCESS_CLIENT_SECRET" \
  "$STAGING_URL/v1/status" >"$staging_status_file"

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

python3 - <<'PY' "$prod_status_file" "$staging_status_file" "$prod_health_code" "$staging_redirect_code" "$staging_health_code" "$prod_admin_redirect_code" "$prod_admin_token_code" "$staging_admin_token_code" "$invoice_status" "$invoice_public_status"
import json
import sys

prod_status_path, staging_status_path = sys.argv[1], sys.argv[2]
prod_health_code = sys.argv[3]
staging_redirect_code = sys.argv[4]
staging_health_code = sys.argv[5]
prod_admin_redirect_code = sys.argv[6]
prod_admin_token_code = sys.argv[7]
staging_admin_token_code = sys.argv[8]
invoice_status = sys.argv[9]
invoice_public_status = sys.argv[10]

with open(prod_status_path, "r", encoding="utf-8") as f:
    prod = (json.load(f).get("data") or {})
with open(staging_status_path, "r", encoding="utf-8") as f:
    staging = (json.load(f).get("data") or {})

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

print(f"prod_health_code={prod_health_code}")
print(f"staging_health_redirect_without_token={staging_redirect_code}")
print(f"staging_health_code_with_token={staging_health_code}")
print(f"prod_admin_redirect_without_token={prod_admin_redirect_code}")
print(f"prod_admin_code_with_token={prod_admin_token_code}")
print(f"staging_admin_code_with_token={staging_admin_token_code}")
print(f"prod_height={pick_height(prod)}")
print(f"staging_height={pick_height(staging)}")
print(f"height_lag={(pick_height(prod) or 0) - (pick_height(staging) or 0)}")
print(f"prod_last_cursor_applied={pick_cursor(prod)}")
print(f"staging_last_cursor_applied={pick_cursor(staging)}")
print(f"cursor_lag={(pick_cursor(prod) or 0) - (pick_cursor(staging) or 0)}")
print(f"prod_pending_deliveries={pick_pending(prod)}")
print(f"staging_pending_deliveries={pick_pending(staging)}")
print(f"prod_scanner_connected={pick_scanner_connected(prod)}")
print(f"staging_scanner_connected={pick_scanner_connected(staging)}")
print(f"synthetic_invoice_create_status={invoice_status}")
print(f"synthetic_invoice_public_status={invoice_public_status}")
PY

if [[ "$prod_health_code" != "200" ]]; then
  echo "production health check failed" >&2
  exit 1
fi
if [[ "$staging_redirect_code" != "302" && "$staging_redirect_code" != "303" ]]; then
  echo "staging did not redirect unauthenticated requests to Access" >&2
  exit 1
fi
if [[ "$staging_health_code" != "200" ]]; then
  echo "staging health check with Access token failed" >&2
  exit 1
fi
if [[ "$prod_admin_redirect_code" != "302" && "$prod_admin_redirect_code" != "303" ]]; then
  echo "production admin path is not Access-protected" >&2
  exit 1
fi
if [[ "$prod_admin_token_code" != "401" && "$prod_admin_token_code" != "200" ]]; then
  echo "production admin path with Access token returned an unexpected status" >&2
  exit 1
fi
if [[ "$staging_admin_token_code" != "401" && "$staging_admin_token_code" != "200" ]]; then
  echo "staging admin path with Access token returned an unexpected status" >&2
  exit 1
fi
