# Fetch the default VPC
data "aws_vpc" "default" {
  default = true
}

# Existing default subnets for ECS/public-facing resources
data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

# Get available AZs so we can place private subnets across at least 2 AZs
data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  private_subnet_cidrs = [
    cidrsubnet(data.aws_vpc.default.cidr_block, 8, 240),
    cidrsubnet(data.aws_vpc.default.cidr_block, 8, 241)
  ]
}

# Create two private subnets for RDS
resource "aws_subnet" "private" {
  count = 2

  vpc_id                  = data.aws_vpc.default.id
  cidr_block              = local.private_subnet_cidrs[count.index]
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = false

  tags = {
    Name = "${var.service_name}-private-${count.index + 1}"
  }
}

# Private route table with no internet route
resource "aws_route_table" "private" {
  vpc_id = data.aws_vpc.default.id

  tags = {
    Name = "${var.service_name}-private-rt"
  }
}

resource "aws_route_table_association" "private" {
  count = 2

  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private.id
}

# ECS security group
resource "aws_security_group" "this" {
  name        = "${var.service_name}-sg"
  description = "Allow inbound on ${var.container_port}"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port   = var.container_port
    to_port     = var.container_port
    protocol    = "tcp"
    cidr_blocks = var.cidr_blocks
    description = "Allow HTTP traffic"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound"
  }
}