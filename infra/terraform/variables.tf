variable "project" {
  description = "Project name used for AWS resource names."
  type        = string
  default     = "transcodex"
}

variable "environment" {
  description = "Deployment environment name."
  type        = string
  default     = "demo"
}

variable "aws_region" {
  description = "AWS region for regional resources."
  type        = string
  default     = "ap-south-1"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC."
  type        = string
  default     = "10.40.0.0/16"
}

variable "az_count" {
  description = "Number of Availability Zones to use."
  type        = number
  default     = 2

  validation {
    condition     = var.az_count >= 2
    error_message = "Use at least two Availability Zones for ALB, RDS, and ElastiCache."
  }
}

variable "enable_nat_gateway" {
  description = "Create NAT gateways for private subnet outbound internet access. Keep false for the low-cost demo path."
  type        = bool
  default     = false
}

variable "ecs_assign_public_ip" {
  description = "Assign public IPs and place ECS tasks in public subnets. This avoids NAT Gateway cost for the demo while security groups still block direct inbound app access."
  type        = bool
  default     = true
}

variable "container_image_tag" {
  description = "Default image tag used by ECS task definitions."
  type        = string
  default     = "latest"
}

variable "image_overrides" {
  description = "Optional full image URI overrides keyed by api, worker, reaper, demo, or dashboard."
  type        = map(string)
  default     = {}
}

variable "app_secret_arns" {
  description = "SSM parameter or Secrets Manager ARNs injected into app containers as environment secrets. Keys become env var names."
  type        = map(string)
  default     = {}
}

variable "frontend_secret_arns" {
  description = "SSM parameter or Secrets Manager ARNs injected into frontend Nginx containers, if frontend ECS services are enabled."
  type        = map(string)
  default     = {}
}

variable "cors_allowed_origins" {
  description = "Allowed browser origins for the API CORS config."
  type        = string
  default     = ""
}

variable "upload_size_limit_bytes" {
  description = "Maximum upload size accepted by the API."
  type        = number
  default     = 524288000
}

variable "db_name" {
  description = "RDS PostgreSQL database name."
  type        = string
  default     = "transcodex"
}

variable "db_username" {
  description = "RDS PostgreSQL master username."
  type        = string
  default     = "transcodex"
}

variable "db_password" {
  description = "RDS PostgreSQL master password. Prefer passing this via TF_VAR_db_password or your CI secret store."
  type        = string
  sensitive   = true
}

variable "db_instance_class" {
  description = "RDS instance class."
  type        = string
  default     = "db.t4g.micro"
}

variable "db_allocated_storage_gb" {
  description = "RDS allocated storage in GB."
  type        = number
  default     = 20
}

variable "db_deletion_protection" {
  description = "Enable deletion protection for RDS."
  type        = bool
  default     = false
}

variable "redis_node_type" {
  description = "ElastiCache Redis node type."
  type        = string
  default     = "cache.t4g.micro"
}

variable "redis_num_cache_clusters" {
  description = "Number of cache clusters in the Redis replication group."
  type        = number
  default     = 1
}

variable "api_desired_count" {
  description = "Desired ECS API task count."
  type        = number
  default     = 1
}

variable "deploy_app_services" {
  description = "Create ECS services for API, worker, and reaper. Keep false for the first apply until images and runtime secrets exist."
  type        = bool
  default     = false
}

variable "worker_desired_count" {
  description = "Desired ECS worker task count."
  type        = number
  default     = 1
}

variable "reaper_desired_count" {
  description = "Desired ECS reaper task count. Multiple replicas are safe because the app uses a Redis leadership lock."
  type        = number
  default     = 1
}

variable "api_cpu" {
  description = "API Fargate CPU units."
  type        = number
  default     = 256
}

variable "api_memory" {
  description = "API Fargate memory in MB."
  type        = number
  default     = 512
}

variable "worker_cpu" {
  description = "Worker Fargate CPU units."
  type        = number
  default     = 1024
}

variable "worker_memory" {
  description = "Worker Fargate memory in MB."
  type        = number
  default     = 2048
}

variable "reaper_cpu" {
  description = "Reaper Fargate CPU units."
  type        = number
  default     = 256
}

variable "reaper_memory" {
  description = "Reaper Fargate memory in MB."
  type        = number
  default     = 512
}

variable "deploy_frontend_services" {
  description = "Deploy demo and dashboard as ECS/Nginx services behind the ALB. Set false if serving built assets from S3/CloudFront instead."
  type        = bool
  default     = false
}

variable "demo_desired_count" {
  description = "Desired ECS demo frontend task count."
  type        = number
  default     = 1
}

variable "dashboard_desired_count" {
  description = "Desired ECS dashboard frontend task count."
  type        = number
  default     = 1
}

variable "certificate_arn" {
  description = "Optional ACM certificate ARN for CloudFront aliases. Must be in us-east-1 for CloudFront."
  type        = string
  default     = ""
}

variable "cloudfront_aliases" {
  description = "Optional CloudFront alternate domain names."
  type        = list(string)
  default     = []
}

variable "cloudfront_price_class" {
  description = "CloudFront price class."
  type        = string
  default     = "PriceClass_100"
}

variable "frontend_read_prefixes" {
  description = "Extra S3 prefixes CloudFront may read for static frontend assets."
  type        = list(string)
  default     = ["demo/*", "dashboard/*", "favicon.ico"]
}
