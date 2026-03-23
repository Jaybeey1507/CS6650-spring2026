output "vpc_id" {
  description = "Default VPC ID"
  value       = data.aws_vpc.default.id
}

output "subnet_ids" {
  description = "IDs of the default VPC subnets for ECS/public resources"
  value       = data.aws_subnets.default.ids
}

output "private_subnet_ids" {
  description = "IDs of the private subnets for RDS"
  value       = aws_subnet.private[*].id
}

output "security_group_id" {
  description = "Security group ID for ECS"
  value       = aws_security_group.this.id
}