resource "aws_security_group" "rds" {
  name        = "${var.service_name}-rds-sg"
  description = "Allow MySQL access from ECS tasks only"
  vpc_id      = var.vpc_id

  ingress {
    from_port       = var.db_port
    to_port         = var.db_port
    protocol        = "tcp"
    security_groups = [var.ecs_security_group_id]
    description     = "Allow MySQL from ECS security group"
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
    description = "Allow all outbound"
  }

  tags = {
    Name = "${var.service_name}-rds-sg"
  }
}

resource "aws_db_subnet_group" "this" {
  name       = "${lower(var.service_name)}-db-subnet-group"
  subnet_ids = var.private_subnet_ids

  tags = {
    Name = "${var.service_name}-db-subnet-group"
  }
}

resource "aws_db_instance" "this" {
  identifier        = "${lower(var.service_name)}-mysql"
  engine            = "mysql"
  engine_version    = var.db_engine_version
  instance_class    = var.db_instance_class
  allocated_storage = var.db_allocated_storage
  storage_type      = "gp2"

  db_name  = var.db_name
  username = var.db_username
  password = var.db_password
  port     = var.db_port

  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.rds.id]

  publicly_accessible = false
  multi_az            = false
  skip_final_snapshot = true
  deletion_protection = false

  backup_retention_period = 0
  apply_immediately       = true

  tags = {
    Name = "${var.service_name}-mysql"
  }
}