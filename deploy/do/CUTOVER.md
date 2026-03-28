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
- Warm syncs and the final cutover sync use snapshot-derived helper transfers of `juno-pay-server` state from the AWS data volume.
- `junocashd` and `juno-scan` run natively on DO and must not be overwritten from AWS snapshots.

Supporting operator tools:

- snapshot sync orchestration: `deploy/aws/scripts/sync-data-volume-snapshot.sh`
- native DO recovery bootstrap: `deploy/do/scripts/recover-native-staging.sh`
- post-sync readiness comparison: `deploy/do/scripts/check-cutover-readiness.sh`
- bootstrap parity gate: `deploy/do/scripts/wait-bootstrap-parity.sh`
- pay-server replay/bootstrap on DO staging: `deploy/do/scripts/rebuild-staging-scan-state.sh`
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

## 2. Recover the DO host onto a native node path

If staging has been damaged by older AWS node/scanner restores, recover it before resuming warm syncs:

```bash
ssh root@159.203.150.96 'bash -se -- --root /opt/juno-pay' \
  < deploy/do/scripts/recover-native-staging.sh
```

This workflow:

- validates `state.sqlite`
- restores the latest local SQLite backup if integrity fails
- wipes DO `junocashd` and `juno-scan` caches
- resets `scan_cursors`
- starts a clean DO-native `junocashd`
- starts a clean DO-native `juno-scan`
- re-registers wallets and backfills them through the DO scanner
- restarts `juno-pay-server`

Run bootstrap readiness until the native DO node and scanner are clearly advancing:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --mode bootstrap \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

Do not run another AWS sync until bootstrap readiness is green.

To block automatically until the DO node and scanner have reached production parity in 2 consecutive samples:

```bash
deploy/do/scripts/wait-bootstrap-parity.sh \
  --required-consecutive 2 \
  --interval-seconds 900 \
  --height-lag-tolerance 1 \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

## 3. Warm-sync pay-server state from AWS snapshots

Run the snapshot-derived pay-server sync from an operator workstation that has:

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
- stops only `juno-pay-server` before the restored SQLite tree is applied
- copies only `/opt/juno-pay/data/juno-pay-server`
- re-owns the restored pay-server tree to `10001:65534`
- deletes all rows from `scan_cursors`
- replays pay-server state from the already-running DO scanner
- removes the temporary DO firewall rule and deletes the temporary sync volume

If a sync is interrupted after the snapshot is already created, resume from it with `--snapshot-id <existing-snapshot-id>` instead of starting over.

Repeat the warm sync until the final maintenance window.

Recommended cadence:

- keep DO `junocashd` and `juno-scan` running continuously
- once `wait-bootstrap-parity.sh` succeeds, run one pay-server-only warm sync
- require warm readiness to pass
- repeat once more after roughly 24 hours
- do not schedule cutover unless two pay-server-only warm cycles reconverge cleanly

Snapshot retention:

- keep the latest successful warm snapshot until the next warm sync succeeds
- delete the previous warm snapshot on the next successful run with `--delete-snapshot-id <old-snapshot-id>`

## 4. Validate staging

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
  --mode warm \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

Add `--merchant-api-key <merchant-api-key>` when you want the scripted synthetic invoice create/public fetch check in the same run.

The warm-mode readiness check now validates:

- DO node parity or near-parity with production
- DO scanner parity with the DO node
- pay-server health and Access behavior
- pay-server replay convergence, or a stable non-zero cursor after scanner-tip convergence
- `juno-scan` `scanned_height` from the live container
- `juno-scan` logs since the current container start

Do not treat container health alone as sufficient. Warm staging is only usable when the warm-mode readiness check passes, and no new warm snapshot should be taken while the current replay is still converging.

## 5. Final maintenance window

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

5. Let the pay-server replay finish against the already-synced DO node and DO scanner.
6. Start or restart the DO stack if needed.
7. Validate on DO:
   - `/v1/health`
   - `/v1/status`
   - admin login
   - synthetic invoice create/fetch/update flow
   - final readiness parity:

     ```bash
     deploy/do/scripts/check-cutover-readiness.sh \
       --mode final \
       --service-token-file tmp/cloudflare-access-service-token.json \
       --target-ssh-key <path-to-existing-do-ssh-key>
     ```

8. Switch the Cloudflare load balancer active pool from AWS to DO.
9. Remove maintenance mode only after DO validation passes.

To promote DO with the current Cloudflare global-key fallback:

```bash
deploy/cloudflare/scripts/switch-lb-primary.sh --target do --exclusive
```

## 6. Rollback window

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

## 7. Final decommission

After 72 stable hours:

1. Snapshot the AWS data volume.
2. Export any final backups required for retention.
3. Remove AWS traffic, EIP, Route53 hosted zone, and legacy ECR artifacts that are no longer needed.
4. Keep the Cloudflare zone authoritative for steady-state production.
