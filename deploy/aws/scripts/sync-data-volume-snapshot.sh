#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
HELPER_SCRIPT_PATH="$SCRIPT_DIR/helper-snapshot-stream.sh"
WARM_RESET_SCRIPT_PATH="$SCRIPT_DIR/../../do/scripts/rebuild-staging-scan-state.sh"

usage() {
  cat <<'EOF'
Usage:
  sync-data-volume-snapshot.sh --do-ssh-key <path> [options]

Options:
  --region <region>                         AWS region. Default: us-east-1
  --source-volume-id <volume-id>           AWS source data volume. Default: vol-0d5701021c67b3f7d
  --helper-instance-id <instance-id>       Helper EC2 instance. Default: i-06f8b5e5c0aa7dece
  --helper-availability-zone <az>          Helper AZ. Default: us-east-1a
  --helper-attach-device <device>          Helper attach device. Default: /dev/sdg
  --helper-mount-point <path>              Helper mount point. Default: /mnt/juno-pay-source
  --target-host <host>                     DO target host. Default: 159.203.150.96
  --target-user <user>                     DO target user. Default: root
  --target-root <path>                     DO target root. Default: /opt/juno-pay/data
  --target-deploy-root <path>              DO deploy root. Default: /opt/juno-pay
  --do-firewall-id <firewall-id>           DO firewall. Default: 8b080d85-7878-4f38-8d97-981eb80a0e3c
  --do-ssh-key <path>                      Existing DO SSH private key
  --snapshot-kind <warm|cold>              Snapshot label. Default: warm
  --snapshot-id <snapshot-id>              Reuse an existing snapshot instead of creating a new one
  --delete-snapshot-id <snapshot-id>       Delete a prior successful warm snapshot after this sync succeeds
  --readiness-service-token-file <path>    Optional Cloudflare Access service token file for post-sync validation
  --merchant-api-key <key>                 Optional merchant API key for synthetic staging invoice validation
  --keep-helper-running                    Do not stop the helper even if this script started it
  --stop-helper-when-done                  Stop the helper at the end even if it was already running
EOF
}

REGION="us-east-1"
SOURCE_VOLUME_ID="vol-0d5701021c67b3f7d"
HELPER_INSTANCE_ID="i-06f8b5e5c0aa7dece"
HELPER_AZ="us-east-1a"
HELPER_ATTACH_DEVICE="/dev/sdg"
HELPER_MOUNT_POINT="/mnt/juno-pay-source"
TARGET_HOST="159.203.150.96"
TARGET_USER="root"
TARGET_ROOT="/opt/juno-pay/data"
TARGET_DEPLOY_ROOT="/opt/juno-pay"
DO_FIREWALL_ID="8b080d85-7878-4f38-8d97-981eb80a0e3c"
DO_SSH_KEY=""
SNAPSHOT_KIND="warm"
EXISTING_SNAPSHOT_ID=""
DELETE_SNAPSHOT_ID=""
READINESS_SERVICE_TOKEN_FILE=""
MERCHANT_API_KEY=""
KEEP_HELPER_RUNNING=0
FORCE_STOP_HELPER=0

