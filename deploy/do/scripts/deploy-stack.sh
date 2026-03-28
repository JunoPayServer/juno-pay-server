#!/usr/bin/env bash
set -euo pipefail

required_vars=(
  ROOT
  DOMAIN_NAME
  WWW_DOMAIN
  STAGING_DOMAIN
  IMAGE_JUNO_PAY_SERVER
  IMAGE_JUNOCASHD
  IMAGE_JUNO_SCAN
  IMAGE_DEMO_APP
  JUNO_PAY_ADMIN_PASSWORD
  JUNO_PAY_TOKEN_KEY_HEX
)

for var in "${required_vars[@]}"; do
  if [[ -z "${!var:-}" ]]; then
    echo "missing required env: ${var}" >&2
    exit 2
  fi
done

ROOT="${ROOT:-/opt/juno-pay}"
DATA_DIR="${DATA_DIR:-${ROOT}/data}"
RUNTIME_ENV_PATH="${RUNTIME_ENV_PATH:-${ROOT}/runtime.env}"
COMPOSE_PATH="${ROOT}/docker-compose.yml"
CADDYFILE_PATH="${ROOT}/Caddyfile"

JUNO_CHAIN="${JUNO_CHAIN:-mainnet}"
JUNO_SCAN_UA_HRP="${JUNO_SCAN_UA_HRP:-j}"
JUNO_SCAN_CONFIRMATIONS="${JUNO_SCAN_CONFIRMATIONS:-100}"
JUNO_PAY_DEMO_MERCHANT_API_KEY="${JUNO_PAY_DEMO_MERCHANT_API_KEY:-}"
CADDY_SERVER_NAMES="${CADDY_SERVER_NAMES:-${DOMAIN_NAME}, ${WWW_DOMAIN}, ${STAGING_DOMAIN}}"

mkdir -p "${ROOT}" "${DATA_DIR}"

if [[ -n "${GHCR_TOKEN:-}" ]]; then
  if [[ -z "${GHCR_USERNAME:-}" ]]; then
    echo "GHCR_USERNAME is required when GHCR_TOKEN is provided" >&2
    exit 2
  fi
  printf '%s' "${GHCR_TOKEN}" | docker login ghcr.io --username "${GHCR_USERNAME}" --password-stdin
fi

umask 077
cat > "${RUNTIME_ENV_PATH}" <<EOF
IMAGE_JUNO_PAY_SERVER=${IMAGE_JUNO_PAY_SERVER}
IMAGE_JUNOCASHD=${IMAGE_JUNOCASHD}
IMAGE_JUNO_SCAN=${IMAGE_JUNO_SCAN}
IMAGE_DEMO_APP=${IMAGE_DEMO_APP}
JUNO_CHAIN=${JUNO_CHAIN}
JUNO_SCAN_UA_HRP=${JUNO_SCAN_UA_HRP}
JUNO_SCAN_CONFIRMATIONS=${JUNO_SCAN_CONFIRMATIONS}
JUNO_PAY_ADMIN_PASSWORD=${JUNO_PAY_ADMIN_PASSWORD}
JUNO_PAY_TOKEN_KEY_HEX=${JUNO_PAY_TOKEN_KEY_HEX}
JUNO_PAY_DEMO_MERCHANT_API_KEY=${JUNO_PAY_DEMO_MERCHANT_API_KEY}
EOF
chmod 600 "${RUNTIME_ENV_PATH}"

