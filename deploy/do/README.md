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

This uses the AWS helper instance to mount a snapshot-derived copy of the AWS data volume read-only and stream:

- `junocashd`
- `juno-scan`
- `juno-pay-server`

to the DO host over SSH.

Legacy fallback path if snapshot migration becomes unusable:

```bash
deploy/do/scripts/sync-state-stream.sh \
  --source-host 18.206.49.27 \
  --target-host 159.203.150.96
```

To compare the current AWS production state against the DO staging state after each warm sync:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --service-token-file tmp/cloudflare-access-service-token.json
```

If you also want the synthetic invoice create/public fetch check in the same run, provide a merchant API key:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --merchant-api-key <merchant-api-key>
```

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
