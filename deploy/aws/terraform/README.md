# AWS reference deployment (Terraform)

AWS is legacy-only for this repo now. Use this Terraform stack as rollback or historical reference only while the DO migration is in flight.

This folder contains a **reference** Terraform stack for running:

- `junocashd` (Docker)
- `juno-scan` (Docker)
- `juno-pay-server` (Docker, serves `/admin/`)

It is intentionally modular and conservative: by default it creates a small VPC + public subnet, but you can also plug it into an existing VPC/subnets and optionally enable managed services like RDS/MSK.

## Prereqs

- Terraform >= 1.6
- AWS credentials configured (SSO or access keys)
- Docker images published to ECR (see `../scripts/build-push-ecr.sh`)

## Secrets

This stack expects the following SSM parameters to exist (SecureString recommended):

- `admin_password_ssm_param` (default: `/juno-pay/admin_password`)
- `token_key_ssm_param` (default: `/juno-pay/token_key_hex`)
- Optional: `pay_store_dsn_ssm_param` (connection string for `juno-pay-server` when using `postgres|mysql|mongo`)
- Optional: `demo_merchant_api_key_ssm_param` (merchant API key used by the demo app for invoice creation)

Create them using the helper script:

```bash
../scripts/put-ssm-params.sh \
  --region us-east-1 \
  --admin-password-param /juno-pay/admin_password \
  --token-key-param /juno-pay/token_key_hex \
  --pay-store-dsn-param /juno-pay/pay_store_dsn \
  --demo-merchant-api-key-param /juno-pay/demo_merchant_api_key
```

## Usage

```bash
terraform init
terraform apply
```

Outputs include the instance public IP and the URL for `/admin/`.

Example variables: `terraform.tfvars.example`.

## Networking

Default (recommended for one-click deploy):

- `create_network=true` (creates VPC + public subnet + internet gateway)

To deploy into an existing VPC/subnet:

- `create_network=false`
- set `vpc_id` and `subnet_id`

## Demo app

By default this stack also runs the demo checkout app (Next.js) on `demo_port` (default `80`).

To make invoice creation work, set `demo_merchant_api_key_ssm_param` to an SSM SecureString containing a merchant API key.

## HTTPS + custom domain (Caddy + Let's Encrypt)

If `domain_name` and `route53_zone_id` are set, this stack:

- creates Route53 `A` records for `domain_name` and `www.domain_name` pointing at the instance EIP
- runs a Caddy reverse proxy on ports `80/443` with automatic TLS
- routes `/admin*` and `/v1/*` to `juno-pay-server` and everything else to the demo app

Make sure your domain registrar is using the Route53 name servers for the hosted zone.

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

## CI/CD (GitHub Actions)

This repo includes a GitHub Actions workflow: `.github/workflows/deploy-aws.yml`.

It can:
- build and push Docker images to ECR
- bootstrap Terraform remote state (S3 + DynamoDB)
- write required secrets to SSM
- run `terraform apply`

### 1) Create an AWS role for GitHub OIDC

In your AWS account:

1. Create an OIDC provider for `token.actions.githubusercontent.com` (audience: `sts.amazonaws.com`).
2. Create an IAM role that GitHub can assume via OIDC with a trust policy like:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::<ACCOUNT_ID>:oidc-provider/token.actions.githubusercontent.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "token.actions.githubusercontent.com:aud": "sts.amazonaws.com"
        },
        "StringLike": {
          "token.actions.githubusercontent.com:sub": "repo:Abdullah1738/juno-pay-server:*"
        }
      }
    }
  ]
}
```

Attach permissions that allow:
- ECR create/push (images)
- S3 + DynamoDB (Terraform state)
- EC2/IAM (this stack)
- SSM Put/GetParameter (admin password + token key)

### 2) Configure GitHub secrets and variables

GitHub **secrets** (Repository → Settings → Secrets and variables → Actions → Secrets):

- `AWS_DEPLOY_ROLE_ARN` (the role created above)
- `JUNO_PAY_ADMIN_PASSWORD`
- `JUNO_PAY_TOKEN_KEY_HEX` (32-byte hex)
- Optional: `DEMO_MERCHANT_API_KEY` (used by the demo app for invoice creation)

GitHub **variables** (same page → Variables):

- `TF_STATE_BUCKET` (S3 bucket for Terraform state)
- `TF_STATE_LOCK_TABLE` (DynamoDB table for state locking)

Optional variables:

- `JUNO_PAY_ADMIN_PASSWORD_SSM_PARAM` (default `/juno-pay/admin_password`)
- `JUNO_PAY_TOKEN_KEY_SSM_PARAM` (default `/juno-pay/token_key_hex`)
- `JUNO_PAY_DEMO_MERCHANT_API_KEY_SSM_PARAM` (default `/juno-pay/demo_merchant_api_key`)

### 3) Run the workflow

Go to GitHub → Actions → `deploy-aws` → Run workflow.

Suggested defaults:
- `aws_region`: your region (example `us-east-1`)
- `name_prefix`: `juno-pay`
- `juno_chain`: `mainnet`
- `allowed_cidrs_json`: `["0.0.0.0/0"]` (or your admin IP range)
- `tf_action`: `apply`
