#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  switch-lb-primary.sh --target aws|do [options]

Required environment:
  CLOUDFLARE_EMAIL
  CLOUDFLARE_GLOBAL_API_KEY

Options:
  --target <aws|do>             Which pool should be primary
  --exclusive                   Keep only the target pool active in default_pools
  --dry-run                     Print the desired changes without applying them
  --zone-id <id>                Default: 6a7b914cfaab0d683a7a459dd9990816
  --aws-pool-id <id>            Default: 4f638d5ea23116d07e1cf1461524a716
  --do-pool-id <id>             Default: 26af9c8e599b3185f6d5dc4a8a283d40
  --apex-lb-id <id>             Default: 429e1bd9e2e202fb83fdc05250fce2ef
  --www-lb-id <id>              Default: 0de5f8521b63674960d049ca25e7c8d6
EOF
}

TARGET=""
EXCLUSIVE=0
DRY_RUN=0
ZONE_ID="6a7b914cfaab0d683a7a459dd9990816"
AWS_POOL_ID="4f638d5ea23116d07e1cf1461524a716"
DO_POOL_ID="26af9c8e599b3185f6d5dc4a8a283d40"
APEX_LB_ID="429e1bd9e2e202fb83fdc05250fce2ef"
WWW_LB_ID="0de5f8521b63674960d049ca25e7c8d6"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --target)
      TARGET="${2:-}"
      shift 2
      ;;
    --exclusive)
      EXCLUSIVE=1
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    --zone-id)
      ZONE_ID="${2:-}"
      shift 2
      ;;
    --aws-pool-id)
      AWS_POOL_ID="${2:-}"
      shift 2
      ;;
    --do-pool-id)
      DO_POOL_ID="${2:-}"
      shift 2
      ;;
    --apex-lb-id)
      APEX_LB_ID="${2:-}"
      shift 2
      ;;
    --www-lb-id)
      WWW_LB_ID="${2:-}"
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

if [[ "$TARGET" != "aws" && "$TARGET" != "do" ]]; then
  usage >&2
  exit 2
fi

if [[ -z "${CLOUDFLARE_EMAIL:-}" || -z "${CLOUDFLARE_GLOBAL_API_KEY:-}" ]]; then
  echo "CLOUDFLARE_EMAIL and CLOUDFLARE_GLOBAL_API_KEY must be set" >&2
  exit 2
fi

if [[ "$TARGET" == "aws" ]]; then
  PRIMARY="$AWS_POOL_ID"
  SECONDARY="$DO_POOL_ID"
else
  PRIMARY="$DO_POOL_ID"
  SECONDARY="$AWS_POOL_ID"
fi

if [[ "$EXCLUSIVE" == "1" ]]; then
  DEFAULT_POOLS_JSON="[\"$PRIMARY\"]"
else
  DEFAULT_POOLS_JSON="[\"$PRIMARY\",\"$SECONDARY\"]"
fi

update_lb() {
  local lb_id payload response
  lb_id="$1"

  response="$(
    curl -fsS \
      -H "X-Auth-Email: $CLOUDFLARE_EMAIL" \
      -H "X-Auth-Key: $CLOUDFLARE_GLOBAL_API_KEY" \
      "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/load_balancers/$lb_id"
  )"

  payload="$(
    LB_RESPONSE="$response" \
    LB_DEFAULT_POOLS="$DEFAULT_POOLS_JSON" \
    LB_FALLBACK="$PRIMARY" \
    python3 - <<'PY'
import json
import os

doc = json.loads(os.environ["LB_RESPONSE"])
result = doc["result"]
payload = {
    "description": result.get("description", ""),
    "enabled": result["enabled"],
    "fallback_pool": os.environ["LB_FALLBACK"],
    "name": result["name"],
    "default_pools": json.loads(os.environ["LB_DEFAULT_POOLS"]),
    "proxied": result["proxied"],
}
if "ttl" in result and result["ttl"] is not None:
    payload["ttl"] = result["ttl"]
for key in (
    "country_pools",
    "region_pools",
    "pop_pools",
    "random_steering",
    "adaptive_routing",
    "location_strategy",
    "rules",
    "session_affinity",
    "session_affinity_attributes",
):
    if key in result and result[key] not in (None, {}, [], ""):
        payload[key] = result[key]
print(json.dumps(payload))
PY
  )"

  if [[ "$DRY_RUN" == "1" ]]; then
    echo "lb_id=$lb_id payload=$payload"
    return
  fi

  curl -fsS -X PUT \
    -H "Content-Type: application/json" \
    -H "X-Auth-Email: $CLOUDFLARE_EMAIL" \
    -H "X-Auth-Key: $CLOUDFLARE_GLOBAL_API_KEY" \
    --data "$payload" \
    "https://api.cloudflare.com/client/v4/zones/$ZONE_ID/load_balancers/$lb_id" \
    >/dev/null
}

update_lb "$APEX_LB_ID"
update_lb "$WWW_LB_ID"

echo "target=$TARGET"
echo "exclusive=$EXCLUSIVE"
echo "apex_lb_id=$APEX_LB_ID"
echo "www_lb_id=$WWW_LB_ID"
