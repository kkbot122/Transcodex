# Transcodex — Task List

## How to read this

Tasks are grouped by phase. Each phase is shippable on its own — complete phase 1 before starting phase 2. Within each phase, tasks are ordered by dependency. Checkboxes are for tracking progress.

Priority tags:
- `[core]` — must have, project does not work without this
- `[feature]` — adds meaningful capability
- `[infra]` — setup and configuration
- `[polish]` — improves demo quality or code quality

---

## Phase 0 — Project Setup

- [x] `[infra]` Create GitHub repository named `transcodex`
- [x] `[infra]` Set up monorepo directory structure
  ```
  transcodex/
    api/
    worker/
    reaper/
    frontend/
      demo/
      dashboard/
    docker-compose.yml
    README.md
    .env.example
  ```
- [x] `[infra]` Initialise Go modules in `api/`, `worker/`, `reaper/`
- [x] `[infra]` Initialise Vite + React projects in `frontend/demo/` and `frontend/dashboard/`
- [x] `[infra]` Create `.env.example` with all required environment variables
  ```
  DATABASE_URL=
  REDIS_URL=
  STORAGE_ENDPOINT=
  STORAGE_BUCKET=
  STORAGE_ACCESS_KEY=
  STORAGE_SECRET_KEY=
  CDN_BASE_URL=
  WORKER_MAX_RETRIES=3
  WORKER_HEARTBEAT_INTERVAL=10
  REAPER_SWEEP_INTERVAL=30
  ```
- [x] `[infra]` Write base `docker-compose.yml` with all services defined
  - API server
  - Worker (scalable via `--scale`)
  - Reaper
  - PostgreSQL
  - Redis
  - MinIO
- [x] `[infra]` Add `.gitignore` for Go, Node, and env files
- [x] `[infra]` Write skeleton `README.md` with project name and description

---

## Phase 1 — Database and Infrastructure

- [x] `[core]` Write PostgreSQL schema migration file
  - `jobs` table
    - includes stored `priority` so retries preserve queue ordering
  - `job_outputs` table
  - `workers` table
  - Indexes: `jobs(status)`, `jobs(updated_at)`, `workers(status, last_heartbeat)`
- [x] `[core]` Set up database migration tooling (golang-migrate or goose)
- [x] `[core]` Run migrations locally and verify schema
- [x] `[infra]` Configure MinIO in docker-compose
  - Create default bucket on startup
  - Set access key and secret key via environment
- [x] `[infra]` Verify Redis is reachable and sorted set operations work locally
- [x] `[infra]` Write shared Go package for Postgres connection pool (`pgxpool`)
- [x] `[infra]` Write shared Go package for Redis client (`go-redis`)
- [x] `[infra]` Write shared Go package for MinIO / S3 client

---

## Phase 2 — API Server

### Setup
- [x] `[core]` Set up Gin HTTP server in `api/`
- [x] `[core]` Add logger middleware (log method, path, status, latency)
- [x] `[core]` Add recovery middleware (panic → 500, never crash server)
- [x] `[core]` Define all routes
  ```
  POST   /uploads
  GET    /jobs/:id
  GET    /jobs/:id/outputs
  GET    /internal/stats
  GET    /internal/jobs
  GET    /internal/workers
  GET    /internal/stats/stream    (SSE)
  ```

### Upload handler
- [x] `[core]` Parse multipart form, extract file and priority field
- [x] `[core]` Validate file — type must be video, size must be under limit
- [x] `[core]` Stream file directly to MinIO/S3 — do not buffer to disk
- [x] `[core]` Generate `job_id` (UUID v4)
- [x] `[core]` Write Job row to Postgres with `status = queued` and stored `priority`
- [x] `[core]` Compute priority score (priority tier + timestamp tiebreak)
- [x] `[core]` Push QueueMessage to Redis sorted set
- [x] `[core]` Return `202` with `job_id` and `status`

### Job status handler
- [x] `[core]` Fetch Job row by ID from Postgres
- [x] `[core]` Left join JobOutputs on job_id
- [x] `[core]` Return 404 if job not found
- [x] `[core]` Return job fields + outputs array (empty if not completed)

