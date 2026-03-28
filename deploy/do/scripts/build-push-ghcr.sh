#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
usage: build-push-ghcr.sh --owner OWNER [--image-prefix PREFIX] [--tag TAG] [--stable-tag TAG] [--juno-scan-repo URL] [--juno-scan-ref REF]

Builds and pushes Docker images to GHCR:
  - juno-pay-server
  - junocashd
  - juno-scan
  - juno-demo-app

The caller must already be logged in to ghcr.io.
EOF
  exit 2
}

OWNER=""
IMAGE_PREFIX="juno-pay"
TAG=""
STABLE_TAG="prod"
JUNO_SCAN_REPO="https://github.com/junocash-tools/juno-scan.git"
JUNO_SCAN_REF="8e40d26577be1e946823ab3f380be5baf8a1dccd"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --owner) OWNER="$2"; shift 2 ;;
    --image-prefix) IMAGE_PREFIX="$2"; shift 2 ;;
    --tag) TAG="$2"; shift 2 ;;
    --stable-tag) STABLE_TAG="$2"; shift 2 ;;
    --juno-scan-repo) JUNO_SCAN_REPO="$2"; shift 2 ;;
    --juno-scan-ref) JUNO_SCAN_REF="$2"; shift 2 ;;
    *) usage ;;
  esac
done

if [[ -z "${OWNER}" ]]; then
  usage
fi

if [[ -z "${TAG}" ]]; then
  TAG="$(git rev-parse --short HEAD)"
fi

OWNER="$(printf '%s' "${OWNER}" | tr '[:upper:]' '[:lower:]')"
REGISTRY="ghcr.io/${OWNER}"

PAY_REPO="${IMAGE_PREFIX}-juno-pay-server"
JUNOCASHD_REPO="${IMAGE_PREFIX}-junocashd"
SCAN_REPO_NAME="${IMAGE_PREFIX}-juno-scan"
DEMO_REPO="${IMAGE_PREFIX}-juno-demo-app"

PAY_IMAGE="${REGISTRY}/${PAY_REPO}:${TAG}"
JUNOCASHD_IMAGE="${REGISTRY}/${JUNOCASHD_REPO}:${TAG}"
SCAN_IMAGE="${REGISTRY}/${SCAN_REPO_NAME}:${TAG}"
DEMO_IMAGE="${REGISTRY}/${DEMO_REPO}:${TAG}"

PAY_IMAGE_STABLE="${REGISTRY}/${PAY_REPO}:${STABLE_TAG}"
JUNOCASHD_IMAGE_STABLE="${REGISTRY}/${JUNOCASHD_REPO}:${STABLE_TAG}"
SCAN_IMAGE_STABLE="${REGISTRY}/${SCAN_REPO_NAME}:${STABLE_TAG}"
DEMO_IMAGE_STABLE="${REGISTRY}/${DEMO_REPO}:${STABLE_TAG}"

PAY_TAGS=(-t "${PAY_IMAGE}")
JUNOCASHD_TAGS=(-t "${JUNOCASHD_IMAGE}")
SCAN_TAGS=(-t "${SCAN_IMAGE}")
DEMO_TAGS=(-t "${DEMO_IMAGE}")
if [[ "${TAG}" != "${STABLE_TAG}" ]]; then
  PAY_TAGS+=(-t "${PAY_IMAGE_STABLE}")
  JUNOCASHD_TAGS+=(-t "${JUNOCASHD_IMAGE_STABLE}")
  SCAN_TAGS+=(-t "${SCAN_IMAGE_STABLE}")
  DEMO_TAGS+=(-t "${DEMO_IMAGE_STABLE}")
fi

docker buildx build --platform linux/amd64 --push "${PAY_TAGS[@]}" -f Dockerfile .
docker buildx build --platform linux/amd64 --push "${JUNOCASHD_TAGS[@]}" -f docker/junocashd/Dockerfile .
docker buildx build --platform linux/amd64 --push "${SCAN_TAGS[@]}" \
  --build-arg "JUNO_SCAN_REPO=${JUNO_SCAN_REPO}" \
  --build-arg "JUNO_SCAN_REF=${JUNO_SCAN_REF}" \
  -f docker/juno-scan/Dockerfile .
docker buildx build --platform linux/amd64 --push "${DEMO_TAGS[@]}" -f docker/demo-app/Dockerfile .

printf "IMAGE_JUNO_PAY_SERVER=%q\n" "${PAY_IMAGE}"
printf "IMAGE_JUNOCASHD=%q\n" "${JUNOCASHD_IMAGE}"
printf "IMAGE_JUNO_SCAN=%q\n" "${SCAN_IMAGE}"
printf "IMAGE_DEMO_APP=%q\n" "${DEMO_IMAGE}"
