resource "terraform_data" "validate" {
  input = {
    name_prefix = var.name_prefix
  }

  lifecycle {
    precondition {
      condition = var.create_network || (
        try(trimspace(var.vpc_id), "") != "" &&
        try(trimspace(var.subnet_id), "") != ""
      )
      error_message = "When create_network=false you must set vpc_id and subnet_id."
    }

    precondition {
      condition     = var.pay_store_driver == "sqlite" || trimspace(var.pay_store_dsn_ssm_param) != ""
      error_message = "pay_store_dsn_ssm_param is required when pay_store_driver is not sqlite."
    }

    precondition {
      condition     = var.pay_store_driver != "mongo" || trimspace(var.pay_store_db) != ""
      error_message = "pay_store_db is required when pay_store_driver=mongo."
    }

    precondition {
      condition     = !var.enable_demo_app || try(trimspace(var.image_demo_app), "") != ""
      error_message = "image_demo_app is required when enable_demo_app=true."
    }

    precondition {
      condition     = !var.enable_rds_postgres || length(var.rds_subnet_ids) >= 2
      error_message = "rds_subnet_ids must include at least 2 subnets when enable_rds_postgres=true."
    }

    precondition {
      condition     = !var.enable_msk || length(var.msk_subnet_ids) >= 2
      error_message = "msk_subnet_ids must include at least 2 subnets when enable_msk=true."
    }

    precondition {
      condition     = trimspace(var.domain_name) == "" || trimspace(var.route53_zone_id) != ""
      error_message = "route53_zone_id is required when domain_name is set."
    }

    precondition {
      condition     = trimspace(var.route53_zone_id) == "" || trimspace(var.domain_name) != ""
      error_message = "domain_name is required when route53_zone_id is set."
    }

    precondition {
      condition     = trimspace(var.domain_name) == "" || trimspace(var.route53_zone_id) == "" || var.demo_port == 80
      error_message = "demo_port must be 80 when domain_name and route53_zone_id are set (required for automatic TLS)."
    }
  }
}