SNAPSHOT_ID=""
TEMP_VOLUME_ID=""
HELPER_WAS_RUNNING=0
HELPER_STARTED_BY_SCRIPT=0
DO_FIREWALL_RULE_ADDED=0
HELPER_EGRESS_CIDR=""
TARGET_CORE_STOPPED=0
TARGET_POST_SYNC_COMPLETED=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --region)
      REGION="${2:-}"
      shift 2
      ;;
    --source-volume-id)
      SOURCE_VOLUME_ID="${2:-}"
      shift 2
      ;;
    --helper-instance-id)
      HELPER_INSTANCE_ID="${2:-}"
      shift 2
      ;;
    --helper-availability-zone)
      HELPER_AZ="${2:-}"
      shift 2
      ;;
    --helper-attach-device)
      HELPER_ATTACH_DEVICE="${2:-}"
      shift 2
      ;;
    --helper-mount-point)
      HELPER_MOUNT_POINT="${2:-}"
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
    --target-root)
      TARGET_ROOT="${2:-}"
      shift 2
      ;;
    --target-deploy-root)
      TARGET_DEPLOY_ROOT="${2:-}"
      shift 2
      ;;
    --do-firewall-id)
      DO_FIREWALL_ID="${2:-}"
      shift 2
      ;;
    --do-ssh-key)
      DO_SSH_KEY="${2:-}"
      shift 2
      ;;
    --snapshot-kind)
      SNAPSHOT_KIND="${2:-}"
      shift 2
      ;;
    --snapshot-id)
      EXISTING_SNAPSHOT_ID="${2:-}"
      shift 2
      ;;
    --delete-snapshot-id)
      DELETE_SNAPSHOT_ID="${2:-}"
      shift 2
      ;;
    --readiness-service-token-file)
      READINESS_SERVICE_TOKEN_FILE="${2:-}"
      shift 2
      ;;
    --merchant-api-key)
      MERCHANT_API_KEY="${2:-}"
      shift 2
      ;;
    --keep-helper-running)
      KEEP_HELPER_RUNNING=1
      shift
      ;;
    --stop-helper-when-done)
      FORCE_STOP_HELPER=1
      shift
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

if [[ -z "$DO_SSH_KEY" ]]; then
  echo "--do-ssh-key is required" >&2
  exit 2
fi

if [[ ! -f "$DO_SSH_KEY" ]]; then
  echo "DO SSH private key not found: $DO_SSH_KEY" >&2
  exit 2
fi

if [[ "$SNAPSHOT_KIND" != "warm" && "$SNAPSHOT_KIND" != "cold" ]]; then
  echo "--snapshot-kind must be warm or cold" >&2
  exit 2
fi

if [[ ! -f "$HELPER_SCRIPT_PATH" ]]; then
  echo "helper script missing: $HELPER_SCRIPT_PATH" >&2
  exit 2
fi

if [[ ! -f "$WARM_RESET_SCRIPT_PATH" ]]; then
  echo "warm reset script missing: $WARM_RESET_SCRIPT_PATH" >&2
  exit 2
fi

cleanup() {
  local state

  if [[ "$DO_FIREWALL_RULE_ADDED" == "1" && -n "$HELPER_EGRESS_CIDR" ]]; then
    doctl --context juno compute firewall remove-rules "$DO_FIREWALL_ID" \
      --inbound-rules "protocol:tcp,ports:22,address:${HELPER_EGRESS_CIDR}" \
      >/dev/null || true
  fi

  if [[ -n "$TEMP_VOLUME_ID" ]]; then
    state="$(
      aws ec2 describe-volumes \
        --region "$REGION" \
        --volume-ids "$TEMP_VOLUME_ID" \
        --query 'Volumes[0].Attachments[0].State' \
        --output text 2>/dev/null || true
    )"
    if [[ "$state" != "None" && "$state" != "detached" && "$state" != "NoneType" && -n "$state" ]]; then
      aws ec2 detach-volume \
        --region "$REGION" \
        --volume-id "$TEMP_VOLUME_ID" \
        >/dev/null 2>&1 || true
      aws ec2 wait volume-available --region "$REGION" --volume-ids "$TEMP_VOLUME_ID" >/dev/null 2>&1 || true
    fi
    aws ec2 delete-volume --region "$REGION" --volume-id "$TEMP_VOLUME_ID" >/dev/null 2>&1 || true
  fi

  if [[ "$FORCE_STOP_HELPER" == "1" ]] || [[ "$KEEP_HELPER_RUNNING" == "0" && "$HELPER_STARTED_BY_SCRIPT" == "1" ]]; then
    aws ec2 stop-instances --region "$REGION" --instance-ids "$HELPER_INSTANCE_ID" >/dev/null 2>&1 || true
    aws ec2 wait instance-stopped --region "$REGION" --instance-ids "$HELPER_INSTANCE_ID" >/dev/null 2>&1 || true
  fi

  if [[ "$TARGET_CORE_STOPPED" == "1" && "$TARGET_POST_SYNC_COMPLETED" == "0" ]]; then
    echo "warning: target core services may still be stopped or inconsistent on ${TARGET_HOST}" >&2
  fi
}

trap cleanup EXIT

