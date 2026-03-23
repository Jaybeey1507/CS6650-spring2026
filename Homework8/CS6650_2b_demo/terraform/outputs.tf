output "ecs_cluster_name" {
  description = "Name of the created ECS cluster"
  value       = module.ecs.cluster_name
}

output "ecs_service_name" {
  description = "Name of the running ECS service"
  value       = module.ecs.service_name
}

output "rds_endpoint" {
  description = "RDS endpoint for MySQL connection"
  value       = module.rds.db_endpoint
}

output "rds_port" {
  description = "RDS MySQL port"
  value       = module.rds.db_port
}

output "rds_db_name" {
  description = "RDS database name"
  value       = module.rds.db_name
}

output "dynamodb_table_name" {
  description = "DynamoDB carts table name"
  value       = module.dynamodb.table_name
}

output "dynamodb_table_arn" {
  description = "DynamoDB carts table ARN"
  value       = module.dynamodb.table_arn
}