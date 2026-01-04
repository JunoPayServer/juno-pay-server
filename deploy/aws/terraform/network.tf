data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  az = length(data.aws_availability_zones.available.names) > 0 ? data.aws_availability_zones.available.names[0] : null
}

resource "aws_vpc" "main" {
  count                = var.create_network ? 1 : 0
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = {
    Name = "${var.name_prefix}-vpc"
  }
}

resource "aws_internet_gateway" "igw" {
  count  = var.create_network ? 1 : 0
  vpc_id = aws_vpc.main[0].id

  tags = {
    Name = "${var.name_prefix}-igw"
  }
}

resource "aws_subnet" "public" {
  count                   = var.create_network ? 1 : 0
  vpc_id                  = aws_vpc.main[0].id
  cidr_block              = var.public_subnet_cidr
  availability_zone       = local.az
  map_public_ip_on_launch = true

  tags = {
    Name = "${var.name_prefix}-public-subnet"
  }
}

resource "aws_route_table" "public" {
  count  = var.create_network ? 1 : 0
  vpc_id = aws_vpc.main[0].id

  tags = {
    Name = "${var.name_prefix}-public-rt"
  }
}

resource "aws_route" "public_internet" {
  count                  = var.create_network ? 1 : 0
  route_table_id         = aws_route_table.public[0].id
  destination_cidr_block = "0.0.0.0/0"
  gateway_id             = aws_internet_gateway.igw[0].id
}

resource "aws_route_table_association" "public" {
  count          = var.create_network ? 1 : 0
  subnet_id      = aws_subnet.public[0].id
  route_table_id = aws_route_table.public[0].id
}

locals {
  effective_vpc_id    = var.create_network ? aws_vpc.main[0].id : var.vpc_id
  effective_subnet_id = var.create_network ? aws_subnet.public[0].id : var.subnet_id
  effective_az        = var.create_network ? local.az : data.aws_subnet.existing[0].availability_zone
}

data "aws_subnet" "existing" {
  count = var.create_network ? 0 : 1
  id    = var.subnet_id
}
