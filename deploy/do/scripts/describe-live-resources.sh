#!/usr/bin/env bash
set -euo pipefail

DOCTL_CONTEXT="${DOCTL_CONTEXT:-juno}"
PROJECT_NAME="${PROJECT_NAME:-junopayserver}"
DROPLET_NAME="${DROPLET_NAME:-junopayserver-prod}"
VOLUME_NAME="${VOLUME_NAME:-junopayserver-data}"
FIREWALL_NAME="${FIREWALL_NAME:-junopayserver-fw}"

PROJECT_ID="$(doctl --context "${DOCTL_CONTEXT}" projects list --format ID,Name --no-header | awk -v name="${PROJECT_NAME}" '$2==name { print $1 }')"
if [[ -z "${PROJECT_ID}" ]]; then
  echo "project not found: ${PROJECT_NAME}" >&2
  exit 1
fi

echo "project:"
doctl --context "${DOCTL_CONTEXT}" projects get "${PROJECT_ID}" --format ID,Name,Purpose,Environment,IsDefault

echo
echo "droplet:"
doctl --context "${DOCTL_CONTEXT}" compute droplet list --format ID,Name,PublicIPv4,PrivateIPv4,Status,Tags --no-header | awk -v name="${DROPLET_NAME}" '$2==name'

echo
echo "volume:"
doctl --context "${DOCTL_CONTEXT}" compute volume list --format ID,Name,Size,Region,DropletIDs,Tags --no-header | awk -v name="${VOLUME_NAME}" '$2==name'

echo
echo "firewall:"
doctl --context "${DOCTL_CONTEXT}" compute firewall list --format ID,Name,DropletIDs --no-header | awk -v name="${FIREWALL_NAME}" '$2==name'

echo
echo "reserved IPs:"
doctl --context "${DOCTL_CONTEXT}" compute reserved-ip list --format IP,DropletID,ProjectID,DropletName --no-header | awk -v name="${DROPLET_NAME}" '$4==name'
