# DigitalOcean deployment

This folder contains the current DigitalOcean deployment path for `juno-pay-server`.

Operational defaults for this repo:

- Use `doctl --context juno` for all DigitalOcean mutations.
- Use the DigitalOcean project `junopayserver`.
- Keep the origin topology close to the current AWS host:
  - `junocashd`
  - `juno-scan` with RocksDB on local block storage
  - `juno-pay-server` with SQLite on local block storage
  - `demo-app`
  - `caddy`

## Live foundation created

The following resources have already been created in the `juno` DigitalOcean account:

- Project: `junopayserver`
- Droplet: `junopayserver-prod`
- Volume: `junopayserver-data`
- Firewall: `junopayserver-fw`
- Reserved IP: attached to `junopayserver-prod`
- SSH key: `junopayserver-admin`

To inspect the current live state:

```bash
deploy/do/scripts/describe-live-resources.sh
```

Cutover and data-migration steps live in `deploy/do/CUTOVER.md`.

AWS snapshot migration, source-access fallback, and legacy reference material live in `deploy/aws/README.md`.

## Account constraint observed

The original plan called for a `250 GiB` volume. The current DigitalOcean account rejected that size during implementation and accepted `100 GiB` instead.

Treat `100 GiB` as the current live baseline unless the account limit is raised and the volume is resized or replaced later.

## Host bootstrap

The DO host is expected to run Ubuntu and mount the attached volume at `/opt/juno-pay/data`.

To bootstrap the host manually:

```bash
ssh root@<reserved-ip> 'bash -s' < deploy/do/scripts/bootstrap-host.sh
```

The bootstrap script:

- installs Docker and the Compose plugin
- installs `rsync` for warm-sync and final-sync operations
- mounts the DO volume by filesystem label `junopaydata`
- creates the required state directories under `/opt/juno-pay/data`

## Image publishing

Images are published to GHCR, not ECR.

Build and push all runtime images:

```bash
docker login ghcr.io
deploy/do/scripts/build-push-ghcr.sh \
  --owner <github-owner> \
  --image-prefix juno-pay \
  --tag <tag>
```

The script prints shell assignments for:

- `IMAGE_JUNO_PAY_SERVER`
- `IMAGE_JUNOCASHD`
- `IMAGE_JUNO_SCAN`
- `IMAGE_DEMO_APP`

## Deploying the stack

The deploy path writes a root-owned runtime env file on the host, renders `docker-compose.yml` and `Caddyfile`, logs into GHCR, and starts the stack.

Required environment variables for `deploy/do/scripts/deploy-stack.sh`:

- `ROOT` (recommended: `/opt/juno-pay`)
- `DOMAIN_NAME`
- `WWW_DOMAIN`
- `STAGING_DOMAIN`
- Optional: `CADDY_SERVER_NAMES` (defaults to `DOMAIN_NAME, WWW_DOMAIN, STAGING_DOMAIN`)
- `IMAGE_JUNO_PAY_SERVER`
- `IMAGE_JUNOCASHD`
- `IMAGE_JUNO_SCAN`
- `IMAGE_DEMO_APP`
- `JUNO_PAY_ADMIN_PASSWORD`
- `JUNO_PAY_TOKEN_KEY_HEX`
- Optional: `JUNO_PAY_DEMO_MERCHANT_API_KEY`
- Optional: `CADDY_ORIGIN_CERT_PEM_B64`
- Optional: `CADDY_ORIGIN_KEY_PEM_B64`
- Optional: `GHCR_USERNAME`, `GHCR_TOKEN`

Manual example:

```bash
ssh root@<reserved-ip> 'mkdir -p /tmp/juno-pay-deploy'
scp -r deploy/do/scripts root@<reserved-ip>:/tmp/juno-pay-deploy/
ssh root@<reserved-ip> '
  export ROOT=/opt/juno-pay
  export DOMAIN_NAME=junopayserver.com
  export WWW_DOMAIN=www.junopayserver.com
  export STAGING_DOMAIN=staging.junopayserver.com
  export CADDY_SERVER_NAMES=staging.junopayserver.com
  export IMAGE_JUNO_PAY_SERVER=ghcr.io/<owner>/juno-pay-juno-pay-server:prod
  export IMAGE_JUNOCASHD=ghcr.io/<owner>/juno-pay-junocashd:prod
  export IMAGE_JUNO_SCAN=ghcr.io/<owner>/juno-pay-juno-scan:prod
  export IMAGE_DEMO_APP=ghcr.io/<owner>/juno-pay-juno-demo-app:prod
  export JUNO_PAY_ADMIN_PASSWORD=<admin-password>
  export JUNO_PAY_TOKEN_KEY_HEX=<token-key-hex>
  export JUNO_PAY_DEMO_MERCHANT_API_KEY=<merchant-api-key>
  export CADDY_ORIGIN_CERT_PEM_B64=<base64-pem-cert>
  export CADDY_ORIGIN_KEY_PEM_B64=<base64-pem-key>
  export GHCR_USERNAME=<github-user>
  export GHCR_TOKEN=<ghcr-token>
  bash /tmp/juno-pay-deploy/scripts/deploy-stack.sh
'
```

Primary mutable-state sync path:

