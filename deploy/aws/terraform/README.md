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

Create them using the helper script:

```bash
../scripts/put-ssm-params.sh \
  --region us-east-1 \
  --admin-password-param /juno-pay/admin_password \
  --token-key-param /juno-pay/token_key_hex
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

## Optional: MSK (Kafka)

Set:
- `enable_msk=true`
- `msk_subnet_ids=[...]` (at least 2 subnets)

Use the `msk_bootstrap_brokers` output when creating a `kafka` event sink in the admin dashboard.
