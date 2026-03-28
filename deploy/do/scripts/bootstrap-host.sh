#!/usr/bin/env bash
set -euo pipefail

ROOT="${ROOT:-/opt/juno-pay}"
DATA_DIR="${DATA_DIR:-${ROOT}/data}"
DO_VOLUME_LABEL="${DO_VOLUME_LABEL:-junopaydata}"
DO_VOLUME_NAME="${DO_VOLUME_NAME:-junopayserver-data}"

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y --no-install-recommends \
  ca-certificates \
  curl \
  docker.io \
  e2fsprogs \
  python3 \
  rsync

if ! docker compose version >/dev/null 2>&1; then
  if apt-cache show docker-compose-v2 >/dev/null 2>&1; then
    apt-get install -y --no-install-recommends docker-compose-v2
  fi
fi

if ! docker compose version >/dev/null 2>&1; then
  COMPOSE_VERSION="v2.27.0"
  DEST="/usr/local/lib/docker/cli-plugins"
  mkdir -p "${DEST}"
  curl -fsSLo "${DEST}/docker-compose" \
    "https://github.com/docker/compose/releases/download/${COMPOSE_VERSION}/docker-compose-linux-x86_64"
  chmod +x "${DEST}/docker-compose"
fi

systemctl enable docker
systemctl restart docker

mkdir -p "${ROOT}" "${DATA_DIR}"

DEVICE="$(blkid -L "${DO_VOLUME_LABEL}" || true)"
if [[ -z "${DEVICE}" ]]; then
  DEVICE="$(readlink -f "/dev/disk/by-id/scsi-0DO_Volume_${DO_VOLUME_NAME}" 2>/dev/null || true)"
fi
if [[ -z "${DEVICE}" ]]; then
  echo "volume device not found for label ${DO_VOLUME_LABEL} or name ${DO_VOLUME_NAME}" >&2
  exit 1
fi

UUID="$(blkid -o value -s UUID "${DEVICE}")"
if [[ -z "${UUID}" ]]; then
  echo "unable to determine UUID for ${DEVICE}" >&2
  exit 1
fi
if ! grep -q "UUID=${UUID} ${DATA_DIR} " /etc/fstab; then
  echo "UUID=${UUID} ${DATA_DIR} ext4 defaults,nofail,discard 0 2" >> /etc/fstab
fi
if ! mountpoint -q "${DATA_DIR}"; then
  mount "${DATA_DIR}"
fi

mkdir -p \
  "${DATA_DIR}/junocashd" \
  "${DATA_DIR}/juno-scan" \
  "${DATA_DIR}/juno-pay-server" \
  "${DATA_DIR}/caddy/data" \
  "${DATA_DIR}/caddy/config"

chown -R 10001:65534 "${DATA_DIR}/juno-pay-server" || true
chmod 700 "${ROOT}"

echo "OK"
