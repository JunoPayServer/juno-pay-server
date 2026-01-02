variable "aws_region" {
  type        = string
  description = "AWS region."
  default     = "us-east-1"
}

variable "name_prefix" {
  type        = string
  description = "Name prefix for AWS resources."
  default     = "juno-pay"
}

variable "vpc_id" {
  type        = string
  description = "VPC ID to deploy into."
}

variable "subnet_id" {
  type        = string
  description = "Subnet ID for the EC2 instance."
}

variable "allowed_cidrs" {
  type        = list(string)
  description = "CIDRs allowed to reach the pay server HTTP port."
  default     = ["0.0.0.0/0"]
}

variable "ssh_allowed_cidrs" {
  type        = list(string)
  description = "CIDRs allowed to SSH (port 22). Empty disables SSH ingress."
  default     = []
}

variable "ssh_key_name" {
  type        = string
  description = "Optional EC2 key pair name for SSH."
  default     = null
}

variable "instance_type" {
  type        = string
  description = "EC2 instance type."
  default     = "t3.large"
}

variable "root_volume_gb" {
  type        = number
  description = "Root EBS volume size (GiB)."
  default     = 200
}

variable "pay_server_port" {
  type        = number
  description = "Host port to expose juno-pay-server on."
  default     = 8080
}

variable "admin_password_ssm_param" {
  type        = string
  description = "SSM parameter name containing JUNO_PAY_ADMIN_PASSWORD."
  default     = "/juno-pay/admin_password"
}

variable "token_key_ssm_param" {
  type        = string
  description = "SSM parameter name containing JUNO_PAY_TOKEN_KEY_HEX."
  default     = "/juno-pay/token_key_hex"
}

variable "pay_store_driver" {
  type        = string
  description = "juno-pay-server store driver: sqlite|postgres|mysql|mongo."
  default     = "sqlite"

  validation {
    condition     = contains(["sqlite", "postgres", "mysql", "mongo"], var.pay_store_driver)
    error_message = "pay_store_driver must be one of: sqlite|postgres|mysql|mongo."
  }
}

variable "pay_store_dsn_ssm_param" {
  type        = string
  description = "Optional SSM parameter name containing JUNO_PAY_STORE_DSN (required for non-sqlite drivers)."
  default     = ""

  validation {
    condition     = var.pay_store_driver == "sqlite" || var.pay_store_dsn_ssm_param != ""
    error_message = "pay_store_dsn_ssm_param is required when pay_store_driver is not sqlite."
  }
}

variable "pay_store_db" {
  type        = string
  description = "MongoDB database name for pay_store_driver=mongo."
  default     = ""

  validation {
    condition     = var.pay_store_driver != "mongo" || var.pay_store_db != ""
    error_message = "pay_store_db is required when pay_store_driver=mongo."
  }
}

variable "pay_store_prefix" {
  type        = string
  description = "Optional table/collection prefix for juno-pay-server store (namespaces tables when sharing a DB)."
  default     = ""
}

variable "image_juno_pay_server" {
  type        = string
  description = "Docker image URI for juno-pay-server (recommended: ECR)."
}

variable "image_junocashd" {
  type        = string
  description = "Docker image URI for junocashd."
}

variable "image_juno_scan" {
  type        = string
  description = "Docker image URI for juno-scan."
}

variable "juno_chain" {
  type        = string
  description = "Juno chain: mainnet|testnet|regtest."
  default     = "mainnet"
}

variable "juno_scan_ua_hrp" {
  type        = string
  description = "Unified address HRP for juno-scan."
  default     = "j"
}

variable "juno_scan_confirmations" {
  type        = number
  description = "Confirmations required for confirmed deposit events."
  default     = 100
}

variable "enable_rds_postgres" {
  type        = bool
  description = "Create an RDS Postgres instance for juno-scan (optional)."
  default     = false
}

variable "rds_subnet_ids" {
  type        = list(string)
  description = "Subnet IDs for the RDS subnet group (required if enable_rds_postgres=true)."
  default     = []
}

variable "rds_instance_class" {
  type        = string
  description = "RDS instance class."
  default     = "db.t4g.medium"
}

variable "enable_msk" {
  type        = bool
  description = "Create an MSK (Kafka) cluster (optional)."
  default     = false
}

variable "msk_subnet_ids" {
  type        = list(string)
  description = "Subnet IDs for MSK brokers (required if enable_msk=true)."
  default     = []

  validation {
    condition     = !var.enable_msk || length(var.msk_subnet_ids) >= 2
    error_message = "msk_subnet_ids must include at least 2 subnets when enable_msk=true."
  }
}

variable "msk_kafka_version" {
  type        = string
  description = "Kafka version for MSK."
  default     = "3.5.1"
}

variable "msk_instance_type" {
  type        = string
  description = "MSK broker instance type."
  default     = "kafka.t3.small"
}

variable "msk_ebs_volume_gb" {
  type        = number
  description = "EBS volume size per broker (GiB)."
  default     = 100
}
