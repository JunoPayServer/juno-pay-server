# AWS migration source reference

AWS is now legacy-only in this repo.

Use the AWS deployment material for:

- source-host inspection during the DO migration
- snapshot-based warm sync and final cutover sync
- source-host access recovery fallback if snapshot migration becomes unusable
- rollback reference until the DO cutover is complete
- final decommission after the 72-hour hold

Do not use AWS here for new target infrastructure.

## Current live source host

The current production origin still runs on AWS and remains the active Cloudflare LB pool until the final maintenance window.

- Instance ID: `i-0fe82490b2e05db4e`
- Public IP: `18.206.49.27`
- Region: `us-east-1`
- Security group: `sg-0595fddf6f6561904`
- Data volume: `vol-0d5701021c67b3f7d`

Current mutable-state migration defaults as of `2026-03-28`:

- primary path: snapshot-derived helper sync from `vol-0d5701021c67b3f7d`
- helper host: `i-06f8b5e5c0aa7dece` (`juno-prod-desktop-signer`)
- final cutover path: cold snapshot after AWS stop
- source-host SSH repair is fallback only

Observed source-access blockers on the live source host:

- `KeyName` is `null`
- the host is not registered in SSM
- EC2 Instance Connect push succeeded, but login still failed
- the host only became reachable on `22/tcp` when a temporary operator CIDR rule was added

## Snapshot sync workflow

Use the snapshot-sync entrypoint for warm syncs and the final cold-sync pass:

```bash
deploy/aws/scripts/sync-data-volume-snapshot.sh \
  --do-ssh-key <path-to-existing-do-ssh-key> \
  --region us-east-1 \
  --readiness-service-token-file tmp/cloudflare-access-service-token.json
```

The script:

- ensures the helper instance is running and SSM-online
- creates a snapshot of `vol-0d5701021c67b3f7d`
- creates and attaches a temporary volume from that snapshot in `us-east-1a`
- mounts that temporary volume read-only on the helper
- detects the helper egress IP and temporarily opens `22/tcp` on the DO firewall for that `/32`
- copies the existing DO SSH private key to the helper for the sync only
- stops the DO core services before applying synced state
- streams `junocashd` and `juno-pay-server` during `warm` syncs
- streams `junocashd`, `juno-scan`, and `juno-pay-server` during `cold` syncs
- rebuilds `juno-scan` on staging after every warm sync by wiping `/opt/juno-pay/data/juno-scan/db`, resetting `scan_cursors` in `/opt/juno-pay/data/juno-pay-server/state.sqlite`, re-registering merchant wallets, and backfilling each wallet history through the warmed chain tip
- removes the temporary DO firewall rule, deletes the temporary sync volume, and optionally stops the helper

Use `--snapshot-kind cold` during the final maintenance window after the AWS source instance is stopped.

If a run is interrupted after the snapshot is already created, resume from that snapshot with:

```bash
deploy/aws/scripts/sync-data-volume-snapshot.sh \
  --snapshot-id <existing-snapshot-id> \
  --do-ssh-key <path-to-existing-do-ssh-key>
```

If a warm sync succeeds, keep its snapshot until the next successful warm sync. The script prints `snapshot_id=...` so the previous warm snapshot can be deleted on the next successful run with `--delete-snapshot-id`.

Live warm snapshots of `juno-scan` are no longer considered safe. Treat `juno-scan` as rebuildable cache during warm-up and authoritative state only during the final cold cutover sync.

Do not queue a second warm snapshot on top of an unconverged staging rebuild. After each warm sync:

1. let staging `juno-scan` rebuild from the warmed chain data
2. monitor `juno-scan /v1/health` `scanned_height` and pay-server `last_cursor_applied`
3. confirm the current rebuild converges cleanly
4. only then resume the 24-hour warm-sync cadence

## Source-access fallback workflow

If the snapshot path becomes unusable, fall back to the source-access check:

```bash
deploy/aws/scripts/check-source-access.sh \
  --instance-id i-0fe82490b2e05db4e \
  --security-group-id sg-0595fddf6f6561904 \
  --instance-ip 18.206.49.27 \
  --region us-east-1 \
  --ssh-private-key ~/.ssh/id_ed25519 \
  --use-ec2-instance-connect
```

If the script cannot verify either SSM or SSH access, stop retrying ad hoc keys and switch to the rescue workflow below.

## Rescue-window workflow

Use this only in a planned maintenance window because it requires stopping the source instance.

1. Stop the AWS source instance.
2. Detach the root volume.
3. Attach the root volume to a helper EC2 instance with verified operator access.
4. Mount the filesystem and repair one durable access path:
   - add a temporary operator key to `ec2-user`
   - and/or repair SSM agent startup and registration
5. Reattach the root volume to the source instance and boot it.
6. Re-run `deploy/aws/scripts/check-source-access.sh` until either SSM or SSH is verified.
7. Remove any temporary SSH exposure after validation.

Once shell access is restored, move immediately to the first warm sync with `deploy/do/scripts/sync-state-stream.sh`.

## Legacy infrastructure reference

The legacy Terraform-based AWS deployment reference remains under `deploy/aws/terraform/`.
