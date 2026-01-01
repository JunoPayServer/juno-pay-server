output "instance_public_ip" {
  value       = aws_instance.host.public_ip
  description = "Public IP of the docker host."
}

output "admin_url" {
  value       = "http://${aws_instance.host.public_ip}:${var.pay_server_port}/admin/"
  description = "Admin dashboard URL (no TLS)."
}

output "rds_endpoint" {
  value       = var.enable_rds_postgres ? aws_db_instance.junoscan[0].address : null
  description = "RDS Postgres endpoint (if enabled)."
}

output "rds_secret_arn" {
  value       = var.enable_rds_postgres ? aws_db_instance.junoscan[0].master_user_secret[0].secret_arn : null
  description = "Secrets Manager ARN for the RDS master password (if enabled)."
}

output "msk_bootstrap_brokers" {
  value       = var.enable_msk ? aws_msk_cluster.events[0].bootstrap_brokers : null
  description = "MSK bootstrap brokers (plaintext, if enabled)."
}

output "msk_bootstrap_brokers_tls" {
  value       = var.enable_msk ? aws_msk_cluster.events[0].bootstrap_brokers_tls : null
  description = "MSK bootstrap brokers (TLS, if enabled)."
}
