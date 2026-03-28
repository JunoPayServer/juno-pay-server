#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage:
  sync-state-stream.sh --source-host <aws-host> --target-host <do-host> [options]

Options:
  --source-user <user>   Source SSH user. Default: ec2-user
  --target-user <user>   Target SSH user. Default: root
  --source-root <path>   Source state root. Default: /opt/juno-pay/data
  --target-root <path>   Target state root. Default: /opt/juno-pay/data
  --ssh-key <path>       SSH private key used for both hosts
EOF
}

SOURCE_USER="ec2-user"
TARGET_USER="root"
SOURCE_ROOT="/opt/juno-pay/data"
TARGET_ROOT="/opt/juno-pay/data"
SOURCE_HOST=""
TARGET_HOST=""
SSH_KEY=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --source-host)
      SOURCE_HOST="${2:-}"
      shift 2
      ;;
    --target-host)
      TARGET_HOST="${2:-}"
      shift 2
      ;;
    --source-user)
      SOURCE_USER="${2:-}"
      shift 2
      ;;
    --target-user)
      TARGET_USER="${2:-}"
      shift 2
      ;;
    --source-root)
      SOURCE_ROOT="${2:-}"
      shift 2
      ;;
    --target-root)
      TARGET_ROOT="${2:-}"
      shift 2
      ;;
    --ssh-key)
      SSH_KEY="${2:-}"
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

if [[ -z "${SOURCE_HOST}" || -z "${TARGET_HOST}" ]]; then
  usage >&2
  exit 2
fi

SSH_OPTS=(
  -o BatchMode=yes
  -o StrictHostKeyChecking=accept-new
)

if [[ -n "${SSH_KEY}" ]]; then
  SSH_OPTS+=(-i "${SSH_KEY}")
fi

DIRS=(
  junocashd
  juno-scan
  juno-pay-server
)

SOURCE_PATHS=()
for dir in "${DIRS[@]}"; do
  SOURCE_PATHS+=("${dir}")
done

ssh "${SSH_OPTS[@]}" "${TARGET_USER}@${TARGET_HOST}" "mkdir -p '${TARGET_ROOT}'"

ssh "${SSH_OPTS[@]}" "${SOURCE_USER}@${SOURCE_HOST}" \
  "tar -C '${SOURCE_ROOT}' --numeric-owner -cpf - ${SOURCE_PATHS[*]}" \
  | ssh "${SSH_OPTS[@]}" "${TARGET_USER}@${TARGET_HOST}" \
      "tar -C '${TARGET_ROOT}' --numeric-owner -xpf -"

echo "Synced ${DIRS[*]} from ${SOURCE_HOST} to ${TARGET_HOST}"
