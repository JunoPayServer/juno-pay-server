# Juno Pay Server DO Cutover Runbook

This runbook assumes:

- AWS remains the live origin until the final maintenance window.
- Cloudflare is already configured with proxied DNS records for:
  - `junopayserver.com`
  - `www.junopayserver.com`
  - `staging.junopayserver.com`
- The DO foundation already exists in project `junopayserver`.
- Cloudflare Access is already live for staging and production admin paths.
- Cloudflare zone SSL mode is already `strict`.
- Cloudflare Load Balancing is already live with AWS active and DO as healthy standby.
- Warm syncs and the final cutover sync use snapshot-derived helper transfers from the AWS data volume.

Supporting operator tools:

- snapshot sync orchestration: `deploy/aws/scripts/sync-data-volume-snapshot.sh`
- source-host access verification fallback: `deploy/aws/scripts/check-source-access.sh`
- post-sync readiness comparison: `deploy/do/scripts/check-cutover-readiness.sh`
- Cloudflare LB primary switch: `deploy/cloudflare/scripts/switch-lb-primary.sh`

## 1. Pre-stage the DO host

1. Bootstrap the host.

   ```bash
   ssh root@159.203.150.96 'bash -s' < deploy/do/scripts/bootstrap-host.sh
   ```

2. Build and push runtime images to GHCR.

   ```bash
   docker login ghcr.io
   eval "$(
     deploy/do/scripts/build-push-ghcr.sh \
       --owner <github-owner> \
       --image-prefix juno-pay \
       --tag <tag>
   )"
   ```

3. Deploy the stack to the DO host with staging enabled.

   Use `.github/workflows/deploy-do.yml` or run `deploy/do/scripts/deploy-stack.sh` manually over SSH.
   For the initial staging deploy, set:

   - `CADDY_SERVER_NAMES=staging.junopayserver.com`

   Expand `CADDY_SERVER_NAMES` to `junopayserver.com, www.junopayserver.com, staging.junopayserver.com` only when production traffic is ready to move.

## 2. Warm-sync mutable state from AWS snapshots

Run the snapshot-derived sync from an operator workstation that has:

- AWS CLI access
- `doctl --context juno`
- the existing DO SSH private key for `root@159.203.150.96`

Warm sync command:

```bash
deploy/aws/scripts/sync-data-volume-snapshot.sh \
  --do-ssh-key <path-to-existing-do-ssh-key> \
  --readiness-service-token-file tmp/cloudflare-access-service-token.json
```

This flow:

- starts helper `i-06f8b5e5c0aa7dece` if needed
- snapshots `vol-0d5701021c67b3f7d`
- creates and attaches a temporary sync volume in `us-east-1a`
- mounts the snapshot volume read-only on the helper
- temporarily allows the helper egress `/32` to reach DO SSH
- copies:
  - `junocashd`
  - `juno-scan`
  - `juno-pay-server`
- removes the temporary DO firewall rule and deletes the temporary sync volume

If a sync is interrupted after the snapshot is already created, resume from it with `--snapshot-id <existing-snapshot-id>` instead of starting over.

Warm sync no longer depends on shell access to the live AWS source host.

Source-host SSH recovery is fallback only. Use it only if the snapshot path becomes unusable:

```bash
deploy/aws/scripts/check-source-access.sh \
  --instance-id i-0fe82490b2e05db4e \
  --security-group-id sg-0595fddf6f6561904 \
  --instance-ip 18.206.49.27 \
  --region us-east-1 \
  --ssh-private-key ~/.ssh/id_ed25519 \
  --use-ec2-instance-connect
```

Repeat the warm sync until the final maintenance window.

Recommended cadence:

- run the first warm sync immediately
- repeat every 24 hours until cutover
- run an extra warm sync after any production-side change that affects mutable state

Snapshot retention:

- keep the latest successful warm snapshot until the next warm sync succeeds
- delete the previous warm snapshot on the next successful run with `--delete-snapshot-id <old-snapshot-id>`

## 3. Validate staging

Before any production cutover:

1. Keep `staging.junopayserver.com` pointed at the DO reserved IP.
2. Confirm the DO host answers:
   - `GET /v1/health`
   - `GET /v1/status`
   - `/admin/`
   - demo app root `/`
3. Confirm unauthenticated staging requests redirect to Access.
4. Confirm the Access service token can reach staging.
5. Confirm invoice creation, public invoice fetch, and webhook/status updates behave correctly.

After each warm sync, run:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --service-token-file tmp/cloudflare-access-service-token.json
```

Add `--merchant-api-key <merchant-api-key>` when you want the scripted synthetic invoice create/public fetch check in the same run.

## 4. Final maintenance window

1. Confirm the Cloudflare load balancer, monitor, and both pools are still healthy with AWS active and DO standby.
2. Enable maintenance mode at the edge.
3. Stop the AWS source instance `i-0fe82490b2e05db4e`.
4. Run one final cold snapshot sync:

   ```bash
   deploy/aws/scripts/sync-data-volume-snapshot.sh \
     --snapshot-kind cold \
     --do-ssh-key <path-to-existing-do-ssh-key> \
     --readiness-service-token-file tmp/cloudflare-access-service-token.json
   ```

5. Start or restart the DO stack.
6. Validate on DO:
   - `/v1/health`
   - `/v1/status`
   - admin login
   - synthetic invoice create/fetch/update flow
7. Switch the Cloudflare load balancer active pool from AWS to DO.
8. Remove maintenance mode only after DO validation passes.

To promote DO with the current Cloudflare global-key fallback:

```bash
deploy/cloudflare/scripts/switch-lb-primary.sh --target do --exclusive
```

## 5. Rollback window

- Keep AWS stopped and out of traffic for 72 hours.
- Treat DO as the source of truth after reopening writes.
- Remove AWS from active failover after writes reopen. Do not leave AWS as an automatic fallback target once DO accepts production writes.
- Any rollback after writes reopen requires:
  - a fresh maintenance window
  - a reverse sync plan from DO back to AWS
  - a deliberate Cloudflare LB switch back to AWS:

    ```bash
    deploy/cloudflare/scripts/switch-lb-primary.sh --target aws
    ```

## 6. Final decommission

After 72 stable hours:

1. Snapshot the AWS data volume.
2. Export any final backups required for retention.
3. Remove AWS traffic, EIP, Route53 hosted zone, and legacy ECR artifacts that are no longer needed.
4. Keep the Cloudflare zone authoritative for steady-state production.
