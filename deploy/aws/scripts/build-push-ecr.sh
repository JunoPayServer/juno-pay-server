#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: build-push-ecr.sh --region REGION [--name-prefix PREFIX] [--tag TAG] [--juno-scan-repo URL] [--juno-scan-ref REF]

Builds and pushes Docker images to ECR:
  - juno-pay-server
  - junocashd
  - juno-scan
  - juno-demo-app
EOF
  exit 2
}

REGION=""
NAME_PREFIX="juno-pay"
TAG=""
JUNO_SCAN_REPO="https://github.com/Abdullah1738/juno-scan.git"
JUNO_SCAN_REF="main"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --region) REGION="$2"; shift 2 ;;
    --name-prefix) NAME_PREFIX="$2"; shift 2 ;;
    --tag) TAG="$2"; shift 2 ;;
    --juno-scan-repo) JUNO_SCAN_REPO="$2"; shift 2 ;;
    --juno-scan-ref) JUNO_SCAN_REF="$2"; shift 2 ;;
    *) usage ;;
  esac
done

if [[ -z "${REGION}" ]]; then
  usage
fi

if [[ -z "${TAG}" ]]; then
  TAG="$(git rev-parse --short HEAD)"
fi

ACCOUNT_ID="$(aws sts get-caller-identity --region "${REGION}" --query Account --output text)"
REGISTRY="${ACCOUNT_ID}.dkr.ecr.${REGION}.amazonaws.com"

aws ecr get-login-password --region "${REGION}" | docker login --username AWS --password-stdin "${REGISTRY}"

ensure_repo() {
  local repo="$1"
  if aws ecr describe-repositories --region "${REGION}" --repository-names "${repo}" >/dev/null 2>&1; then
    return 0
  fi
  aws ecr create-repository --region "${REGION}" --repository-name "${repo}" >/dev/null
}

PAY_REPO="${NAME_PREFIX}-juno-pay-server"
JUNOCASHD_REPO="${NAME_PREFIX}-junocashd"
SCAN_REPO_NAME="${NAME_PREFIX}-juno-scan"
DEMO_REPO="${NAME_PREFIX}-juno-demo-app"

ensure_repo "${PAY_REPO}"
ensure_repo "${JUNOCASHD_REPO}"
ensure_repo "${SCAN_REPO_NAME}"
ensure_repo "${DEMO_REPO}"

PAY_IMAGE="${REGISTRY}/${PAY_REPO}:${TAG}"
JUNOCASHD_IMAGE="${REGISTRY}/${JUNOCASHD_REPO}:${TAG}"
SCAN_IMAGE="${REGISTRY}/${SCAN_REPO_NAME}:${TAG}"
DEMO_IMAGE="${REGISTRY}/${DEMO_REPO}:${TAG}"

docker buildx build --platform linux/amd64 --push -t "${PAY_IMAGE}" -f Dockerfile .
docker buildx build --platform linux/amd64 --push -t "${JUNOCASHD_IMAGE}" -f docker/junocashd/Dockerfile .
docker buildx build --platform linux/amd64 --push -t "${SCAN_IMAGE}" \
  --build-arg "JUNO_SCAN_REPO=${JUNO_SCAN_REPO}" \
  --build-arg "JUNO_SCAN_REF=${JUNO_SCAN_REF}" \
  -f docker/juno-scan/Dockerfile .
docker buildx build --platform linux/amd64 --push -t "${DEMO_IMAGE}" -f docker/demo-app/Dockerfile .

cat <<EOF
OK

image_juno_pay_server = "${PAY_IMAGE}"
image_junocashd       = "${JUNOCASHD_IMAGE}"
image_juno_scan       = "${SCAN_IMAGE}"
image_demo_app        = "${DEMO_IMAGE}"
EOF
