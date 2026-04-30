#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: bootstrap-tfstate.sh --region REGION --bucket BUCKET --table TABLE

Creates (if missing) the S3 bucket + DynamoDB lock table used for Terraform remote state.
EOF
  exit 2
}

REGION=""
BUCKET=""
TABLE=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --region) REGION="$2"; shift 2 ;;
    --bucket) BUCKET="$2"; shift 2 ;;
    --table) TABLE="$2"; shift 2 ;;
    *) usage ;;
  esac
done

if [[ -z "${REGION}" || -z "${BUCKET}" || -z "${TABLE}" ]]; then
  usage
fi

create_bucket() {
  if [[ "${REGION}" == "us-east-1" ]]; then
    aws s3api create-bucket --region "${REGION}" --bucket "${BUCKET}" >/dev/null
  else
    aws s3api create-bucket \
      --region "${REGION}" \
      --bucket "${BUCKET}" \
      --create-bucket-configuration "LocationConstraint=${REGION}" >/dev/null
  fi
}

if aws s3api head-bucket --bucket "${BUCKET}" >/dev/null 2>&1; then
  :
else
  create_bucket
fi

aws s3api put-public-access-block --region "${REGION}" --bucket "${BUCKET}" --public-access-block-configuration \
  BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true >/dev/null

aws s3api put-bucket-versioning --region "${REGION}" --bucket "${BUCKET}" --versioning-configuration Status=Enabled >/dev/null

aws s3api put-bucket-encryption --region "${REGION}" --bucket "${BUCKET}" --server-side-encryption-configuration \
  '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}' >/dev/null

if aws dynamodb describe-table --region "${REGION}" --table-name "${TABLE}" >/dev/null 2>&1; then
  :
else
  aws dynamodb create-table \
    --region "${REGION}" \
    --table-name "${TABLE}" \
    --attribute-definitions AttributeName=LockID,AttributeType=S \
    --key-schema AttributeName=LockID,KeyType=HASH \
    --billing-mode PAY_PER_REQUEST \
    --sse-specification Enabled=true >/dev/null

  aws dynamodb wait table-exists --region "${REGION}" --table-name "${TABLE}"
fi

aws dynamodb update-continuous-backups \
  --region "${REGION}" \
  --table-name "${TABLE}" \
  --point-in-time-recovery-specification PointInTimeRecoveryEnabled=true >/dev/null 2>&1 || true

echo "OK"