### Job outputs handler
- [x] `[core]` Fetch Job row — return 404 if not found
- [x] `[core]` Return 409 if job status is not completed
- [x] `[core]` Fetch and return all JobOutput rows for the job

### Internal stats handler
- [x] `[feature]` Fetch queue depth from Redis (`ZCARD job_queue`)
- [x] `[feature]` Fetch job state counts from Postgres (`GROUP BY status`)
- [x] `[feature]` Fetch worker state counts from Postgres (`GROUP BY status`)
- [x] `[feature]` Compute throughput — jobs completed in last 1 minute
- [x] `[feature]` Run all three DB/Redis queries in parallel via goroutines
- [x] `[feature]` Return aggregated stats as JSON

### Internal workers handler
- [x] `[feature]` Fetch all worker rows from Postgres
- [x] `[feature]` Return worker list with id, status, current_job, last_heartbeat

### Internal recent jobs handler
- [x] `[feature]` Add `GET /internal/jobs` with optional `status` and `limit` query params
- [x] `[feature]` Fetch recent jobs from Postgres ordered by `updated_at DESC`
- [x] `[feature]` Return job_id, status, retry_count, priority, input_file, created_at, updated_at

### SSE stats stream
- [x] `[feature]` Set SSE headers (`Content-Type: text/event-stream`)
- [x] `[feature]` Tick every 5 seconds, collect stats, write `data: {json}\n\n`
- [x] `[feature]` Flush after each write
- [x] `[feature]` Exit goroutine cleanly on client disconnect (context cancel)

---

## Phase 3 — Worker

### Setup
- [x] `[core]` Set up worker binary in `worker/`
- [x] `[core]` Register worker row in Postgres on startup (status = idle)
- [x] `[core]` Handle SIGTERM gracefully — start bounded drain, finish current job if possible, otherwise cancel context and let reaper recover

### Heartbeat
- [x] `[core]` Start heartbeat goroutine on worker startup
- [x] `[core]` Update `workers.last_heartbeat` every 10 seconds
- [x] `[core]` Stop heartbeat goroutine cleanly on context cancel

### Poll loop
- [x] `[core]` Atomically pop the highest priority FIFO tier from Redis with a Lua script
- [x] `[core]` Sleep 2 seconds on empty queue before retrying
- [x] `[core]` Pass message to job processor on receipt

### Attempt lease
- [x] `[core]` Create a unique attempt and current-attempt reference during conditional claim
- [x] `[core]` Renew the PostgreSQL processing lease while work is running
- [x] `[core]` Fence stale workers with current-attempt conditional transitions

### Job processing
- [x] `[core]` Update job status — `processing` — with `AND status='queued'` guard
- [x] `[core]` Update worker row — `status = busy`, `current_job = job_id`
- [x] `[core]` Download raw file from MinIO/S3 to `/tmp/{job_id}/raw.mp4`
- [x] `[core]` Create output directory `/tmp/{job_id}/`
- [x] `[core]` Defer `os.RemoveAll(/tmp/{job_id}/)` immediately after directory creation

### FFmpeg transcoding
- [x] `[core]` Run FFmpeg 360p transcode subprocess
- [x] `[core]` Run FFmpeg 720p transcode subprocess
- [x] `[core]` Run FFmpeg 1080p transcode subprocess
- [x] `[core]` Run FFmpeg thumbnail extraction subprocess
- [x] `[core]` Run all four subprocesses in parallel via cancellable goroutines
- [x] `[benchmark]` Support sequential mode with identical FFmpeg arguments
- [x] `[core]` Use `exec.CommandContext` so subprocesses respect context cancellation
- [x] `[core]` Capture FFmpeg stderr for error logging
- [x] `[core]` Fail entire job if any single subprocess fails

### Output upload
- [x] `[core]` Upload each output file to MinIO/S3 under an attempt-scoped output key
- [x] `[core]` Run all uploads in parallel via goroutines
- [x] `[core]` Build CDN URL for each output using `CDN_BASE_URL` env var

