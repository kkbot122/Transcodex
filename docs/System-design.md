# Transcodex — System Design Document

## Table of Contents

1. [Project Overview](#1-project-overview)
2. [Domains Covered](#2-domains-covered)
3. [Functional Requirements](#3-functional-requirements)
4. [Non Functional Requirements](#4-non-functional-requirements)
5. [Core Entities](#5-core-entities)
6. [API Design](#6-api-design)
7. [High Level Design](#7-high-level-design)
8. [Deep Dives](#8-deep-dives)
   - [Retry Flow](#81-retry-flow)
   - [Processing Flow](#82-processing-flow)
   - [CDN Delivery](#83-cdn-delivery)
9. [Low Level Design](#9-low-level-design)
   - [API Server](#91-api-server)
   - [Worker](#92-worker)
   - [Reaper](#93-reaper)
   - [Observability Dashboard](#94-observability-dashboard)
10. [Deployment Architecture](#10-deployment-architecture)
11. [Stack](#11-stack)
12. [Interview Tradeoffs](#12-interview-tradeoffs)

---

## 1. Project Overview

Transcodex is a distributed video processing pipeline that accepts raw video uploads, transcodes them into multiple resolutions, generates thumbnails, and delivers outputs via CDN. Designed as a backend infrastructure service — any application can integrate with it via upload API and poll for job completion.

Real world analogy: a self-hosted, simplified AWS MediaConvert or Cloudinary video pipeline.

**Who uses it:**

| Surface | Used by | Purpose |
|---|---|---|
| `POST /uploads`, `GET /jobs/{id}` | Other applications / developers | Integrate pipeline into their product |
| Demo frontend (React) | Interviewers / demos | Demonstrate end to end flow |
| Observability dashboard (React) | Operator | Monitor system health |

---

## 2. Domains Covered

- **Distributed systems** — worker coordination, fault tolerance, at-least-once job delivery
- **System design** — queue-based decoupling, pipeline architecture, tradeoff reasoning
- **Backend engineering** — API design, file handling, worker processes
- **Async / concurrent programming** — goroutines, job lifecycle management
- **Observability** — metrics collection, job state machines, live dashboard
- **DevOps / infra** — Docker, Docker Compose, multi-service orchestration
- **Database design** — job metadata schema, state transitions, indexing
- **Cloud deployment** — AWS (EC2, RDS, ElastiCache, S3, CloudFront)

---

## 3. Functional Requirements

1. **Video upload** — client uploads a raw video file via multipart API, streamed directly to object storage
2. **Job creation** — every upload creates a processing job with a unique job ID returned immediately
3. **Transcoding** — worker transcodes video into 360p, 720p, 1080p
4. **Thumbnail generation** — extract a thumbnail at a fixed timestamp
5. **Job status tracking** — caller polls `/jobs/{id}` to check state and retrieve output URLs on completion
6. **CDN delivery** — processed files and thumbnails served via CDN
7. **Retry on failure** — processing errors are retried up to N times before the job is marked dead
8. **Observability dashboard** — live view of queue depth, job states, worker utilization

**Explicit out of scope:**
- User authentication / multi-tenancy
- Webhook notifications
- Video streaming or playback
- Billing or rate limiting

---

## 4. Non Functional Requirements

| Property | Requirement |
|---|---|
| Reliability | A job must not be lost even if a worker crashes mid-processing |
| Fault tolerance | Worker failure must not affect other jobs in the queue |
| Scalability | Workers must be horizontally scalable |
| Idempotency | Retrying a job must not produce duplicate outputs |
| Durability | Processed files and job metadata must persist beyond worker lifecycle |
| Observability | System internals must be inspectable without touching the DB directly |
| Low coupling | Upload API and workers must be independently deployable |

---

## 5. Core Entities

### Job
Central entity. Everything revolves around it.

```
Job {
  id            uuid        PK
  status        enum        queued | processing | completed | dead
  retry_count   int         default 0
  max_retries   int         default 3
  priority      int         default 0
  input_file    string      path/key in object storage
  created_at    timestamp
  updated_at    timestamp
}
```

### JobOutput
Separate from Job — one job produces multiple outputs.

```
JobOutput {
  id            uuid        PK
  job_id        uuid        FK → Job
  type          enum        video_360p | video_720p | video_1080p | thumbnail
  cdn_url       string      public CDN URL for this output
  file_size     int         bytes
  created_at    timestamp
}
```

### Worker
Represents a running worker process. Used by the observability layer.

```
Worker {
  id            uuid        PK
  status        enum        idle | busy | dead
  current_job   uuid        FK → Job (nullable)
  last_heartbeat timestamp
}
```

### QueueMessage
Not a DB table — lives in Redis. Payload pushed when a job is created.

```
QueueMessage {
  job_id        uuid
  input_file    string
  priority      int
  enqueued_at   timestamp
}
```

### Relationships

```
Job ──< JobOutput     one job → many outputs
Job ──< Worker        one worker handles one job at a time
Job ──> QueueMessage  one job → one message in Redis (temporary)
```

---

## 6. API Design

### POST /uploads
```
Content-Type: multipart/form-data
Body: { file: binary, priority: int (optional, default 0) }

Response 202: { job_id: uuid, status: "queued" }
```

Returns immediately. `202 Accepted` signals async work — processing has not happened yet.

### GET /jobs/{job_id}
```
Response 200: {
  job_id, status, retry_count, priority, created_at, updated_at,
  outputs: JobOutput[]   // empty until completed
}
Response 404: { error: "job not found" }
```

### GET /jobs/{job_id}/outputs
```
Response 200: {
  outputs: [
    { type: "video_360p", cdn_url: "...", file_size: int },
    { type: "video_720p", cdn_url: "...", file_size: int },
    { type: "video_1080p", cdn_url: "...", file_size: int },
    { type: "thumbnail",  cdn_url: "...", file_size: int }
  ]
}
Response 409: { error: "job not yet completed" }
```

### GET /internal/stats
```
Response 200: {
  queue_depth: int,
  throughput_per_min: int,
  workers: { total, idle, busy, dead },
  jobs: { queued, processing, completed, dead }
}
```

### GET /internal/jobs
```
Query params: status optional, limit optional default 50

Response 200: {
  jobs: [{
    job_id, status, retry_count, priority, input_file, created_at, updated_at
  }]
}
```

### GET /internal/workers
```
Response 200: {
  workers: [{ id, status, current_job, last_heartbeat }]
}
```

**Design decisions:**
- `202` on upload — signals async, not a mistake
- `409` on outputs when job not done — explicit over silent empty array
- Internal endpoints are separate — private network or basic auth in production
- Full historical pagination is out of scope; `/internal/jobs` is a bounded recent list for the dashboard

---

## 7. High Level Design

```
Client
  │
  ▼
API Server (Go)
  ├── writes Job → PostgreSQL
  ├── pushes QueueMessage → Redis (priority sorted set)
  └── raw file → Object Storage (MinIO / S3)
          │
          ▼
     Redis Queue
          │
          ▼
    Worker Pool (Go × N)
          ├── FFmpeg transcode (360p, 720p, 1080p) in parallel
          ├── thumbnail extraction
          ├── outputs → Object Storage
          ├── JobOutput rows → PostgreSQL
          └── job status update → PostgreSQL
                    │
                    ▼
             Object Storage
                    │
                    ▼
                  CDN
                    │
                    ▼
                 Client
```

**Observability** reads queue depth from Redis and job/worker state from PostgreSQL. Dashboard served via SSE — server pushes updates every 5 seconds.

**Reaper** runs as a sidecar on the API server, sweeps every 30 seconds, recovers dead workers and orphaned jobs.

---

## 8. Deep Dives

### 8.1 Retry Flow

Two mechanisms work together:

**Worker heartbeats** — every worker updates `last_heartbeat` in Postgres every 10 seconds. Stops on crash.

**Reaper sweep (every 30s):**
1. Find workers where `last_heartbeat < now - 30s` → mark dead, requeue their current job
2. Find jobs where `status = 'processing' AND updated_at < now - 5min` → requeue (orphan recovery)
3. Find stale queued jobs missing from Redis → re-enqueue (Postgres/Redis consistency repair)

**Job state machine:**
```
queued
  ↓ worker picks up
processing
  ↓ success          ↓ failure / crash
completed         retry_count++
                  ↓ retry_count < max_retries
                queued  (requeued)
                  ↓ retry_count >= max_retries
                dead
```

**Idempotency on retry:**
- Worker checks if output files already exist before re-uploading
- JobOutput writes use upsert (`ON CONFLICT DO UPDATE`) — no duplicate rows

**At-least-once delivery:** Redis can redeliver a message if worker crashes before acknowledging. Fix: Redis lock (`SET lock:job:{id} NX EX 300`) — only one worker processes a job at a time. Lock expires if worker dies.

### 8.2 Processing Flow

Inside a single worker, per job:

1. Poll Redis — `ZPOPMAX job_queue` (highest priority score first)
2. Acquire lock — `SET lock:job:{id} {worker_id} NX EX 300`
3. Mark processing — `UPDATE jobs SET status='processing' WHERE id=$1 AND status='queued'`
4. Download raw file from object storage → `/tmp/{job_id}/raw.mp4`
5. FFmpeg transcode — 3 resolutions + thumbnail in parallel via goroutines
6. Upload outputs to object storage — parallel
7. Write JobOutput rows — upsert
8. Mark completed — `UPDATE jobs SET status='completed'`
9. Cleanup — `os.RemoveAll(/tmp/{job_id}/)`, release Redis lock

**Failure at any step** → `handleFailure()` → increment retry, requeue or mark dead.

**Key:** `defer os.RemoveAll` ensures temp cleanup runs whether job succeeds or fails.

### 8.3 CDN Delivery

CloudFront sits in front of S3. Workers write to S3, CloudFront serves from edge.

**Worker stores CloudFront URL, not S3 URL:**
```go
cdnURL := fmt.Sprintf("%s/outputs/%s/%s", os.Getenv("CDN_BASE_URL"), jobID, outputType)
```

**CloudFront distribution — two origins:**
- `/outputs/*` → S3 bucket (long cache TTL, videos are immutable)
- `/uploads`, `/jobs/*`, `/internal/*` → ALB (cache disabled, dynamic)

**S3 bucket stays fully private.** Only CloudFront accesses it via Origin Access Control (OAC). Raw uploads (`uploads/` prefix) never exposed via CloudFront.

**Cache headers on outputs:**
```
Cache-Control: public, max-age=86400, immutable
```

**Local dev:** `CDN_BASE_URL=http://localhost:9000/bucket` — MinIO serves directly. Code unchanged between environments.

---

## 9. Low Level Design

### 9.1 API Server

**Structure:**
```
api/
  main.go
  server.go
  handlers/
    upload.go
    jobs.go
    internal.go
  services/
    job_service.go
    queue_service.go
    storage_service.go
  db/
    postgres.go
    queries.go
  middleware/
    logger.go
    recovery.go
```

**Upload handler flow:**
1. Parse multipart form, validate file type and size
2. Stream file directly to object storage (never buffer to disk)
3. Create Job row in Postgres
4. Push QueueMessage to Redis
5. Return 202 with job_id

Postgres write before Redis push — crash safety. If Redis push fails, reaper detects queued job with no message and re-enqueues.

**Priority scoring:**
```go
func PriorityScore(priority int, enqueuedAt time.Time) float64 {
    return float64(priority)*1_000_000_000_000_000 - float64(enqueuedAt.UnixMilli())
}
```

Higher priority wins. Within same priority, older jobs win (FIFO). The function lives in `pkg/queue` so API enqueue, worker requeue, and reaper repair all use the same formula.

**Redis queue indexes:**
- `job_queue` sorted set stores serialized queue messages scored by priority
- `queued_jobs` set stores job IDs currently expected in the queue

Workers remove a job ID from `queued_jobs` after `ZPOPMAX`. The reaper checks `SISMEMBER queued_jobs {job_id}` instead of scanning the sorted set.

**Connection pools:**
- Postgres: `pgxpool`, max 20 connections
- Redis: `go-redis`, pool size 10

**Middleware stack:** `request → logger → recovery → handler`

Recovery catches panics, returns 500, never crashes the server.

### 9.2 Worker

**Structure:**
```
worker/
  main.go
  worker.go
  processor/
    transcode.go
    thumbnail.go
    upload.go
  services/
    queue_service.go
    job_service.go
    storage_service.go
  heartbeat/
    heartbeat.go
  db/
    postgres.go
```

**Main poll loop:**
```go
func (w *Worker) Run(ctx context.Context) {
    w.heartbeat.Start(ctx)
    for {
        select {
        case <-ctx.Done():
            return
        default:
            msg, err := w.queue.Poll(ctx)
            if err != nil || msg == nil {
                time.Sleep(2 * time.Second)
                continue
            }
            w.processJob(ctx, msg)
        }
    }
}
```

2 second backoff on empty queue — avoids hammering Redis.

**Parallel FFmpeg:**
```go
var wg sync.WaitGroup
results := make([]Output, 4)
errs    := make([]error, 4)

for i, job := range transcodeJobs {
    wg.Add(1)
    go func(i int, job TranscodeJob) {
        defer wg.Done()
        results[i], errs[i] = p.runFFmpeg(ctx, inputPath, jobID, job)
    }(i, job)
}
wg.Wait()
```

All 4 subprocesses (360p, 720p, 1080p, thumbnail) run simultaneously. Any failure fails the whole job.

`exec.CommandContext` — FFmpeg subprocesses respect context cancellation on shutdown.

**Heartbeat goroutine** — independent of job loop, ticks every 10 seconds, never blocked by a long transcode.

**Graceful shutdown** — SIGTERM starts a bounded drain period. If the current job finishes within the grace window, the worker exits cleanly. If not, the context is cancelled, FFmpeg subprocesses are killed, the job stays in `processing`, and the reaper requeues it.

### 9.3 Reaper

**Structure:**
```
reaper/
  main.go
  reaper.go
  detectors/
    worker_detector.go
    job_detector.go
  services/
    job_service.go
    worker_service.go
    queue_service.go
```

**Tick loop:** every 30 seconds. Two passes — dead workers first, orphaned jobs second. Order matters.

**Pass 1 — dead workers:**
```sql
SELECT id, current_job FROM workers
WHERE status = 'busy'
AND last_heartbeat < now() - interval '30 seconds'
```
Mark dead, requeue their current job.

**Pass 2 — orphaned jobs:**
```sql
SELECT * FROM jobs
WHERE status = 'processing'
AND updated_at < now() - interval '5 minutes'
```
5 minute threshold — safety net only, not primary recovery.

**Pass 3 — stale queued jobs:**
```sql
SELECT * FROM jobs
WHERE status = 'queued'
AND updated_at < now() - interval '30 seconds'
```
For each candidate, check whether `job_id` exists in the Redis `queued_jobs` set. If missing, push a new QueueMessage using the job's stored `priority` and add the ID back to `queued_jobs`.

**Requeue logic:**
- If `retry_count >= max_retries` → mark `dead`
- Else → increment `retry_count`, set `status = 'queued'`, push to Redis at original priority

**Race condition defence:**
- Status guard: `UPDATE jobs SET status='processing' WHERE id=$1 AND status='queued'` — 0 rows affected = abort
- Redis lock: worker holds lock, second worker skips job entirely

**At scale:** single reaper instance. Use Redis distributed lock (`SET reaper_lock NX EX 60`) to prevent multiple reapers racing.

### 9.4 Observability Dashboard

**Backend — parallel stat collection:**
```go
// Three goroutines run simultaneously:
// 1. Redis ZCARD → queue depth
// 2. Postgres GROUP BY status → job counts
// 3. Postgres GROUP BY status → worker counts
```

**Throughput:**
```sql
SELECT count(*) FROM jobs
WHERE status = 'completed'
AND updated_at > now() - interval '1 minute'
```

**SSE endpoint** — server pushes stats every 5 seconds:
```go
c.Header("Content-Type", "text/event-stream")
// tick every 5s, marshal stats, fmt.Fprintf(c.Writer, "data: %s\n\n", data)
```

Client disconnect → context cancels → goroutine exits. No leak.

**Recent jobs endpoint** backs the dashboard table:
```sql
SELECT * FROM jobs
WHERE ($1::job_status IS NULL OR status = $1)
ORDER BY updated_at DESC
LIMIT $2
```

**Frontend — React (Vite):**

Two apps:

```
frontend/
  demo/               upload form + job status poller
    UploadForm.jsx
    JobStatus.jsx
    OutputLinks.jsx
  dashboard/          live observability
    QueueStats.jsx
    WorkerGrid.jsx
    JobCounts.jsx
    JobTable.jsx
    hooks/useSSE.js
```

`useSSE` hook:
```javascript
function useSSE(url) {
    const [data, setData] = useState(null);
    useEffect(() => {
        const es = new EventSource(url);
        es.onmessage = (e) => setData(JSON.parse(e.data));
        return () => es.close();
    }, [url]);
    return data;
}
```

SSE auto-reconnects natively — no reconnection logic needed.

---

## 10. Deployment Architecture

### AWS Service Mapping

| Local | AWS |
|---|---|
| API server container | EC2 / ECS |
| Worker containers | EC2 / ECS × N |
| Redis | ElastiCache |
| PostgreSQL | RDS |
| MinIO | S3 |
| CDN | CloudFront |
| React frontends | S3 static + CloudFront |

### Network Layout

```
Internet
    │
CloudFront (CDN + routing)
    ├── /outputs/*     → S3 (video files, long cache)
    ├── /uploads       → ALB → API server
    ├── /jobs/*        → ALB → API server
    └── /internal/*    → ALB → API server

VPC
  ├── Public subnet
  │     ├── ALB
  │     └── NAT Gateway (outbound only for private subnet)
  └── Private subnet
        ├── EC2 — API server + reaper (sidecar)
        ├── EC2 × N — Workers
        ├── ElastiCache — Redis
        └── RDS — PostgreSQL

S3 — outside VPC, accessed via VPC endpoint
```

### Security

- Nothing in private subnet is publicly reachable
- Workers never expose a port
- Only ALB faces the internet
- S3 bucket fully private — CloudFront accesses via OAC only
- Raw uploads (`uploads/` prefix) never exposed via CloudFront

### Scaling

- Workers are stateless — add EC2 instances or ECS tasks, point at same Redis and Postgres
- API server — multiple instances behind ALB, session state in Redis not memory
- Database — read replicas for observability queries, PgBouncer for connection pooling at high concurrency

### Free Tier

| Service | Free tier |
|---|---|
| EC2 t2.micro | 750 hrs/month |
| RDS t3.micro | 750 hrs/month |
| S3 | 5GB storage |
| CloudFront | 1TB transfer |
| ElastiCache | Not free — run Redis on EC2 during dev |

### Deployment Order

1. VPC with public + private subnets, NAT Gateway
2. RDS Postgres in private subnet
3. Redis on EC2 (swap to ElastiCache before final demo)
4. S3 bucket — block public access on `uploads/`
5. EC2 — API server + reaper as Docker containers
6. EC2 × N — workers as Docker containers
7. ALB pointing to API server
8. CloudFront — two origins (ALB + S3)
9. S3 static hosting for React frontends via CloudFront

### Monorepo Structure

```
transcodex/
  api/              Go — API server
  worker/           Go — worker binary
  reaper/           Go — reaper binary
  frontend/
    demo/           React — upload demo
    dashboard/      React — observability
  docker-compose.yml
  README.md
```

Single `docker-compose up` spins everything locally.

---

## 11. Stack

| Layer | Technology | Reason |
|---|---|---|
| API + Workers + Reaper | Go | Concurrency model, goroutines, you know it |
| Queue | Redis sorted set | Priority ordering, atomic pop, fast |
| Database | PostgreSQL | Job metadata, state tracking |
| Transcoding | FFmpeg | Industry standard |
| Object storage (local) | MinIO | S3-compatible, no AWS account needed |
| Object storage (cloud) | S3 | Direct swap from MinIO |
| CDN | CloudFront | Native S3 integration |
| Frontend | React (Vite) | Familiar, no SSR needed |
| Infra | Docker Compose (local), AWS (cloud) | Clean multi-service setup |

---

## 12. Interview Tradeoffs

**Why a queue instead of direct worker invocation?**
Decouples arrival rate from processing rate. Jobs accumulate when workers are busy, drain when free. Workers can crash without losing jobs. Scale independently. Fundamental argument for async job queues.

**Why Redis and not Postgres as a queue?**
Postgres polling with `FOR UPDATE SKIP LOCKED` is legitimate but adds lock contention and polling overhead. Redis sorted sets give priority ordering, atomic pop, and sub-millisecond latency out of the box. Postgres as a queue works — Redis is purpose-fit. If Redis goes down, jobs in Postgres as `queued` are recoverable by the reaper.

**Why not SQS?**
SQS is the right production answer — managed, built-in retry, dead letter queues. Built Redis-based queue here for learning — you understand what SQS does under the hood because you built the equivalent.

**How does the system handle duplicate processing?**
Three layers: Redis lock (`SET NX`) prevents two workers acquiring the same job. Status guard (`AND status='queued'`) prevents double Postgres transition. Upsert on JobOutput prevents duplicate rows. Can't guarantee exactly-once across Redis and Postgres without a distributed transaction — what you get is effectively-once.

**What breaks first under load?**
Workers — FFmpeg is CPU and memory intensive. At scale: one EC2 instance per worker, larger instance types, separate queues for heavy vs light jobs. Second bottleneck is Postgres — read replicas for observability queries, PgBouncer for connection pooling.

**Why not store videos in the DB?**
Databases are optimised for structured, queryable data — not binary blobs. Object storage is cheap, durable, scalable, and enables CDN delivery. Postgres stores metadata only — paths, sizes, URLs.

**How would you scale to 10x traffic?**
Workers — stateless, add instances, Auto Scaling Group triggered by Redis queue depth via CloudWatch custom metric. API server — multiple instances behind ALB. Database — read replicas, PgBouncer.

**Biggest reliability risk?**
The reaper. If it goes down, orphaned jobs accumulate. Mitigation: sidecar on API server (shared lifecycle). At scale: distributed lock so only one reaper runs. Production: alert on jobs stuck in `processing` beyond threshold.

**Why FFmpeg and not MediaConvert?**
MediaConvert is the right production answer. FFmpeg here is intentional — the point is to understand the pipeline, not glue managed services. Swapping FFmpeg for MediaConvert would change only the worker's processor layer.

**How do you know the system is healthy?**
Queue depth shows if jobs back up faster than workers process. Worker heartbeat age shows silent deaths. Dead job count shows classes of input consistently failing. Throughput per minute shows if processing rate dropped. Without observability the answer is "I don't" — with it you have a live dashboard.

**Decision framework for interviews:**
For every decision — queue choice, retry mechanism, CDN setup, deployment topology — be able to say: *"I chose X, the alternative was Y, I chose X because Z, the tradeoff is W."*
