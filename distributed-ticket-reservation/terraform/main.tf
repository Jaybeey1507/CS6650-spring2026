terraform {
  required_version = ">= 1.5.0"

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

variable "aws_region" {
  type    = string
  default = "us-west-2"
}

variable "project_name" {
  type    = string
  default = "ticket-reservation"
}

variable "container_port" {
  type    = number
  default = 8081
}

variable "desired_count" {
  type    = number
  default = 1
}

data "aws_vpc" "default" {
  default = true
}

data "aws_subnets" "default" {
  filter {
    name   = "vpc-id"
    values = [data.aws_vpc.default.id]
  }
}

data "aws_subnet" "default_details" {
  for_each = toset(data.aws_subnets.default.ids)
  id       = each.value
}

locals {
  subnet_ids_by_az = {
    for s in data.aws_subnet.default_details :
    s.availability_zone => s.id...
  }

  alb_subnet_ids = [for az, ids in local.subnet_ids_by_az : ids[0]]
}

data "aws_iam_role" "labrole" {
  name = "LabRole"
}

resource "aws_cloudwatch_log_group" "reservation" {
  name              = "/ecs/ticket-reservation"
  retention_in_days = 7
}

resource "aws_dynamodb_table" "seats" {
  name         = "TicketSeats"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "EventID"
  range_key    = "SeatID"

  attribute {
    name = "EventID"
    type = "S"
  }

  attribute {
    name = "SeatID"
    type = "S"
  }
}

resource "aws_dynamodb_table" "holds" {
  name         = "TicketHolds"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "HoldID"

  attribute {
    name = "HoldID"
    type = "S"
  }
}

resource "aws_dynamodb_table" "reservations" {
  name         = "TicketReservations"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "ReservationID"

  attribute {
    name = "ReservationID"
    type = "S"
  }
}

resource "aws_ecr_repository" "reservation" {
  name = "ticket-reservation"
  image_scanning_configuration {
    scan_on_push = false
  }
  force_delete = true
}

resource "aws_ecs_cluster" "main" {
  name = "ticket-reservation-cluster"
}

resource "aws_security_group" "alb" {
  name        = "ticket-reservation-alb-sg"
  description = "ALB security group"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group" "ecs" {
  name        = "ticket-reservation-ecs-sg"
  description = "ECS service security group"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    from_port       = var.container_port
    to_port         = var.container_port
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_lb" "main" {
  name               = "ticket-reservation-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = local.alb_subnet_ids
}

resource "aws_lb_target_group" "reservation" {
  name        = "ticket-reservation-tg"
  port        = var.container_port
  protocol    = "HTTP"
  target_type = "ip"
  vpc_id      = data.aws_vpc.default.id

  health_check {
    path                = "/health"
    protocol            = "HTTP"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.main.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.reservation.arn
  }
}

resource "aws_ecs_task_definition" "reservation" {
  family                   = "ticket-reservation-task"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = "512"
  memory                   = "1024"
  execution_role_arn       = data.aws_iam_role.labrole.arn
  task_role_arn            = data.aws_iam_role.labrole.arn

  container_definitions = jsonencode([
    {
      name      = "reservation"
      image     = "${aws_ecr_repository.reservation.repository_url}:latest"
      essential = true

      portMappings = [
        {
          containerPort = var.container_port
          hostPort      = var.container_port
          protocol      = "tcp"
        }
      ]

      environment = [
        { name = "STORE_BACKEND", value = "dynamo" },
        { name = "DDB_SEATS_TABLE", value = aws_dynamodb_table.seats.name },
        { name = "DDB_HOLDS_TABLE", value = aws_dynamodb_table.holds.name },
        { name = "DDB_RESERVATIONS_TABLE", value = aws_dynamodb_table.reservations.name },
        { name = "AWS_REGION", value = var.aws_region }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          awslogs-group         = aws_cloudwatch_log_group.reservation.name
          awslogs-region        = var.aws_region
          awslogs-stream-prefix = "ecs"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "reservation" {
  name            = "ticket-reservation-service"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.reservation.arn
  desired_count   = var.desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = data.aws_subnets.default.ids
    security_groups  = [aws_security_group.ecs.id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.reservation.arn
    container_name   = "reservation"
    container_port   = var.container_port
  }

  depends_on = [aws_lb_listener.http]
}

output "ecr_repo_url" {
  value = aws_ecr_repository.reservation.repository_url
}

output "alb_dns_name" {
  value = aws_lb.main.dns_name
}

output "seats_table" {
  value = aws_dynamodb_table.seats.name
}

output "holds_table" {
  value = aws_dynamodb_table.holds.name
}

output "reservations_table" {
  value = aws_dynamodb_table.reservations.name
}