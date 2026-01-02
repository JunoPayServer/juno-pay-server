#!/usr/bin/env bash
set -euo pipefail

usage() {
  echo "usage: $0 --region REGION --admin-password-param NAME --token-key-param NAME [--pay-store-dsn-param NAME]" >&2
  exit 2
}

REGION=""
ADMIN_PARAM=""
TOKEN_PARAM=""
PAY_STORE_PARAM=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --region) REGION="$2"; shift 2 ;;
    --admin-password-param) ADMIN_PARAM="$2"; shift 2 ;;
    --token-key-param) TOKEN_PARAM="$2"; shift 2 ;;
    --pay-store-dsn-param) PAY_STORE_PARAM="$2"; shift 2 ;;
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
PAY_STORE_DSN=""
if [[ -n "${PAY_STORE_PARAM}" ]]; then
  read -r -s -p "Pay store DSN: " PAY_STORE_DSN
  echo
  if [[ -z "${PAY_STORE_DSN}" ]]; then
    echo "pay store DSN cannot be empty when --pay-store-dsn-param is set" >&2
    exit 2
  fi
fi

aws ssm put-parameter --region "${REGION}" --name "${ADMIN_PARAM}" --type SecureString --overwrite --value "${ADMIN_PASSWORD}" >/dev/null
aws ssm put-parameter --region "${REGION}" --name "${TOKEN_PARAM}" --type SecureString --overwrite --value "${TOKEN_KEY_HEX}" >/dev/null
if [[ -n "${PAY_STORE_PARAM}" ]]; then
  aws ssm put-parameter --region "${REGION}" --name "${PAY_STORE_PARAM}" --type SecureString --overwrite --value "${PAY_STORE_DSN}" >/dev/null
fi

echo "OK"
