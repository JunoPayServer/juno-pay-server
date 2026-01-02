# AWS reference deployment (Terraform)

This folder contains a **reference** Terraform stack for running:

- `junocashd` (Docker)
- `juno-scan` (Docker)
- `juno-pay-server` (Docker, serves `/admin/`)

It is intentionally modular and conservative: you can plug it into an existing VPC/subnets and optionally enable managed services like RDS/MSK.

## Prereqs

- Terraform >= 1.6
- AWS credentials configured (SSO or access keys)
- Docker images published to ECR (see `../scripts/build-push-ecr.sh`)

## Secrets

This stack expects the following SSM parameters to exist (SecureString recommended):

- `admin_password_ssm_param` (default: `/juno-pay/admin_password`)
- `token_key_ssm_param` (default: `/juno-pay/token_key_hex`)
- Optional: `pay_store_dsn_ssm_param` (connection string for `juno-pay-server` when using `postgres|mysql|mongo`)

Create them using the helper script:

```bash
../scripts/put-ssm-params.sh \
  --region us-east-1 \
  --admin-password-param /juno-pay/admin_password \
  --token-key-param /juno-pay/token_key_hex \
  --pay-store-dsn-param /juno-pay/pay_store_dsn
```

## Usage

```bash
terraform init
terraform apply
```

Outputs include the instance public IP and the URL for `/admin/`.

Example variables: `terraform.tfvars.example`.

## Optional: RDS (Postgres) for `juno-scan`

Set:
- `enable_rds_postgres=true`
- `rds_subnet_ids=[...]` (subnets that can reach the EC2 host; typically private subnets)

The EC2 host fetches the generated RDS master password from Secrets Manager at boot time and injects it into the Docker Compose DSN.

## Optional: External DB for `juno-pay-server`

By default, the reference stack runs `juno-pay-server` with embedded SQLite on the instance volume.

To use an external DB (recommended for larger deployments), set:

- `pay_store_driver=postgres|mysql|mongo`
- `pay_store_dsn_ssm_param=/path/to/ssm/param` (contains the DSN/URI)
- If `pay_store_driver=mongo`: `pay_store_db=...`
- Optional: `pay_store_prefix=...` to namespace tables/collections when sharing a DB.

This stack does not create the external DB for you; you can point it at an existing RDS/DocumentDB/self-hosted DB reachable from the EC2 instance.

## Optional: MSK (Kafka)

Set:
- `enable_msk=true`
- `msk_subnet_ids=[...]` (at least 2 subnets)

Use the `msk_bootstrap_brokers` output when creating a `kafka` event sink in the admin dashboard.
