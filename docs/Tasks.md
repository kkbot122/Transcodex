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
- [x] `[core]` Poll Redis sorted set with `ZPOPMAX job_queue`
- [x] `[core]` Sleep 2 seconds on empty queue before retrying
- [x] `[core]` Pass message to job processor on receipt

### Lock
- [x] `[core]` Acquire Redis lock — `SET lock:job:{id} {worker_id} NX EX 300`
- [x] `[core]` Skip job silently if lock not acquired
- [x] `[core]` Release lock on job completion or failure (`DEL lock:job:{id}`)

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
- [x] `[core]` Run all four subprocesses in parallel via `sync.WaitGroup` + goroutines
- [x] `[core]` Use `exec.CommandContext` so subprocesses respect context cancellation
- [x] `[core]` Capture FFmpeg stderr for error logging
- [x] `[core]` Fail entire job if any single subprocess fails

### Output upload
- [x] `[core]` Upload each output file to MinIO/S3 under `outputs/{job_id}/{type}`
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
- [x] `[core]` Set up reaper binary in `reaper/` (or sidecar goroutine in API server)
- [x] `[core]` Start tick loop — sweep every 30 seconds
- [x] `[core]` Handle context cancellation cleanly

### Dead worker detection
- [x] `[core]` Query workers where `status = busy AND last_heartbeat < now - 30s`
- [x] `[core]` Mark each dead worker — `status = dead`, `current_job = null`
- [x] `[core]` Call `requeueJob` for each dead worker's current job

### Orphaned job detection
- [x] `[core]` Query jobs where `status = processing AND updated_at < now - 5min`
- [x] `[core]` Call `requeueJob` for each orphaned job

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
- [ ] `[infra]` Create AWS account and set up IAM user with least-privilege policy
- [ ] `[infra]` Install and configure AWS CLI

### Networking
- [ ] `[infra]` Create VPC with CIDR block
- [ ] `[infra]` Create public subnet and private subnet
- [ ] `[infra]` Create Internet Gateway, attach to VPC
- [ ] `[infra]` Create NAT Gateway in public subnet
- [ ] `[infra]` Configure route tables — public subnet routes to IGW, private subnet routes to NAT

### Storage and database
- [ ] `[infra]` Create S3 bucket — block all public access
- [ ] `[infra]` Set S3 bucket policy — allow CloudFront OAC on `outputs/*` prefix only
- [ ] `[infra]` Launch RDS PostgreSQL (`db.t3.micro`) in private subnet
- [ ] `[infra]` Run migrations against RDS instance
- [ ] `[infra]` Launch EC2 instance for Redis in private subnet (until ElastiCache budget allows)

### Compute
- [ ] `[infra]` Launch EC2 instance for API server in private subnet
- [ ] `[infra]` Install Docker on EC2 instances
- [ ] `[infra]` Deploy API server container with production env vars
- [ ] `[infra]` Launch EC2 instance(s) for workers in private subnet
- [ ] `[infra]` Deploy worker container(s) with production env vars
- [ ] `[infra]` Verify workers connect to RDS, Redis, S3 successfully

### Load balancer and CDN
- [ ] `[infra]` Create Application Load Balancer in public subnet
- [ ] `[infra]` Configure ALB target group pointing to API server EC2
- [ ] `[infra]` Create CloudFront distribution
  - Origin 1: S3 bucket with OAC (`/outputs/*`)
  - Origin 2: ALB (`/uploads`, `/jobs/*`, `/internal/*`)
- [ ] `[infra]` Set cache policy — long TTL on `/outputs/*`, no cache on API paths
- [ ] `[infra]` Update `CDN_BASE_URL` env var on workers to CloudFront domain

### Frontend deployment
- [ ] `[infra]` Build demo frontend with production API URL
- [ ] `[infra]` Build dashboard frontend with production SSE URL
- [ ] `[infra]` Upload both builds to S3 static hosting prefixes
- [ ] `[infra]` Configure CloudFront to serve frontends from S3

### Verification
- [ ] `[infra]` End to end test on production — upload, process, verify CDN URLs load
- [ ] `[infra]` Verify observability dashboard connects and shows live data
- [ ] `[infra]` Verify reaper recovers a manually killed worker

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
