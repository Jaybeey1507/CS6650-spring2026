output "table_name" {
  description = "DynamoDB carts table name"
  value       = aws_dynamodb_table.carts.name
}

output "table_arn" {
  description = "DynamoDB carts table ARN"
  value       = aws_dynamodb_table.carts.arn
}