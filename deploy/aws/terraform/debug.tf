resource "terraform_data" "bootstrap_diagnostics" {
  triggers_replace = {
    instance_id = aws_instance.host.id
  }

  provisioner "local-exec" {
    interpreter = ["/bin/bash", "-c"]
    command     = <<EOT
set -euo pipefail

echo "Bootstrap diagnostics for instance: ${aws_instance.host.id}"

sleep 60

OUT="$(aws ec2 get-console-output --region "${var.aws_region}" --instance-id "${aws_instance.host.id}" --latest --query Output --output text 2>/dev/null || true)"
if [[ -z "$OUT" || "$OUT" == "None" ]]; then
  echo "No EC2 console output available (or not permitted)."
  exit 0
fi

echo "$OUT" | base64 --decode | tail -n 200 || true
EOT
  }
}