```bash
deploy/aws/scripts/sync-data-volume-snapshot.sh \
  --do-ssh-key <path-to-existing-do-ssh-key> \
  --readiness-service-token-file tmp/cloudflare-access-service-token.json
```

This uses the AWS helper instance to mount a snapshot-derived copy of the AWS data volume read-only.

Both warm and cold snapshot syncs now stream only:

- `juno-pay-server`

`warm` versus `cold` now affects snapshot timing only:

- `warm`: snapshot while AWS is still live
- `cold`: snapshot after the AWS source instance is stopped during maintenance

The native DO chain path is now authoritative for staging preparation:

- `junocashd` stays DO-native and keeps syncing from the network
- `juno-scan` stays DO-native and rebuilds from the DO node
- every restored `juno-pay-server` tree is re-owned to `10001:65534`
- every pay-server restore deletes all rows from `scan_cursors`, then replays from the DO scanner

Do not overwrite DO `/opt/juno-pay/data/junocashd` or DO `/opt/juno-pay/data/juno-scan/db` from AWS again.

If staging needs a full native recovery, use:

```bash
ssh root@159.203.150.96 'bash -se -- --root /opt/juno-pay' \
  < deploy/do/scripts/recover-native-staging.sh
```

That workflow:

- validates `state.sqlite` with `PRAGMA integrity_check`
- restores the latest local SQLite backup if the DB is invalid
- wipes DO `junocashd` and `juno-scan` caches
- resets `scan_cursors`
- starts a fresh DO-native `junocashd`
- starts a fresh DO-native `juno-scan`
- re-registers merchant wallets from `state.sqlite`
- backfills wallet history through `juno-scan`
- restarts `juno-pay-server`

Standalone replay/reset entrypoint:

```bash
ssh root@159.203.150.96 'bash -se -- --mode replay --root /opt/juno-pay' \
  < deploy/do/scripts/rebuild-staging-scan-state.sh
```

For a full DO-native bootstrap on an already deployed host:

```bash
ssh root@159.203.150.96 'bash -se -- --mode bootstrap --root /opt/juno-pay' \
  < deploy/do/scripts/rebuild-staging-scan-state.sh
```

Legacy direct-host sync tooling remains in the repo only as rollback/reference material. Do not use `deploy/do/scripts/sync-state-stream.sh` under the current migration design because it can overwrite DO-native node or scanner state.

Use bootstrap readiness while the native DO node and scanner are still catching up after a full recovery:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --mode bootstrap \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

Once the DO node and scanner are already established, compare the current AWS production state against the DO staging state after each pay-server-only warm sync:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --mode warm \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

If you also want the synthetic invoice create/public fetch check in the same run, provide a merchant API key:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --mode warm \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key> \
  --merchant-api-key <merchant-api-key>
```

For the final cold-sync validation, switch to:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --mode final \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

Warm readiness now checks the live `juno-scan` container directly over SSH and records:

- DO `junocashd` height
- pay-server cursor progress
- scanner `scanned_height` progress
- scanner log health since the current container start

Bootstrap mode is intentionally looser than warm mode:

- the DO node must be healthy and making forward progress
- the DO scanner must be healthy and making forward progress
- the pay-server cursor may still be `0` until replay has actual events to apply

After a pay-server replay, staging cursor IDs are not expected to match production cursor IDs because the scanner event stream is rebuilt locally. Warm validation cares about replay convergence against the DO-native scanner, not cursor ID equality with AWS.

To wait for actual bootstrap exit criteria instead of sampling manually:

```bash
deploy/do/scripts/wait-bootstrap-parity.sh \
  --required-consecutive 2 \
  --interval-seconds 900 \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

That gate only exits successfully after 2 consecutive samples where:

- DO `junocashd` height equals production height
- DO `juno-scan` `scanned_height` equals DO `junocashd` height
- bootstrap readiness remains green

Do not start another pay-server warm snapshot while the current staging replay is still catching up. Wait until the current rebuild converges cleanly, then resume the 24-hour warm-sync cadence.

## GitHub Actions deployment

The repository workflow `.github/workflows/deploy-do.yml` is the preferred repeatable deployment entrypoint.

Required repository secrets:

- `DO_SSH_PRIVATE_KEY`
- `JUNO_PAY_ADMIN_PASSWORD`
- `JUNO_PAY_TOKEN_KEY_HEX`
- Optional: `DEMO_MERCHANT_API_KEY`
- Optional: `DEMO_MERCHANT_API_KEY_STAGING`
- Optional: `CADDY_ORIGIN_CERT_PEM_B64`
- Optional: `CADDY_ORIGIN_KEY_PEM_B64`

The workflow:

- builds and pushes images to GHCR
- copies the DO deploy scripts to the host
- bootstraps the host (optional per run)
- deploys the stack over SSH
- prefers `DEMO_MERCHANT_API_KEY_STAGING` when `verify_host` matches `staging_domain`
- uses Cloudflare Origin CA certs for Caddy when `CADDY_ORIGIN_CERT_PEM_B64` and `CADDY_ORIGIN_KEY_PEM_B64` are present
- defaults to staging-only certificate issuance with `caddy_server_names=staging.junopayserver.com`
- checks `/v1/health` and `/v1/status` over HTTPS against the DO reserved IP using the configured `verify_host`
- trusts Cloudflare Origin CA roots during direct-origin verification when Origin CA cert secrets are configured
