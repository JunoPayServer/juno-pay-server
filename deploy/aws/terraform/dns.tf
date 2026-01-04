resource "aws_route53_record" "apex" {
  count   = local.enable_caddy ? 1 : 0
  zone_id = var.route53_zone_id
  name    = var.domain_name
  type    = "A"
  ttl     = 60
  records = [aws_eip.host.public_ip]
}

resource "aws_route53_record" "www" {
  count   = local.enable_caddy ? 1 : 0
  zone_id = var.route53_zone_id
  name    = "www.${var.domain_name}"
  type    = "A"
  ttl     = 60
  records = [aws_eip.host.public_ip]
}