### Job completion
- [x] `[core]` Write JobOutput rows to Postgres — one per output type (upsert)
- [x] `[core]` Update job status to `completed`
- [x] `[core]` Update worker row — `status = idle`, `current_job = null`

### Failure handling
- [x] `[core]` On any step failure — call `handleFailure(jobID, err)`
- [x] `[core]` In `handleFailure` — check `retry_count` vs `max_retries`
- [x] `[core]` If retries remaining — increment `retry_count`, set `status = queued`, requeue to Redis using stored priority
- [x] `[core]` If retries exhausted — set `status = dead`, log permanent failure

---

## Phase 4 — Reaper

### Setup
- [x] `[core]` Set up reaper binary in `reaper/`
- [x] `[core]` Start tick loop — sweep every 30 seconds
- [x] `[core]` Handle context cancellation cleanly

### Dead worker detection
- [x] `[core]` Query workers where `status = busy AND last_heartbeat < now - 30s`
- [x] `[core]` Mark each dead worker — `status = dead`, `current_job = null`
- [x] `[core]` Call `requeueJob` for each dead worker's current job

### Expired attempt detection
- [x] `[core]` Query running attempts where the processing lease has expired
- [x] `[core]` Recover each still-current attempt conditionally

### Stale queued job detection
- [x] `[core]` Query jobs where `status = queued AND updated_at < now - 30s`
- [x] `[core]` For each candidate, check Redis sorted set for its job_id and re-enqueue missing jobs

### Reaper leadership lock
- [x] `[core]` Acquire Redis lock — `SET reaper_lock {instance_id} NX EX 60`
- [x] `[core]` Skip sweep if another reaper holds the lock

### Requeue logic
- [x] `[core]` In `requeueJob` — fetch full job row
- [x] `[core]` If `retry_count >= max_retries` — mark job `dead`, log, return
- [x] `[core]` Else — increment `retry_count`, set `status = queued`, push to Redis at original priority
- [x] `[core]` Log all requeue events with reason (`worker_death`, `orphan`, or `missing_queue_message`)

---

## Phase 5 — Demo Frontend

- [x] `[feature]` Set up React + Vite project in `frontend/demo/`
- [x] `[feature]` Build `UploadForm` component — file picker + priority selector + upload button
- [x] `[feature]` On upload — call `POST /uploads`, store returned `job_id` in state
- [x] `[feature]` Build `JobStatus` component — poll `GET /jobs/{id}` every 3 seconds
- [x] `[feature]` Display job state with visual indicator (queued / processing / completed / dead)
- [x] `[feature]` Stop polling when status is `completed` or `dead`
- [x] `[feature]` Build `OutputLinks` component — render CDN URLs as clickable links when completed
- [x] `[feature]` Show thumbnail as preview image on completion
- [x] `[feature]` Handle error states — dead job, network error, 404
- [x] `[polish]` Add upload progress indicator
- [x] `[polish]` Show elapsed time since job was created

---

## Phase 6 — Observability Dashboard

- [x] `[feature]` Set up React + Vite project in `frontend/dashboard/`
- [x] `[feature]` Write `useSSE(url)` custom hook — connect to SSE stream, return live data
- [x] `[feature]` Build `QueueStats` component — queue depth + throughput per minute
- [x] `[feature]` Build `WorkerGrid` component — worker cards showing id, status, current job
- [x] `[feature]` Build `JobCounts` component — counts per status with visual breakdown
- [x] `[feature]` Build `JobTable` component — filterable list of recent jobs by status
- [x] `[feature]` Wire aggregate components to `useSSE` hook
- [x] `[feature]` Wire `JobTable` to `GET /internal/jobs`
- [x] `[feature]` Show connection status indicator — connected / reconnecting
- [x] `[polish]` Highlight dead workers in red
- [x] `[polish]` Highlight dead jobs in red
- [x] `[polish]` Auto-reconnect indicator when SSE drops

---

## Phase 7 — Docker Compose Integration

