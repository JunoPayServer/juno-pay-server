data "aws_ami" "al2023" {
  most_recent = true
  owners      = ["amazon"]

  filter {
    name   = "name"
    values = ["al2023-ami-*-x86_64"]
  }
}

resource "aws_security_group" "host" {
  name        = "${var.name_prefix}-host"
  description = "juno-pay-server host SG"
  vpc_id      = var.vpc_id

  ingress {
    description = "Pay server HTTP"
    from_port   = var.pay_server_port
    to_port     = var.pay_server_port
    protocol    = "tcp"
    cidr_blocks = var.allowed_cidrs
  }

  dynamic "ingress" {
    for_each = length(var.ssh_allowed_cidrs) > 0 ? [1] : []
    content {
      description = "SSH"
      from_port   = 22
      to_port     = 22
      protocol    = "tcp"
      cidr_blocks = var.ssh_allowed_cidrs
    }
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name = "${var.name_prefix}-host"
  }
}

data "aws_iam_policy_document" "assume_ec2" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ec2.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "host" {
  name               = "${var.name_prefix}-host"
  assume_role_policy = data.aws_iam_policy_document.assume_ec2.json
}

resource "aws_iam_role_policy_attachment" "ssm_core" {
  role       = aws_iam_role.host.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_role_policy_attachment" "ecr_readonly" {
  role       = aws_iam_role.host.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

data "aws_iam_policy_document" "host_inline" {
  statement {
    sid     = "ReadSSMParameters"
    actions = ["ssm:GetParameter", "ssm:GetParameters"]
    resources = [
      "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${var.admin_password_ssm_param}",
      "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter${var.token_key_ssm_param}",
    ]
  }

  statement {
    sid       = "DecryptSSMDefaultKey"
    actions   = ["kms:Decrypt"]
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "kms:ViaService"
      values   = ["ssm.${var.aws_region}.amazonaws.com"]
    }
  }

  dynamic "statement" {
    for_each = var.enable_rds_postgres ? [1] : []
    content {
      sid       = "ReadRDSSecret"
      actions   = ["secretsmanager:GetSecretValue"]
      resources = [aws_db_instance.junoscan[0].master_user_secret[0].secret_arn]
    }
  }
}

resource "aws_iam_role_policy" "host_inline" {
  name   = "${var.name_prefix}-host-inline"
  role   = aws_iam_role.host.id
  policy = data.aws_iam_policy_document.host_inline.json
}

resource "aws_iam_instance_profile" "host" {
  name = "${var.name_prefix}-host"
  role = aws_iam_role.host.name
}

locals {
  compose_yml = templatefile("${path.module}/templates/docker-compose.yml.tftpl", {
    image_junocashd     = var.image_junocashd
    image_juno_scan     = var.image_juno_scan
    image_juno_pay      = var.image_juno_pay_server
    pay_server_port     = var.pay_server_port
    juno_chain          = var.juno_chain
    juno_scan_ua_hrp    = var.juno_scan_ua_hrp
    juno_scan_confirms  = var.juno_scan_confirmations
    enable_rds_postgres = var.enable_rds_postgres
    rds_endpoint        = var.enable_rds_postgres ? aws_db_instance.junoscan[0].address : ""
    rds_port            = var.enable_rds_postgres ? aws_db_instance.junoscan[0].port : 0
    rds_db_name         = var.enable_rds_postgres ? aws_db_instance.junoscan[0].db_name : ""
    rds_username        = var.enable_rds_postgres ? aws_db_instance.junoscan[0].username : ""
    rds_secret_arn      = var.enable_rds_postgres ? aws_db_instance.junoscan[0].master_user_secret[0].secret_arn : ""
  })
}

resource "aws_instance" "host" {
  ami                         = data.aws_ami.al2023.id
  instance_type               = var.instance_type
  subnet_id                   = var.subnet_id
  vpc_security_group_ids      = [aws_security_group.host.id]
  iam_instance_profile        = aws_iam_instance_profile.host.name
  key_name                    = var.ssh_key_name
  associate_public_ip_address = true

  root_block_device {
    volume_size = var.root_volume_gb
    volume_type = "gp3"
  }

  user_data = templatefile("${path.module}/templates/user-data.sh.tftpl", {
    name_prefix              = var.name_prefix
    aws_region               = var.aws_region
    pay_server_port          = var.pay_server_port
    admin_password_ssm_param = var.admin_password_ssm_param
    token_key_ssm_param      = var.token_key_ssm_param
    rds_secret_arn           = var.enable_rds_postgres ? aws_db_instance.junoscan[0].master_user_secret[0].secret_arn : ""
    docker_compose_yml       = local.compose_yml
  })
  user_data_replace_on_change = true

  tags = {
    Name = "${var.name_prefix}-host"
  }
}

resource "aws_security_group" "rds" {
  count       = var.enable_rds_postgres ? 1 : 0
  name        = "${var.name_prefix}-rds"
  description = "RDS access for juno-scan"
  vpc_id      = var.vpc_id

  ingress {
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.host.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_db_subnet_group" "junoscan" {
  count      = var.enable_rds_postgres ? 1 : 0
  name       = "${var.name_prefix}-junoscan"
  subnet_ids = var.rds_subnet_ids
}

resource "aws_db_instance" "junoscan" {
  count                   = var.enable_rds_postgres ? 1 : 0
  identifier              = "${var.name_prefix}-junoscan"
  engine                  = "postgres"
  engine_version          = "16"
  instance_class          = var.rds_instance_class
  allocated_storage       = 100
  storage_type            = "gp3"
  storage_encrypted       = true
  publicly_accessible     = false
  backup_retention_period = 7

  db_name                     = "junoscan"
  username                    = "junoscan"
  manage_master_user_password = true

  db_subnet_group_name   = aws_db_subnet_group.junoscan[0].name
  vpc_security_group_ids = [aws_security_group.rds[0].id]

  skip_final_snapshot = true
}

resource "aws_security_group" "msk" {
  count       = var.enable_msk ? 1 : 0
  name        = "${var.name_prefix}-msk"
  description = "MSK access for juno-pay-server"
  vpc_id      = var.vpc_id

  ingress {
    description     = "Kafka plaintext"
    from_port       = 9092
    to_port         = 9092
    protocol        = "tcp"
    security_groups = [aws_security_group.host.id]
  }

  ingress {
    description     = "Kafka TLS"
    from_port       = 9094
    to_port         = 9094
    protocol        = "tcp"
    security_groups = [aws_security_group.host.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_msk_cluster" "events" {
  count                  = var.enable_msk ? 1 : 0
  cluster_name           = "${var.name_prefix}-events"
  kafka_version          = var.msk_kafka_version
  number_of_broker_nodes = length(var.msk_subnet_ids)

  broker_node_group_info {
    instance_type   = var.msk_instance_type
    client_subnets  = var.msk_subnet_ids
    security_groups = [aws_security_group.msk[0].id]

    storage_info {
      ebs_storage_info {
        volume_size = var.msk_ebs_volume_gb
      }
    }
  }

  encryption_info {
    encryption_in_transit {
      client_broker = "TLS_PLAINTEXT"
      in_cluster    = true
    }
  }

  client_authentication {
    unauthenticated = true
  }
}
