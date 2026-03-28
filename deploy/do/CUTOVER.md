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
- The remaining blocker before cutover is verified shell access to the AWS source host for the warm sync.

Supporting operator tools:

- source-host access verification: `deploy/aws/scripts/check-source-access.sh`
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

Warm sync requires source-host shell access. If the AWS host blocks SSH, restore temporary operator access before attempting the sync. Do not proceed to production cutover without a verified source access path.

Use the scripted source access check instead of ad hoc SSH retries:

```bash
deploy/aws/scripts/check-source-access.sh \
  --instance-id i-0fe82490b2e05db4e \
  --security-group-id sg-0595fddf6f6561904 \
  --instance-ip 18.206.49.27 \
  --region us-east-1 \
  --ssh-private-key ~/.ssh/id_ed25519 \
  --use-ec2-instance-connect
```

Current source-host blocker observed during implementation:

- port `22/tcp` was not open by default
- temporary source-side SSH access from the operator workstation reached the host, but none of the available private keys authenticated successfully
- `ec2-user`, `ubuntu`, `admin`, and `root` were all rejected
- EC2 Instance Connect accepted the public key push for `ec2-user`, but the host still rejected the login

Resolve the source-host access path before scheduling the cutover window.

Repeat the warm sync until the final maintenance window.

Recommended cadence:

- run the first warm sync immediately after source access is restored
- repeat every 24 hours until cutover
- run an extra warm sync after any production-side change that affects mutable state

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
3. Stop the AWS stack cleanly.
4. Run one final sync:

   ```bash
   deploy/do/scripts/sync-state-stream.sh \
     --source-host 18.206.49.27 \
     --source-user ec2-user \
     --target-host 159.203.150.96 \
     --target-user root
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

- Keep AWS online but out of traffic for 72 hours.
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
