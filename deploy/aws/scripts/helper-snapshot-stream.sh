#!/usr/bin/env bash
set -euo pipefail

: "${TEMP_VOLUME_ID:?TEMP_VOLUME_ID is required}"
: "${TARGET_HOST:?TARGET_HOST is required}"
: "${TARGET_USER:?TARGET_USER is required}"
: "${TARGET_ROOT:?TARGET_ROOT is required}"
: "${DO_SSH_KEY_B64:?DO_SSH_KEY_B64 is required}"
: "${SNAPSHOT_KIND:?SNAPSHOT_KIND is required}"

HELPER_MOUNT_POINT="${HELPER_MOUNT_POINT:-/mnt/juno-pay-source}"
DO_SSH_KEY_PATH="${DO_SSH_KEY_PATH:-/root/.ssh/junopayserver-snapshot-sync}"
SOURCE_DIRS=()

case "$SNAPSHOT_KIND" in
  warm)
    SOURCE_DIRS=(
      juno-pay-server
    )
    ;;
  cold)
    SOURCE_DIRS=(
      juno-pay-server
    )
    ;;
  *)
    echo "unsupported snapshot kind: $SNAPSHOT_KIND" >&2
    exit 2
    ;;
esac

MOUNTED=0

cleanup() {
  if [[ "$MOUNTED" == "1" ]]; then
    umount "$HELPER_MOUNT_POINT" >/dev/null 2>&1 || true
  fi
  rm -f "$DO_SSH_KEY_PATH"
}

trap cleanup EXIT

find_device_for_volume() {
  local volume_id_noprefix dev_by_id_1 dev_by_id_2 dev

  volume_id_noprefix="${TEMP_VOLUME_ID#vol-}"
  dev_by_id_1="/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_${TEMP_VOLUME_ID}"
  dev_by_id_2="/dev/disk/by-id/nvme-Amazon_Elastic_Block_Store_vol${volume_id_noprefix}"
  dev=""

  for _ in {1..90}; do
    if [[ -b "$dev_by_id_1" ]]; then
      dev="$dev_by_id_1"
      break
    fi
    if [[ -b "$dev_by_id_2" ]]; then
      dev="$dev_by_id_2"
      break
    fi
    sleep 2
  done

  if [[ -z "$dev" ]]; then
    echo "unable to resolve device for $TEMP_VOLUME_ID" >&2
    ls -l /dev/disk/by-id >&2 || true
    exit 1
  fi

  readlink -f "$dev"
}

mount_snapshot_volume() {
  local dev fstype

  dev="$(find_device_for_volume)"
  fstype="$(blkid -o value -s TYPE "$dev" || true)"

  mkdir -p "$HELPER_MOUNT_POINT"
  if [[ "$fstype" == ext2 || "$fstype" == ext3 || "$fstype" == ext4 ]]; then
    mount -o ro,noload "$dev" "$HELPER_MOUNT_POINT"
  else
    mount -o ro "$dev" "$HELPER_MOUNT_POINT"
  fi
  MOUNTED=1
}

write_temp_ssh_key() {
  mkdir -p "$(dirname "$DO_SSH_KEY_PATH")"
  umask 077
  printf '%s' "$DO_SSH_KEY_B64" | base64 -d >"$DO_SSH_KEY_PATH"
  chmod 600 "$DO_SSH_KEY_PATH"
}

stream_to_target() {
  local ssh_opts=()

  ssh_opts=(
    -i "$DO_SSH_KEY_PATH"
    -o BatchMode=yes
    -o StrictHostKeyChecking=accept-new
    -o ConnectTimeout=10
  )

  for dir in "${SOURCE_DIRS[@]}"; do
    if [[ ! -e "$HELPER_MOUNT_POINT/$dir" ]]; then
      echo "missing expected source directory: $HELPER_MOUNT_POINT/$dir" >&2
      exit 1
    fi
  done

  ssh "${ssh_opts[@]}" "${TARGET_USER}@${TARGET_HOST}" "mkdir -p '$TARGET_ROOT'"

  tar -C "$HELPER_MOUNT_POINT" --numeric-owner -cpf - "${SOURCE_DIRS[@]}" \
    | ssh "${ssh_opts[@]}" "${TARGET_USER}@${TARGET_HOST}" \
        "tar -C '$TARGET_ROOT' --numeric-owner -xpf -"
}

mount_snapshot_volume
write_temp_ssh_key
stream_to_target