ssm_send() {
  local instance_id params_json
  instance_id="$1"
  shift

  params_json="$(
    python3 - "$@" <<'PY'
import json
import sys

print(json.dumps({"commands": sys.argv[1:]}))
PY
  )"

  aws ssm send-command \
    --region "$REGION" \
    --instance-ids "$instance_id" \
    --document-name AWS-RunShellScript \
    --parameters "$params_json" \
    --query 'Command.CommandId' \
    --output text
}

ssm_wait_capture() {
  local cmd_id instance_id status stdout stderr
  cmd_id="$1"
  instance_id="$2"

  for _ in {1..180}; do
    status="$(
      aws ssm get-command-invocation \
        --region "$REGION" \
        --command-id "$cmd_id" \
        --instance-id "$instance_id" \
        --query 'Status' \
        --output text 2>/dev/null || true
    )"
    case "$status" in
      Success)
        stdout="$(
          aws ssm get-command-invocation \
            --region "$REGION" \
            --command-id "$cmd_id" \
            --instance-id "$instance_id" \
            --query 'StandardOutputContent' \
            --output text
        )"
        printf '%s' "$stdout"
        return 0
        ;;
      Failed|Cancelled|TimedOut|Undeliverable|Terminated)
        stderr="$(
          aws ssm get-command-invocation \
            --region "$REGION" \
            --command-id "$cmd_id" \
            --instance-id "$instance_id" \
            --query 'StandardErrorContent' \
            --output text 2>/dev/null || true
        )"
        if [[ -n "$stderr" ]]; then
          printf '%s\n' "$stderr" >&2
        fi
        return 1
        ;;
      *)
        sleep 5
        ;;
    esac
  done

  echo "timed out waiting for SSM command $cmd_id on $instance_id" >&2
  return 1
}

helper_exec_capture() {
  local cmd_id
  cmd_id="$(ssm_send "$HELPER_INSTANCE_ID" "$@")"
  ssm_wait_capture "$cmd_id" "$HELPER_INSTANCE_ID"
}

target_ssh() {
  ssh \
    -i "$DO_SSH_KEY" \
    -o BatchMode=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "${TARGET_USER}@${TARGET_HOST}" \
    "$@"
}

ensure_helper_running() {
  local state ping
  state="$(
    aws ec2 describe-instances \
      --region "$REGION" \
      --instance-ids "$HELPER_INSTANCE_ID" \
      --query 'Reservations[0].Instances[0].State.Name' \
      --output text
  )"

  if [[ "$state" == "running" ]]; then
    HELPER_WAS_RUNNING=1
  elif [[ "$state" == "stopped" ]]; then
    aws ec2 start-instances --region "$REGION" --instance-ids "$HELPER_INSTANCE_ID" >/dev/null
    aws ec2 wait instance-running --region "$REGION" --instance-ids "$HELPER_INSTANCE_ID"
    HELPER_STARTED_BY_SCRIPT=1
  else
    echo "helper instance is in unsupported state: $state" >&2
    exit 1
  fi

  for _ in {1..60}; do
    ping="$(
      aws ssm describe-instance-information \
        --region "$REGION" \
        --filters "Key=InstanceIds,Values=$HELPER_INSTANCE_ID" \
        --query 'InstanceInformationList[0].PingStatus' \
        --output text 2>/dev/null || true
    )"
    if [[ "$ping" == "Online" ]]; then
      return
    fi
    sleep 5
  done

  echo "helper instance did not become SSM-online: $HELPER_INSTANCE_ID" >&2
  exit 1
}

ensure_do_firewall_rule() {
  local existing firewall_json

  firewall_json="$(doctl --context juno compute firewall get "$DO_FIREWALL_ID" --output json)"
  existing="$(
    FIREWALL_JSON="$firewall_json" \
    python3 - "$HELPER_EGRESS_CIDR" <<'PY'
import json
import os
import sys

cidr = sys.argv[1]
docs = json.loads(os.environ["FIREWALL_JSON"])
rules = docs[0].get("inbound_rules", []) if docs else []
for rule in rules:
    if rule.get("protocol") == "tcp" and rule.get("ports") == "22":
        for address in (rule.get("sources") or {}).get("addresses", []):
            if address == cidr:
                print("yes")
                raise SystemExit(0)
print("no")
PY
  )"

  if [[ "$existing" == "yes" ]]; then
    return
  fi

  doctl --context juno compute firewall add-rules "$DO_FIREWALL_ID" \
    --inbound-rules "protocol:tcp,ports:22,address:${HELPER_EGRESS_CIDR}" \
    >/dev/null
  DO_FIREWALL_RULE_ADDED=1
}

