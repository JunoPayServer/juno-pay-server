#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --region REGION --admin-password-param NAME --token-key-param NAME" >&2
  exit 2
}

REGION=""
ADMIN_PARAM=""
TOKEN_PARAM=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --region) REGION="$2"; shift 2 ;;
    --admin-password-param) ADMIN_PARAM="$2"; shift 2 ;;
    --token-key-param) TOKEN_PARAM="$2"; shift 2 ;;
    *) usage ;;
  esac
done

if [[ -z "${REGION}" || -z "${ADMIN_PARAM}" || -z "${TOKEN_PARAM}" ]]; then
  usage
fi

read -r -s -p "Admin password: " ADMIN_PASSWORD
echo
read -r -s -p "Token key hex (32-byte hex): " TOKEN_KEY_HEX
echo

aws ssm put-parameter --region "${REGION}" --name "${ADMIN_PARAM}" --type SecureString --overwrite --value "${ADMIN_PASSWORD}" >/dev/null
aws ssm put-parameter --region "${REGION}" --name "${TOKEN_PARAM}" --type SecureString --overwrite --value "${TOKEN_KEY_HEX}" >/dev/null

echo "OK"

