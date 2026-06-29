# Transcodex Terraform

This directory provisions the AWS demo deployment for Transcodex:

- VPC with public, private app, and private data subnets
- ALB for API ingress
- ECS/Fargate services for API, worker, and reaper
- Optional ECS/Nginx services for demo and dashboard frontends, if you do not serve static builds from S3
- RDS PostgreSQL
- ElastiCache Redis
- Private S3 bucket
- CloudFront distribution with S3 Origin Access Control
- ECR repositories
- ECS IAM roles and CloudWatch logs

The demo defaults avoid NAT Gateway cost. ECS tasks run in public subnets with public IPs for outbound access to ECR, CloudWatch, SSM, and AWS APIs, while inbound traffic is still restricted to the ALB security group. RDS and Redis remain in private data subnets.

## Prerequisites

- Terraform `>= 1.6`
- AWS CLI configured with deploy permissions
- An ACM certificate in `us-east-1` if using custom CloudFront aliases

## First Apply

Copy the example variables:

```bash
cp terraform.tfvars.example terraform.tfvars
```

Set `db_password` through your shell or `terraform.tfvars`:

```bash
export TF_VAR_db_password='replace-with-a-real-password'
```

Initialize and apply:

```bash
terraform init
terraform plan
terraform apply
```

The first apply should keep `deploy_app_services = false`. That creates ECR, RDS, Redis, S3, CloudFront, and task definitions without asking ECS to pull images that do not exist yet. A practical deployment flow is:

1. Apply once to create RDS/Redis and read the outputs.
2. Create SSM parameters or Secrets Manager secrets for `DATABASE_URL` and `REDIS_URL`.
3. Add those ARNs to `app_secret_arns`.
4. Build and push Docker images to the ECR URLs in the outputs.
5. Set `deploy_app_services = true`.
6. Apply again so ECS creates the API, worker, and reaper services.

For a production-like private app subnet deployment, set `enable_nat_gateway = true` and `ecs_assign_public_ip = false`, or add the required VPC interface endpoints for ECR, CloudWatch Logs, and SSM/Secrets Manager.

## Image Push

Use the output `ecr_repository_urls` and push each image:

```bash
aws ecr get-login-password --region <region> \
  | docker login --username AWS --password-stdin <account-id>.dkr.ecr.<region>.amazonaws.com

docker build -f api/Dockerfile -t <api-repo>:latest ../..
docker push <api-repo>:latest

docker build -f worker/Dockerfile -t <worker-repo>:latest ../..
docker push <worker-repo>:latest

docker build -f reaper/Dockerfile -t <reaper-repo>:latest ../..
docker push <reaper-repo>:latest

docker build -f migrations/Dockerfile -t <migrate-repo>:latest ../..
docker push <migrate-repo>:latest
```

Run the same pattern for `frontend/demo/Dockerfile` and `frontend/dashboard/Dockerfile` if `deploy_frontend_services = true`.

## Frontends

The default Terraform path is S3 + CloudFront for frontend assets. Build locally with the production API base and upload the generated files under the bucket prefixes CloudFront can read:

```bash
pnpm --dir ../../frontend/demo install
VITE_API_BASE_URL=https://<cloudfront-domain> pnpm --dir ../../frontend/demo build
aws s3 sync ../../frontend/demo/dist "s3://<storage-bucket>/demo" --delete

pnpm --dir ../../frontend/dashboard install
VITE_API_BASE_URL=https://<cloudfront-domain> pnpm --dir ../../frontend/dashboard build
aws s3 sync ../../frontend/dashboard/dist "s3://<storage-bucket>/dashboard" --delete
```

Set `deploy_frontend_services = true` only if you prefer running the existing Nginx frontend containers behind the ALB.

## Secrets

The app task role uses IAM credentials for S3. Leave `STORAGE_ACCESS_KEY` and `STORAGE_SECRET_KEY` unset in AWS.

Create at least these runtime secrets:

| Env var | Example |
|---|---|
| `DATABASE_URL` | `postgres://transcodex:<password>@<rds-endpoint>:5432/transcodex?sslmode=require` |
| `REDIS_URL` | `redis://<elasticache-primary-endpoint>:6379` |

`STORAGE_ENDPOINT`, `STORAGE_BUCKET`, `CDN_BASE_URL`, prefixes, cache control, and CORS are supplied as plain ECS environment variables by Terraform.

## Migrations

Run migrations as a one-off ECS task after RDS is reachable and before production traffic. The Terraform output `migrate_task_definition_arn` gives the task definition to run.

The task expects `DATABASE_URL` in `app_secret_arns`, and runs:

```bash
migrate -path /migrations -database "$DATABASE_URL" up
```

## Smoke Test

```bash
curl -fsS "https://<cloudfront-domain>/healthz"
curl -fsS -X POST -F "file=@sample.mp4" -F "priority=10" "https://<cloudfront-domain>/uploads"
curl -fsS "https://<cloudfront-domain>/jobs/<job-id>"
```

Confirm:

- Job reaches `completed`
- Output URLs use the CloudFront domain
- Output URLs return `200`
- Direct raw S3 object access fails without AWS credentials
- Dashboard SSE updates if dashboard access is enabled