create_snapshot_if_needed() {
  local snapshot_name timestamp
  if [[ -n "$EXISTING_SNAPSHOT_ID" ]]; then
    SNAPSHOT_ID="$EXISTING_SNAPSHOT_ID"
    aws ec2 wait snapshot-completed --region "$REGION" --snapshot-ids "$SNAPSHOT_ID"
    return
  fi

  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  snapshot_name="junopayserver-${SNAPSHOT_KIND}-sync-${timestamp}"

  SNAPSHOT_ID="$(
    aws ec2 create-snapshot \
      --region "$REGION" \
      --volume-id "$SOURCE_VOLUME_ID" \
      --description "junopayserver ${SNAPSHOT_KIND} sync ${timestamp}" \
      --tag-specifications "ResourceType=snapshot,Tags=[{Key=Name,Value=${snapshot_name}},{Key=Project,Value=JunoPayServer},{Key=Purpose,Value=${SNAPSHOT_KIND}-sync}]" \
      --query 'SnapshotId' \
      --output text
  )"
  aws ec2 wait snapshot-completed --region "$REGION" --snapshot-ids "$SNAPSHOT_ID"
}

create_and_attach_temp_volume() {
  local volume_name timestamp
  timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
  volume_name="junopayserver-${SNAPSHOT_KIND}-sync-volume-${timestamp}"

  TEMP_VOLUME_ID="$(
    aws ec2 create-volume \
      --region "$REGION" \
      --availability-zone "$HELPER_AZ" \
      --snapshot-id "$SNAPSHOT_ID" \
      --volume-type gp3 \
      --tag-specifications "ResourceType=volume,Tags=[{Key=Name,Value=${volume_name}},{Key=Project,Value=JunoPayServer},{Key=Purpose,Value=${SNAPSHOT_KIND}-sync-temp}]" \
      --query 'VolumeId' \
      --output text
  )"
  aws ec2 wait volume-available --region "$REGION" --volume-ids "$TEMP_VOLUME_ID"

  aws ec2 attach-volume \
    --region "$REGION" \
    --volume-id "$TEMP_VOLUME_ID" \
    --instance-id "$HELPER_INSTANCE_ID" \
    --device "$HELPER_ATTACH_DEVICE" \
    >/dev/null
  aws ec2 wait volume-in-use --region "$REGION" --volume-ids "$TEMP_VOLUME_ID"
}

run_helper_snapshot_stream() {
  local helper_script_b64 do_ssh_key_b64 cmd_id

  helper_script_b64="$(base64 < "$HELPER_SCRIPT_PATH" | tr -d '\n')"
  do_ssh_key_b64="$(base64 < "$DO_SSH_KEY" | tr -d '\n')"

  cmd_id="$(
    ssm_send "$HELPER_INSTANCE_ID" \
      "printf '%s' '$helper_script_b64' | base64 -d >/tmp/junopayserver-helper-snapshot-stream.sh" \
      "chmod 700 /tmp/junopayserver-helper-snapshot-stream.sh" \
      "export TEMP_VOLUME_ID='$TEMP_VOLUME_ID' TARGET_HOST='$TARGET_HOST' TARGET_USER='$TARGET_USER' TARGET_ROOT='$TARGET_ROOT' HELPER_MOUNT_POINT='$HELPER_MOUNT_POINT' DO_SSH_KEY_B64='$do_ssh_key_b64' SNAPSHOT_KIND='$SNAPSHOT_KIND'; /tmp/junopayserver-helper-snapshot-stream.sh"
  )"
  ssm_wait_capture "$cmd_id" "$HELPER_INSTANCE_ID" >/dev/null
}

