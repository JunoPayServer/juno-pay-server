# juno-pay-server

Self-hosted payment backend for the Juno Cash ecosystem.

This service is designed for **Orchard-only / shielded-by-default** chains:
deposit detection is done via `juno-scan` note scanning (trial-decrypt with UFVK), not by parsing transparent outputs.

## Components

- [`junocashd`](https://github.com/juno-cash/junocash/releases): full node (consensus + validated block source).
- [`juno-scan`](https://github.com/Abdullah1738/juno-scan): watch-only scanner/indexer (UFVK → deposit events).
- `juno-pay-server` (this repo): merchant config + invoices + durable event delivery (webhook/brokers).
- Admin dashboard: served by `juno-pay-server` at `GET /admin/` (static export).
- Demo checkout UI: `demo-app/` (localStorage-only; can be hosted separately).

Canonical API schema: `api/openapi.yaml`.

## Quickstart (Docker Compose)

1) Create `.env`:

```bash
cp .env.example .env
./scripts/gen-token-key-hex.sh
```

Set:
- `JUNO_PAY_ADMIN_PASSWORD`
- `JUNO_PAY_TOKEN_KEY_HEX` (32-byte hex)

2) Start stack:

```bash
docker compose up -d --build
```

3) Open admin UI:

- `http://127.0.0.1:${JUNO_PAY_PORT_HOST:-18082}/admin/`

## Configuration

### `juno-pay-server` env vars

- `JUNO_PAY_ADDR` (default `127.0.0.1:8080`): HTTP listen address.
- `JUNO_PAY_ADMIN_PASSWORD` (**required**): admin login password (cookie-based session).
- `JUNO_PAY_ADMIN_UI_DIR` (optional): directory containing exported admin UI to serve under `/admin/`.
  - If unset, the server serves the embedded UI (release builds).
- `JUNO_PAY_STORE_DRIVER` (default `sqlite`): `sqlite|postgres|mysql|mongo`.
- `JUNO_PAY_STORE_DSN` (required for `postgres|mysql|mongo`): connection string / URI.
- `JUNO_PAY_STORE_DB` (required for `mongo`): database name.
- `JUNO_PAY_STORE_PREFIX` (optional): table/collection prefix (useful when sharing a DB with other apps).
- `JUNO_PAY_DATA_DIR` (default `~/.juno-pay-server`): data directory for embedded SQLite (ignored for non-sqlite drivers).
- `JUNO_PAY_TOKEN_KEY_HEX` (**required**): 32-byte hex key used to encrypt invoice checkout tokens in the DB.
- `JUNO_SCAN_URL` (**required**): base URL of `juno-scan` (example `http://127.0.0.1:18080`).
- `JUNO_CASHD_RPC_URL` (default `http://127.0.0.1:8232`): `junocashd` RPC URL.
- `JUNO_CASHD_RPC_USER` / `JUNO_CASHD_RPC_PASS`: `junocashd` RPC auth (empty → cookie/rpcauth setups are also possible if you front it yourself).
- `JUNO_PAY_SCAN_POLL_MS` (default `1000`): poll interval for ingesting `juno-scan` events.
- `JUNO_PAY_OUTBOX_POLL_MS` (default `500`): poll interval for delivering outbound events to sinks.
- `JUNO_PAY_OUTBOX_BATCH_SIZE` (default `100`): max deliveries per poll.
- `JUNO_PAY_OUTBOX_MAX_ATTEMPTS` (default `25`): max delivery attempts before marking failed.

### Storage

Supported store drivers:

- `sqlite` (default): embedded SQLite (via `modernc.org/sqlite`) stored under `JUNO_PAY_DATA_DIR`.
- `postgres`: PostgreSQL (tables created automatically; use `JUNO_PAY_STORE_PREFIX` to namespace tables).
- `mysql`: MySQL (tables created automatically; use `JUNO_PAY_STORE_PREFIX` to namespace tables).
- `mongo`: MongoDB (collections created automatically; requires a replica set for transactions).

Operational notes:
- For `sqlite`, treat the data directory as stateful (use block storage + backups).
- For `postgres|mysql|mongo`, DB backups/HA are your responsibility.
- If you wipe the DB and reuse the same UFVKs, late payments to previously-issued addresses may be attributed to newly-created invoices.

## Admin dashboard

The admin UI is served by the backend:

- UI: `GET /admin/`
- Login API: `POST /admin/login`
- Logout API: `POST /admin/logout`

For local development (without Docker), build the static UI bundle:

```bash
cd admin-dashboard
npm ci
npm run build
```

Then run `juno-pay-server` with `JUNO_PAY_ADMIN_UI_DIR=admin-dashboard/out` (useful when iterating on the UI without rebuilding the Go binary).

Typical admin flow:

1. Create a merchant (configures invoice expiry + confirmations + payment policies).
2. Set the merchant wallet UFVK (immutable; this is the only chain secret the backend needs).
3. Create merchant API keys (used only for invoice creation).
4. Configure event sinks (webhook and/or brokers) for invoice/settlement events.
5. Monitor `/status` for scanner cursor progress, restarts, and delivery backlog.
6. Review deposits, invoices, and review cases; optionally create refunds (record + event emission).

## Outbound events (webhooks + brokers)

`juno-pay-server` stores outbound events durably and retries delivery with exponential backoff.

Supported sink kinds:
- `webhook`
- `kafka` (works with AWS MSK)
- `rabbitmq` (works with Amazon MQ RabbitMQ)
- `nats` (self-hosted)

See `api/openapi.yaml` for exact schemas.

## Demo app

`demo-app/` is a standalone demo checkout UI:
- localStorage-only (no DB)
- uses `POST /v1/invoices` (merchant API key) + public invoice endpoints

## Deployment references

Primary migration target:

- DigitalOcean host infrastructure and GHCR-based deployment: `deploy/do/`
- Cloudflare DNS / migration notes: `deploy/cloudflare/`
- AWS source-host recovery and legacy reference: `deploy/aws/`

Current production migration shape:

- `junocashd` on a single DO Droplet with attached block storage
- `juno-scan` on the same host with local RocksDB
- `juno-pay-server` on the same host with local SQLite
- `caddy` serving the apex, `www`, and staging hostnames
- Cloudflare staged in front of the origin during the DNS cutover

Legacy AWS reference deployment remains under `deploy/aws/terraform/` until the final cutover and rollback window are complete.
