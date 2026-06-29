output "ecr_repository_urls" {
  description = "ECR repository URLs keyed by service name."
  value = {
    for name, repo in aws_ecr_repository.app : name => repo.repository_url
  }
}

output "alb_dns_name" {
  description = "ALB DNS name for API ingress."
  value       = aws_lb.app.dns_name
}

output "cloudfront_domain_name" {
  description = "CloudFront distribution domain name."
  value       = aws_cloudfront_distribution.main.domain_name
}

output "storage_bucket" {
  description = "Private S3 storage bucket."
  value       = aws_s3_bucket.storage.bucket
}

output "rds_endpoint" {
  description = "RDS PostgreSQL endpoint."
  value       = aws_db_instance.postgres.endpoint
}

output "redis_primary_endpoint" {
  description = "ElastiCache Redis primary endpoint."
  value       = aws_elasticache_replication_group.redis.primary_endpoint_address
}

output "ecs_cluster_name" {
  description = "ECS cluster name."
  value       = aws_ecs_cluster.main.name
}

output "migrate_task_definition_arn" {
  description = "One-off ECS task definition ARN for database migrations."
  value       = aws_ecs_task_definition.migrate.arn
}
