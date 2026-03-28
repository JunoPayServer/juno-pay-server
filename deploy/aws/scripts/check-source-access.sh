#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  check-source-access.sh --instance-id <id> --security-group-id <sg> --instance-ip <ip> [options]

Options:
  --region <region>          AWS region. Default: us-east-1
  --ssh-user <user>          SSH user to test. Default: ec2-user
  --ssh-private-key <path>   Private key to use for SSH verification
  --operator-cidr <cidr>     Operator CIDR for temporary port 22 exposure
  --use-ec2-instance-connect Publish a one-time EC2 Instance Connect key before SSH
  --skip-ssh                 Skip direct SSH verification
  --leave-ssh-open           Keep the temporary port 22 rule after the script exits
EOF
}

INSTANCE_ID=""
SECURITY_GROUP_ID=""
INSTANCE_IP=""
REGION="us-east-1"
SSH_USER="ec2-user"
SSH_PRIVATE_KEY=""
OPERATOR_CIDR=""
USE_EIC=0
SKIP_SSH=0
LEAVE_SSH_OPEN=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --instance-id)
      INSTANCE_ID="${2:-}"
      shift 2
      ;;
    --security-group-id)
      SECURITY_GROUP_ID="${2:-}"
      shift 2
      ;;
    --instance-ip)
      INSTANCE_IP="${2:-}"
      shift 2
      ;;
    --region)
      REGION="${2:-}"
      shift 2
      ;;
    --ssh-user)
      SSH_USER="${2:-}"
      shift 2
      ;;
    --ssh-private-key)
      SSH_PRIVATE_KEY="${2:-}"
      shift 2
      ;;
    --operator-cidr)
      OPERATOR_CIDR="${2:-}"
      shift 2
      ;;
    --use-ec2-instance-connect)
      USE_EIC=1
      shift
      ;;
    --skip-ssh)
      SKIP_SSH=1
      shift
      ;;
    --leave-ssh-open)
      LEAVE_SSH_OPEN=1
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

if [[ -z "$INSTANCE_ID" || -z "$SECURITY_GROUP_ID" || -z "$INSTANCE_IP" ]]; then
  usage >&2
  exit 2
fi

if [[ "$SKIP_SSH" == "0" && -z "$SSH_PRIVATE_KEY" ]]; then
  echo "--ssh-private-key is required unless --skip-ssh is set" >&2
  exit 2
fi

if [[ -n "$SSH_PRIVATE_KEY" && ! -f "$SSH_PRIVATE_KEY" ]]; then
  echo "SSH private key not found: $SSH_PRIVATE_KEY" >&2
  exit 2
fi

INGRESS_ADDED=0
SSH_RESULT="not-run"
SSM_RESULT="not-run"

cleanup() {
  if [[ "$INGRESS_ADDED" == "1" && "$LEAVE_SSH_OPEN" == "0" ]]; then
    aws ec2 revoke-security-group-ingress \
      --region "$REGION" \
      --group-id "$SECURITY_GROUP_ID" \
      --ip-permissions "IpProtocol=tcp,FromPort=22,ToPort=22,IpRanges=[{CidrIp=$OPERATOR_CIDR}]" \
      >/dev/null || true
  fi
}

trap cleanup EXIT

if [[ -z "$OPERATOR_CIDR" ]]; then
  OPERATOR_IP="$(curl -fsS https://checkip.amazonaws.com | tr -d '[:space:]')"
  OPERATOR_CIDR="${OPERATOR_IP}/32"
fi

ensure_temp_ssh_rule() {
  local out rc
  set +e
  out="$(aws ec2 authorize-security-group-ingress \
    --region "$REGION" \
    --group-id "$SECURITY_GROUP_ID" \
    --ip-permissions "IpProtocol=tcp,FromPort=22,ToPort=22,IpRanges=[{CidrIp=$OPERATOR_CIDR,Description=Temporary source access check}]" 2>&1)"
  rc=$?
  set -e
  if [[ "$rc" == "0" ]]; then
    INGRESS_ADDED=1
    return
  fi
  if grep -q "InvalidPermission.Duplicate" <<<"$out"; then
    return
  fi
  echo "$out" >&2
  exit "$rc"
}