- [x] `[infra]` Write `Dockerfile` for API server (multi-stage Go build)
- [x] `[infra]` Write `Dockerfile` for Worker (multi-stage Go build, include FFmpeg)
- [x] `[infra]` Write `Dockerfile` for Reaper (multi-stage Go build)
- [x] `[infra]` Write `Dockerfile` for demo frontend (Vite build + Nginx serve)
- [x] `[infra]` Write `Dockerfile` for dashboard frontend (Vite build + Nginx serve)
- [x] `[infra]` Complete `docker-compose.yml`
  - All services with correct env vars
  - Postgres with healthcheck
  - Redis with healthcheck
  - MinIO with bucket init script
  - Worker with `depends_on` API, Redis, Postgres
  - Worker scalable via `docker compose up --scale worker=3`
- [x] `[infra]` Verify `docker compose up` brings full system online
- [x] `[infra]` Verify end to end — upload via demo frontend, watch job complete, view outputs

---

## Phase 8 — AWS Deployment

### Pre-deployment
- [x] `[infra]` Add Terraform scaffold for the AWS deployment
- [ ] `[infra]` Choose primary region and DNS name for the demo deployment
- [ ] `[infra]` Install and configure AWS CLI with a least-privilege deploy identity
- [ ] `[infra]` Create IAM roles for ECS task execution and app task access
  - Execution role: ECR image pulls, CloudWatch logs, Secrets Manager / SSM reads
  - App task role: S3 raw/output read-write access only to the Transcodex bucket prefixes
- [ ] `[infra]` Move production config to Secrets Manager or SSM Parameter Store
  - `DATABASE_URL`
  - `REDIS_URL`
  - `STORAGE_ENDPOINT`
  - `STORAGE_BUCKET`
  - `CDN_BASE_URL`
  - `CORS_ALLOWED_ORIGINS`

### Networking
- [ ] `[infra]` Create VPC across at least two Availability Zones
- [ ] `[infra]` Create public subnets for ALB and NAT Gateway
- [ ] `[infra]` Create private app subnets for ECS API, worker, and reaper tasks
- [ ] `[infra]` Create private data subnets for RDS and ElastiCache
- [ ] `[infra]` Attach Internet Gateway and public route table
- [ ] `[infra]` Add NAT Gateway only if private tasks need outbound internet
- [ ] `[infra]` Add S3 Gateway VPC endpoint if workers should reach S3 without NAT
- [ ] `[infra]` Create narrow security groups
  - ALB -> API service on app port
  - API / worker / reaper -> RDS Postgres
  - API / worker / reaper -> ElastiCache Redis
  - API / worker -> S3 through IAM and optional VPC endpoint

### Storage, CDN, and frontends
- [ ] `[infra]` Create S3 bucket with block public access enabled
- [ ] `[infra]` Store raw uploads under `raw/` and keep them private
- [ ] `[infra]` Store processed outputs under `outputs/`
- [ ] `[infra]` Create CloudFront Origin Access Control for the S3 origin
- [ ] `[infra]` Set S3 bucket policy to allow CloudFront OAC reads for `outputs/*`
- [ ] `[infra]` Avoid public bucket ACLs and public bucket policies
- [ ] `[infra]` Create CloudFront distribution
  - `/outputs/*` -> S3 origin with long cache TTL
  - `/uploads` -> ALB/API origin with caching disabled
  - `/jobs/*` -> ALB/API origin with caching disabled
  - `/internal/*` -> ALB/API origin with caching disabled and dashboard access restricted
- [ ] `[infra]` Build demo frontend with production API URL
- [ ] `[infra]` Build dashboard frontend with production API/SSE URL
- [ ] `[infra]` Upload frontend builds to S3 static prefixes or deploy frontend Nginx containers
- [ ] `[infra]` Serve frontends through CloudFront

### Database and Redis
- [ ] `[infra]` Create RDS PostgreSQL in private data subnets
- [ ] `[infra]` Enable automated RDS backups
- [ ] `[infra]` Restrict RDS inbound access to app security groups only
- [ ] `[infra]` Run migrations as a one-off deployment job before app services start
- [ ] `[infra]` Create ElastiCache Redis in private data subnets
- [ ] `[infra]` Restrict Redis inbound access to API, worker, and reaper security groups only
- [ ] `[infra]` Set Redis memory/eviction behavior so queue and lock keys are not unexpectedly evicted

