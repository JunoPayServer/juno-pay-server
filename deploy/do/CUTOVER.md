# Juno Pay Server DO Cutover Runbook

This runbook assumes:

- AWS remains the live origin until the final maintenance window.
- Cloudflare is already configured with DNS records for:
  - `junopayserver.com`
  - `www.junopayserver.com`
  - `staging.junopayserver.com`
- The DO foundation already exists in project `junopayserver`.
- Cloudflare Access and Load Balancing remain blocked until the Cloudflare plugin is reconnected with the required Zero Trust and Load Balancing permissions.

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

## 2. Warm-sync mutable state from AWS

Run the streaming sync from an operator workstation that can SSH to both hosts:

```bash
deploy/do/scripts/sync-state-stream.sh \
  --source-host 18.206.49.27 \
  --source-user ec2-user \
  --target-host 159.203.150.96 \
  --target-user root
```

This copies:

- `/opt/juno-pay/data/junocashd`
- `/opt/juno-pay/data/juno-scan`
- `/opt/juno-pay/data/juno-pay-server`

Repeat the warm sync until the final maintenance window.

## 3. Validate staging

Before any production cutover:

1. Point `staging.junopayserver.com` at the DO reserved IP.
2. Confirm the DO host answers:
   - `GET /v1/health`
   - `GET /v1/status`
   - `/admin/`
   - demo app root `/`
3. Confirm invoice creation, public invoice fetch, and webhook/status updates behave correctly.
4. Once Cloudflare Access permissions are fixed, protect the full staging hostname before broader validation.

## 4. Final maintenance window

1. Enable maintenance mode at the edge.
2. Stop the AWS stack cleanly.
3. Run one final sync:

   ```bash
   deploy/do/scripts/sync-state-stream.sh \
     --source-host 18.206.49.27 \
     --source-user ec2-user \
     --target-host 159.203.150.96 \
     --target-user root
   ```

4. Start or restart the DO stack.
5. Validate on DO:
   - `/v1/health`
   - `/v1/status`
   - admin login
   - synthetic invoice create/fetch/update flow
6. Switch Cloudflare traffic from AWS to DO.
7. Remove maintenance mode only after DO validation passes.

## 5. Rollback window

- Keep AWS online but out of traffic for 72 hours.
- Treat DO as the source of truth after reopening writes.
- Any rollback after writes reopen requires:
  - a fresh maintenance window
  - a reverse sync plan from DO back to AWS
  - a deliberate Cloudflare origin switch

## 6. Final decommission

After 72 stable hours:

1. Snapshot the AWS data volume.
2. Export any final backups required for retention.
3. Remove AWS traffic, EIP, Route53 hosted zone, and legacy ECR artifacts that are no longer needed.
4. Keep the Cloudflare zone authoritative for steady-state production.