run_ssm_probe() {
  local ping_status cmd_id status stdout
  ping_status="$(
    aws ssm describe-instance-information \
      --region "$REGION" \
      --filters "Key=InstanceIds,Values=$INSTANCE_ID" \
      --query 'InstanceInformationList[0].PingStatus' \
      --output text 2>/dev/null || true
  )"

  if [[ "$ping_status" != "Online" ]]; then
    SSM_RESULT="offline"
    return
  fi

  cmd_id="$(
    aws ssm send-command \
      --region "$REGION" \
      --instance-ids "$INSTANCE_ID" \
      --document-name AWS-RunShellScript \
      --parameters 'commands=["id -un","hostname","test -d /opt/juno-pay/data && echo DATA_OK"]' \
      --query 'Command.CommandId' \
      --output text
  )"

  for _ in {1..20}; do
    status="$(
      aws ssm get-command-invocation \
        --region "$REGION" \
        --command-id "$cmd_id" \
        --instance-id "$INSTANCE_ID" \
        --query 'Status' \
        --output text 2>/dev/null || true
    )"
    case "$status" in
      Success)
        stdout="$(
          aws ssm get-command-invocation \
            --region "$REGION" \
            --command-id "$cmd_id" \
            --instance-id "$INSTANCE_ID" \
            --query 'StandardOutputContent' \
            --output text
        )"
        if grep -q "DATA_OK" <<<"$stdout"; then
          SSM_RESULT="verified"
        else
          SSM_RESULT="missing-data-root"
        fi
        return
        ;;
      Failed|Cancelled|TimedOut|Undeliverable|Terminated)
        SSM_RESULT="${status,,}"
        return
        ;;
      *)
        sleep 3
        ;;
    esac
  done

  SSM_RESULT="timed-out"
}

run_ssh_probe() {
  local az pubkey_file out rc

  ensure_temp_ssh_rule

  if [[ "$USE_EIC" == "1" ]]; then
    az="$(
      aws ec2 describe-instances \
        --region "$REGION" \
        --instance-ids "$INSTANCE_ID" \
        --query 'Reservations[0].Instances[0].Placement.AvailabilityZone' \
        --output text
    )"
    pubkey_file="$(mktemp)"
    ssh-keygen -y -f "$SSH_PRIVATE_KEY" >"$pubkey_file"
    aws ec2-instance-connect send-ssh-public-key \
      --region "$REGION" \
      --instance-id "$INSTANCE_ID" \
      --availability-zone "$az" \
      --instance-os-user "$SSH_USER" \
      --ssh-public-key "file://$pubkey_file" \
      >/dev/null
    rm -f "$pubkey_file"
  fi

  set +e
  out="$(
    ssh \
      -o BatchMode=yes \
      -o StrictHostKeyChecking=accept-new \
      -o ConnectTimeout=10 \
      -i "$SSH_PRIVATE_KEY" \
      "$SSH_USER@$INSTANCE_IP" \
      'id -un; hostname; test -d /opt/juno-pay/data && echo DATA_OK' 2>&1
  )"
  rc=$?
  set -e

  if [[ "$rc" == "0" ]] && grep -q "DATA_OK" <<<"$out"; then
    SSH_RESULT="verified"
    return
  fi

  if [[ "$rc" != "0" ]]; then
    SSH_RESULT="failed"
    printf '%s\n' "$out" >&2
    return
  fi

  SSH_RESULT="missing-data-root"
}

run_ssm_probe

if [[ "$SKIP_SSH" == "0" ]]; then
  run_ssh_probe
fi

echo "instance_id=$INSTANCE_ID"
echo "region=$REGION"
echo "operator_cidr=$OPERATOR_CIDR"
echo "ssm_result=$SSM_RESULT"
echo "ssh_result=$SSH_RESULT"
echo "ssh_rule_added=$INGRESS_ADDED"

if [[ "$SSM_RESULT" == "verified" || "$SSH_RESULT" == "verified" ]]; then
  exit 0
fi

exit 1