### Images and compute
- [ ] `[infra]` Create ECR repositories for API, worker, reaper, demo frontend, and dashboard
- [ ] `[infra]` Build and push Docker images to ECR
- [ ] `[infra]` Create ECS cluster
- [ ] `[infra]` Create API ECS service behind ALB with `/healthz` health check
- [ ] `[infra]` Create worker ECS service scaled independently from API
- [ ] `[infra]` Give workers more CPU/memory than API because FFmpeg is the bottleneck
- [ ] `[infra]` Create one small reaper ECS service, or multiple replicas relying on Redis leadership lock
- [ ] `[infra]` Configure container logs to CloudWatch
- [ ] `[infra]` Configure task CPU/memory limits and restart behavior
- [ ] `[infra]` Add worker autoscaling based on queue depth or CPU

### Verification
- [ ] `[infra]` Verify API health through ALB and CloudFront API routes
- [ ] `[infra]` End to end production smoke test — upload, process, verify CDN URLs load
- [ ] `[infra]` Verify observability dashboard connects and shows live data
- [ ] `[infra]` Verify reaper recovers a manually killed worker
- [ ] `[infra]` Verify raw uploads are not publicly readable
- [ ] `[infra]` Verify only output URLs are readable through CloudFront

### Production hardening
- [ ] `[infra]` Enforce HTTPS with ACM certificates
- [ ] `[infra]` Lock CORS to production frontend domains
- [ ] `[infra]` Restrict or authenticate `/internal/*` and dashboard access
- [ ] `[infra]` Add CloudWatch metrics for queue depth, jobs by status, dead jobs, worker heartbeat age, throughput/min, and reaper sweep errors
- [ ] `[infra]` Add alerts for high queue depth, dead jobs above threshold, no active workers, stuck processing jobs, and API 5xx rate
- [ ] `[infra]` Consider malware scanning if accepting public untrusted uploads
- [ ] `[infra]` Consider DLQ-style reporting for permanently dead jobs

---

## Phase 9 — Polish and Resume Prep

### Code quality
- [x] `[polish]` Add structured logging throughout (slog or zap)
- [x] `[polish]` Ensure all errors are logged with context (job_id, worker_id)
- [x] `[polish]` Remove all hardcoded values — everything via environment variables
- [x] `[polish]` Write Go unit tests for priority scoring logic
- [x] `[polish]` Write Go unit tests for retry / failure handler
- [x] `[polish]` Write Go unit tests for reaper requeue logic

### Documentation
- [ ] `[polish]` Write full `README.md`
  - Project description
  - Architecture diagram (embed HLD image)
  - Local setup instructions (`docker compose up`)
  - API reference
  - Environment variable reference
  - Deployment notes
- [ ] `[polish]` Add architecture diagram image to repo
- [ ] `[polish]` Add demo GIF or screen recording to README

### Resume and interview prep
- [ ] `[polish]` Write one-line resume description for the project
- [ ] `[polish]` Write two-paragraph project summary for portfolio or LinkedIn
- [ ] `[polish]` Record a 2-3 minute demo walkthrough video
- [ ] `[polish]` Review all 10 interview tradeoffs from design document
- [ ] `[polish]` Practice explaining the retry flow out loud without notes
- [ ] `[polish]` Practice explaining the processing flow out loud without notes

---

## Task Count Summary

| Phase | Tasks | Priority |
|---|---|---|
| 0 — Project setup | 8 | infra |
| 1 — Database and infra | 9 | core + infra |
| 2 — API server | 27 | core + feature |
| 3 — Worker | 26 | core |
| 4 — Reaper | 14 | core |
| 5 — Demo frontend | 11 | feature + polish |
| 6 — Observability dashboard | 12 | feature + polish |
| 7 — Docker Compose | 9 | infra |
| 8 — AWS deployment | 24 | infra |
| 9 — Polish and resume prep | 14 | polish |
| **Total** | **154** | |
