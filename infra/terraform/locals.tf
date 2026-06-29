locals {
  name = "${var.project}-${var.environment}"

  azs = slice(data.aws_availability_zones.available.names, 0, var.az_count)

  ecr_repositories = toset([
    "api",
    "worker",
    "reaper",
    "migrate",
    "demo",
    "dashboard",
  ])

  app_static_environment = concat([
    {
      name  = "STORAGE_ENDPOINT"
      value = "https://s3.${var.aws_region}.amazonaws.com"
    },
    {
      name  = "STORAGE_BUCKET"
      value = aws_s3_bucket.storage.bucket
    },
    {
      name  = "CDN_BASE_URL"
      value = "https://${aws_cloudfront_distribution.main.domain_name}"
    },
    {
      name  = "UPLOAD_PREFIX"
      value = "raw"
    },
    {
      name  = "OUTPUT_PREFIX"
      value = "outputs"
    },
    {
      name  = "OUTPUT_CACHE_CONTROL"
      value = "public, max-age=86400, immutable"
    },
    ], var.cors_allowed_origins == "" ? [] : [
    {
      name  = "CORS_ALLOWED_ORIGINS"
      value = var.cors_allowed_origins
    },
  ])

  api_environment = concat(local.app_static_environment, [
    {
      name  = "UPLOAD_SIZE_LIMIT_BYTES"
      value = tostring(var.upload_size_limit_bytes)
    },
  ])

  app_secrets = [
    for name, arn in var.app_secret_arns : {
      name      = name
      valueFrom = arn
    }
  ]

  frontend_secrets = [
    for name, arn in var.frontend_secret_arns : {
      name      = name
      valueFrom = arn
    }
  ]

  image_uri = {
    for repo in local.ecr_repositories :
    repo => lookup(var.image_overrides, repo, "${aws_ecr_repository.app[repo].repository_url}:${var.container_image_tag}")
  }

  ecs_subnet_ids = var.ecs_assign_public_ip ? [for subnet in aws_subnet.public : subnet.id] : [for subnet in aws_subnet.private_app : subnet.id]

  tags = {
    Project     = var.project
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}
