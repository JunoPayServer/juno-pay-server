# AWS migration source reference

AWS is now legacy-only in this repo.

Use the AWS deployment material for:

- source-host inspection during the DO migration
- warm-sync access recovery
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

Observed source-access blockers as of `2026-03-28`:

- `KeyName` is `null`
- the host is not registered in SSM
- EC2 Instance Connect push succeeded, but login still failed
- the host only became reachable on `22/tcp` when a temporary operator CIDR rule was added

## Access recovery workflow

Use the scripted access check first:

```bash
deploy/aws/scripts/check-source-access.sh \
  --instance-id i-0fe82490b2e05db4e \
  --security-group-id sg-0595fddf6f6561904 \
  --instance-ip 18.206.49.27 \
  --region us-east-1 \
  --ssh-private-key ~/.ssh/id_ed25519 \
  --use-ec2-instance-connect
```

The script:

- checks whether SSM is online and can execute a shell command
- temporarily opens `22/tcp` only to the operator CIDR when SSH testing is requested
- optionally publishes a one-time EC2 Instance Connect key for `ec2-user`
- verifies shell access by checking `/opt/juno-pay/data`
- removes the temporary `22/tcp` rule automatically unless `--leave-ssh-open` is used

If the script cannot verify either SSM or SSH access, stop retrying ad hoc keys and switch to the rescue workflow.

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