stop_target_core_services() {
  target_ssh bash -se -- "$TARGET_DEPLOY_ROOT" <<'EOF'
set -euo pipefail

ROOT="$1"
COMPOSE_PATH="${ROOT}/docker-compose.yml"
RUNTIME_ENV_PATH="${ROOT}/runtime.env"

docker compose --env-file "$RUNTIME_ENV_PATH" -f "$COMPOSE_PATH" stop juno-pay-server juno-scan junocashd >/dev/null
EOF
  TARGET_CORE_STOPPED=1
}

start_target_core_services() {
  target_ssh bash -se -- "$TARGET_DEPLOY_ROOT" <<'EOF'
set -euo pipefail

ROOT="$1"
COMPOSE_PATH="${ROOT}/docker-compose.yml"
RUNTIME_ENV_PATH="${ROOT}/runtime.env"

docker compose --env-file "$RUNTIME_ENV_PATH" -f "$COMPOSE_PATH" up -d junocashd juno-scan juno-pay-server >/dev/null
EOF
  TARGET_POST_SYNC_COMPLETED=1
  TARGET_CORE_STOPPED=0
}

run_target_warm_rebuild() {
  target_ssh bash -se -- --root "$TARGET_DEPLOY_ROOT" < "$WARM_RESET_SCRIPT_PATH"
  TARGET_POST_SYNC_COMPLETED=1
  TARGET_CORE_STOPPED=0
}

detach_and_delete_temp_volume() {
  if [[ -z "$TEMP_VOLUME_ID" ]]; then
    return
  fi

  aws ec2 detach-volume \
    --region "$REGION" \
    --volume-id "$TEMP_VOLUME_ID" \
    --instance-id "$HELPER_INSTANCE_ID" \
    --device "$HELPER_ATTACH_DEVICE" \
    >/dev/null
  aws ec2 wait volume-available --region "$REGION" --volume-ids "$TEMP_VOLUME_ID"
  aws ec2 delete-volume --region "$REGION" --volume-id "$TEMP_VOLUME_ID" >/dev/null
  TEMP_VOLUME_ID=""
}

run_readiness_check_if_requested() {
  local cmd=() readiness_mode
  if [[ -z "$READINESS_SERVICE_TOKEN_FILE" ]]; then
    return
  fi

  readiness_mode="final"
  if [[ "$SNAPSHOT_KIND" == "warm" ]]; then
    readiness_mode="warm"
  fi

  cmd=(
    "$SCRIPT_DIR/../../do/scripts/check-cutover-readiness.sh"
    --mode "$readiness_mode"
    --service-token-file "$READINESS_SERVICE_TOKEN_FILE"
    --target-host "$TARGET_HOST"
    --target-user "$TARGET_USER"
    --target-ssh-key "$DO_SSH_KEY"
    --target-deploy-root "$TARGET_DEPLOY_ROOT"
  )
  if [[ -n "$MERCHANT_API_KEY" ]]; then
    cmd+=(--merchant-api-key "$MERCHANT_API_KEY")
  fi
  "${cmd[@]}"
}

delete_prior_snapshot_if_requested() {
  if [[ -z "$DELETE_SNAPSHOT_ID" ]]; then
    return
  fi
  if [[ "$DELETE_SNAPSHOT_ID" == "$SNAPSHOT_ID" ]]; then
    echo "refusing to delete the just-created snapshot: $DELETE_SNAPSHOT_ID" >&2
    exit 1
  fi
  aws ec2 delete-snapshot --region "$REGION" --snapshot-id "$DELETE_SNAPSHOT_ID" >/dev/null
}

ensure_helper_running
HELPER_EGRESS_CIDR="$(helper_exec_capture "curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]'" | tr -d '[:space:]')/32"
ensure_do_firewall_rule
create_snapshot_if_needed
create_and_attach_temp_volume
stop_target_core_services
run_helper_snapshot_stream
detach_and_delete_temp_volume
if [[ "$SNAPSHOT_KIND" == "warm" ]]; then
  run_target_warm_rebuild
else
  start_target_core_services
fi
run_readiness_check_if_requested
delete_prior_snapshot_if_requested

echo "snapshot_id=$SNAPSHOT_ID"
echo "helper_instance_id=$HELPER_INSTANCE_ID"
echo "helper_egress_cidr=$HELPER_EGRESS_CIDR"
echo "target_host=$TARGET_HOST"
