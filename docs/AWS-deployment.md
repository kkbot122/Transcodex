# AWS Deployment Runbook

This runbook turns the local Docker Compose stack into a small production-style AWS deployment. It targets a demo or self-managed setup, with Terraform used as the repeatable deployment path.

Terraform scaffold: [infra/terraform/README.md](../infra/terraform/README.md)

## Target Architecture

| Component | AWS service |
|---|---|
| API | ECS/Fargate service behind an ALB |
| Workers | ECS/Fargate service scaled independently |
| Reaper | One small ECS service, or multiple with Redis leadership lock |
| Database | RDS PostgreSQL in private subnets |
| Queue and locks | ElastiCache Redis in private subnets |
| Storage | S3 private bucket |
| CDN | CloudFront with S3 Origin Access Control |
| Frontends | S3 + CloudFront, or the existing Nginx frontend containers |
| Secrets | Secrets Manager or SSM Parameter Store |
| Images | ECR |

## Deployment Order

1. Create a VPC across at least two Availability Zones.
2. Create public subnets for ALB and NAT, private app subnets for ECS, and private data subnets for RDS/Redis.
3. Create narrow security groups:
   - ALB -> API tasks on `8080`
   - API / worker / reaper -> RDS on `5432`
   - API / worker / reaper -> Redis on `6379`
   - API / worker -> S3 through IAM and an optional S3 Gateway VPC endpoint
4. Create the private S3 bucket.
5. Create CloudFront with:
   - `/outputs/*` -> S3 origin with Origin Access Control and long cache TTL
   - `/uploads`, `/jobs/*`, `/internal/*` -> ALB origin with caching disabled
6. Create RDS PostgreSQL and ElastiCache Redis in private data subnets.
7. Store production configuration in Secrets Manager or SSM.
8. Build and push API, worker, reaper, demo frontend, and dashboard images to ECR.
9. Run database migrations as a one-off job.
10. Deploy ECS services for API, workers, and reaper.
11. Deploy frontends to S3/CloudFront or as Nginx containers.
12. Run the production smoke test.

The included Terraform scaffold defaults to a lower-cost demo topology: no NAT Gateway, ECS tasks in public subnets with public IPs for outbound AWS/ECR access, and RDS/Redis in private data subnets. Security groups still prevent direct inbound access to app containers.

## Storage

Use one private S3 bucket with prefixes:

| Prefix | Purpose | Public access |
|---|---|---|
| `raw/` | Original uploads | Private only |
| `outputs/` | Transcoded videos and thumbnails | Readable only through CloudFront |

Keep S3 Block Public Access enabled. Do not use public ACLs. Give the ECS app task role write access to `raw/*` and `outputs/*`; give CloudFront OAC read access only to `outputs/*`.

`CDN_BASE_URL` should be the CloudFront distribution URL or custom domain, with no trailing slash.

## Runtime Configuration

Store these values in Secrets Manager or SSM and inject them into ECS tasks:

| Variable | Production value |
|---|---|
| `DATABASE_URL` | RDS PostgreSQL connection string |
| `REDIS_URL` | ElastiCache Redis connection string |
| `STORAGE_ENDPOINT` | `https://s3.<region>.amazonaws.com` |
| `STORAGE_BUCKET` | S3 bucket name |
| `CDN_BASE_URL` | CloudFront URL or custom domain |
| `CORS_ALLOWED_ORIGINS` | Production frontend origins |

Prefer ECS task roles over static AWS keys. Leave `STORAGE_ACCESS_KEY` and `STORAGE_SECRET_KEY` unset in AWS so the storage client uses task-role credentials. Keep those variables set only for local MinIO or explicit static-key deployments.

## ECS Services

Deploy three separately scalable services:

| Service | Desired count | Notes |
|---|---:|---|
| API | `1+` | Behind ALB. Use `GET /healthz` as the health check. |
| Worker | `1+` | CPU/memory heavier than API because FFmpeg is the bottleneck. Scale by queue depth or CPU. |
| Reaper | `1` | Multiple replicas are safe because Redis leadership lock prevents duplicate sweeps. |

Suggested starting sizes for a demo:

| Service | CPU | Memory |
|---|---:|---:|
| API | `0.25 vCPU` | `512 MB` |
| Reaper | `0.25 vCPU` | `512 MB` |
| Worker | `1 vCPU` | `2 GB` |

Tune workers based on the input video size and FFmpeg runtime.

## CloudFront Routing

Use separate cache behavior policies:

| Path | Origin | Cache |
|---|---|---|
| `/outputs/*` | S3 | Long TTL, immutable outputs |
| `/uploads` | ALB/API | Disabled |
| `/jobs/*` | ALB/API | Disabled |
| `/internal/*` | ALB/API | Disabled and restricted/authenticated |

The dashboard currently needs `/internal/stats/stream` for SSE. Ensure the CloudFront behavior and ALB idle timeout allow long-lived streaming responses.

## Smoke Test

1. Open the API health endpoint through ALB or CloudFront:
   ```bash
   curl -fsS https://<api-domain>/healthz
   ```
2. Upload a small video through the demo frontend or API:
   ```bash
   curl -fsS -X POST \
     -F "file=@sample.mp4" \
     -F "priority=10" \
     https://<api-domain>/uploads
   ```
3. Poll the returned job ID:
   ```bash
   curl -fsS https://<api-domain>/jobs/<job-id>
   ```
4. Confirm the job reaches `completed` and all output URLs use `CDN_BASE_URL`.
5. Open every output URL and confirm CloudFront returns `200`.
6. Confirm direct S3 reads for `raw/<job-id>/...` fail without AWS credentials.
7. Stop one worker task during processing and confirm the reaper eventually requeues or marks the job dead after retries.
8. Open the dashboard and confirm stats stream updates.

## Hardening Checklist

- Enforce HTTPS with ACM certificates.
- Lock CORS to production frontend domains.
- Restrict `/internal/*` and dashboard access with auth or network controls.
- Send ECS logs to CloudWatch.
- Add metrics for queue depth, jobs by status, dead jobs, worker heartbeat age, throughput/min, and reaper sweep errors.
- Alert on high queue depth, dead jobs above threshold, no active workers, stuck processing jobs, and API 5xx rate.
- Set Redis memory and eviction behavior so queue and lock keys are not evicted under memory pressure.
- Add RDS automated backups and deletion protection for non-demo environments.
- Consider malware scanning before processing public untrusted uploads.