cat > "${COMPOSE_PATH}" <<'EOF'
services:
  junocashd:
    image: ${IMAGE_JUNOCASHD}
    restart: unless-stopped
    environment:
      JUNO_CHAIN: ${JUNO_CHAIN}
      JUNO_DATADIR: /data
      JUNO_RPC_USER: rpcuser
      JUNO_RPC_PASS: rpcpass
      JUNO_RPC_PORT: 8232
    command:
      - -server=1
      - -txindex=1
      - -daemon=0
      - -printtoconsole=1
      - -datadir=/data
      - -rpcbind=0.0.0.0
      - -rpcallowip=0.0.0.0/0
      - -rpcport=8232
      - -rpcuser=rpcuser
      - -rpcpassword=rpcpass
    volumes:
      - /opt/juno-pay/data/junocashd:/data
    healthcheck:
      test: ["CMD", "juno-cli", "getblockcount"]
      interval: 5s
      timeout: 5s
      retries: 60

  juno-scan:
    image: ${IMAGE_JUNO_SCAN}
    restart: unless-stopped
    depends_on:
      junocashd:
        condition: service_healthy
    environment:
      JUNO_SCAN_LISTEN: 0.0.0.0:8080
      JUNO_SCAN_RPC_URL: http://junocashd:8232
      JUNO_SCAN_RPC_USER: rpcuser
      JUNO_SCAN_RPC_PASS: rpcpass
      JUNO_SCAN_UA_HRP: ${JUNO_SCAN_UA_HRP}
      JUNO_SCAN_CONFIRMATIONS: ${JUNO_SCAN_CONFIRMATIONS}
      JUNO_SCAN_DB_DRIVER: rocksdb
      JUNO_SCAN_DB_PATH: /data/db
    volumes:
      - /opt/juno-pay/data/juno-scan:/data
    healthcheck:
      test: ["CMD", "curl", "-fsS", "http://127.0.0.1:8080/v1/health"]
      interval: 5s
      timeout: 2s
      retries: 60

  juno-pay-server:
    image: ${IMAGE_JUNO_PAY_SERVER}
    restart: unless-stopped
    depends_on:
      junocashd:
        condition: service_healthy
      juno-scan:
        condition: service_healthy
    environment:
      JUNO_PAY_ADDR: 0.0.0.0:8080
      JUNO_PAY_ADMIN_PASSWORD: ${JUNO_PAY_ADMIN_PASSWORD}
      JUNO_PAY_TOKEN_KEY_HEX: ${JUNO_PAY_TOKEN_KEY_HEX}
      JUNO_PAY_DATA_DIR: /data
      JUNO_PAY_STORE_DRIVER: sqlite
      JUNO_SCAN_URL: http://juno-scan:8080
      JUNO_CASHD_RPC_URL: http://junocashd:8232
      JUNO_CASHD_RPC_USER: rpcuser
      JUNO_CASHD_RPC_PASS: rpcpass
    volumes:
      - /opt/juno-pay/data/juno-pay-server:/data

  demo-app:
    image: ${IMAGE_DEMO_APP}
    restart: unless-stopped
    depends_on:
      juno-pay-server:
        condition: service_started
    environment:
      JUNO_PAY_BASE_URL: http://juno-pay-server:8080
      JUNO_PAY_MERCHANT_API_KEY: ${JUNO_PAY_DEMO_MERCHANT_API_KEY}

  caddy:
    image: caddy:2.7.6
    restart: unless-stopped
    depends_on:
      juno-pay-server:
        condition: service_started
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - /opt/juno-pay/Caddyfile:/etc/caddy/Caddyfile:ro
      - /opt/juno-pay/data/caddy/data:/data
      - /opt/juno-pay/data/caddy/config:/config
EOF

cat > "${CADDYFILE_PATH}" <<EOF
${CADDY_SERVER_NAMES} {
  encode zstd gzip

  @backend path /admin* /v1/*
  handle @backend {
    reverse_proxy juno-pay-server:8080
  }

  handle {
    reverse_proxy demo-app:3000
  }
}
EOF

docker compose --env-file "${RUNTIME_ENV_PATH}" -f "${COMPOSE_PATH}" config >/dev/null
docker compose --env-file "${RUNTIME_ENV_PATH}" -f "${COMPOSE_PATH}" pull
docker compose --env-file "${RUNTIME_ENV_PATH}" -f "${COMPOSE_PATH}" up -d --remove-orphans

echo "OK"
