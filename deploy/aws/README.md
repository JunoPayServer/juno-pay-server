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
- sync scope: `juno-pay-server` state only
- final cutover path: cold pay-server snapshot after AWS stop
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
- stops only `juno-pay-server` before applying restored state
- streams only `/opt/juno-pay/data/juno-pay-server` during both `warm` and `cold` syncs
- re-owns the restored pay-server tree to `10001:65534`
- resets `scan_cursors` after every restore
- replays pay-server state from the already-running DO-native scanner
- removes the temporary DO firewall rule, deletes the temporary sync volume, and optionally stops the helper

Use `--snapshot-kind cold` during the final maintenance window after the AWS source instance is stopped. `warm` versus `cold` now affects snapshot timing only.

If a run is interrupted after the snapshot is already created, resume from that snapshot with:

```bash
deploy/aws/scripts/sync-data-volume-snapshot.sh \
  --snapshot-id <existing-snapshot-id> \
  --do-ssh-key <path-to-existing-do-ssh-key>
```

If a warm sync succeeds, keep its snapshot until the next successful warm sync. The script prints `snapshot_id=...` so the previous warm snapshot can be deleted on the next successful run with `--delete-snapshot-id`.

Do not copy AWS `junocashd` or AWS `juno-scan` state to DO again. The native DO node/scanner path is now the only supported staging and cutover preparation path.

If DO staging needs a full native rebuild, run:

```bash
ssh root@159.203.150.96 'bash -se -- --root /opt/juno-pay' \
  < deploy/do/scripts/recover-native-staging.sh
```

Then validate with bootstrap readiness:

```bash
deploy/do/scripts/check-cutover-readiness.sh \
  --mode bootstrap \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

Do not queue a second warm snapshot on top of an unconverged staging replay. After each pay-server warm sync:

1. keep the DO node and DO scanner running continuously
2. reset `scan_cursors` and replay pay-server state from the DO scanner
3. confirm warm readiness converges cleanly
4. only then resume the 24-hour warm-sync cadence

Before the first pay-server warm sync after a native DO recovery, require:

```bash
deploy/do/scripts/wait-bootstrap-parity.sh \
  --required-consecutive 2 \
  --interval-seconds 900 \
  --service-token-file tmp/cloudflare-access-service-token.json \
  --target-ssh-key <path-to-existing-do-ssh-key>
```

Do not take the first pay-server warm snapshot until that gate succeeds.

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

Once shell access is restored, do not resume the old direct-host stream automatically. Revisit the migration plan explicitly before using any host-to-host sync path.

## Legacy infrastructure reference

The legacy Terraform-based AWS deployment reference remains under `deploy/aws/terraform/`.
