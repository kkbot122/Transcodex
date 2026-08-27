# transcodex

A distributed video processing pipeline. Upload a raw video, get back multiple resolutions and a thumbnail — delivered via CDN.

Built as a backend infrastructure service. Any application can integrate via a simple upload API and poll for job completion.

---

## What it does

- Accepts raw video uploads via REST API
- Transcodes to 360p, 720p, and 1080p in parallel
- Extracts a thumbnail
- Tracks job state through a full lifecycle — queued → processing → completed or dead
- Retries processing errors automatically, recovers from worker crashes
- Serves processed files via CDN
- Exposes a live observability dashboard

---

## Architecture

```
Client
  │
  ▼
API Server (Go)
  ├── writes job metadata → PostgreSQL
  ├── pushes job → Redis (priority queue)
  └── stores raw file → S3 / MinIO
              │
              ▼
        Worker Pool (Go × N)
              ├── FFmpeg transcode — parallel
              ├── outputs → S3 / MinIO
              └── updates job status → PostgreSQL
                          │
                          ▼
                    CloudFront CDN
                          │
                          ▼
                       Client
```

A reaper process sweeps every 30 seconds to detect dead workers, orphaned jobs, and queued jobs missing from Redis, requeuing them automatically. Job status is one of `queued`, `processing`, `completed`, or `dead`.

---

## Stack

| Layer | Technology |
|---|---|
| API + Workers + Reaper | Go |
| Queue | Redis sorted set for priority ordering plus `queued_jobs` set for O(1) membership checks |
| Database | PostgreSQL |
| Transcoding | FFmpeg |
| Object storage | MinIO (local) / S3 (cloud) |
| CDN | CloudFront |
| Frontend | React (Vite) |
| Infra | Docker Compose / AWS |

---

## Local setup

**Prerequisites:** Docker, Docker Compose

```bash
git clone https://github.com/your-username/transcodex
cd transcodex
cp .env.example .env
docker compose up
```

### Phase 1 infrastructure

Start only the local infrastructure:

```bash
docker compose up -d postgres redis minio minio-create-bucket
```

Run database migrations:

```bash
docker compose --profile tools run --rm migrate
```

Verify the schema:

```bash
docker compose exec postgres psql -U postgres -d transcodex -c "\dt"
docker compose exec postgres psql -U postgres -d transcodex -c "\d jobs"
docker compose exec postgres psql -U postgres -d transcodex -c "\d job_outputs"
docker compose exec postgres psql -U postgres -d transcodex -c "\d workers"
```

Verify Redis priority-tier queue behavior:

```bash
docker compose exec redis redis-cli ZRANGE job_queue:priorities 0 -1 WITHSCORES
docker compose exec redis redis-cli LRANGE job_queue:priority:0 0 -1
docker compose exec redis redis-cli SCARD queued_jobs
```

Services:

| Service | URL |
|---|---|
| API server | http://localhost:8080 |
| Demo frontend | http://localhost:3000 |
| Observability dashboard | http://localhost:3001 |
| MinIO console | http://localhost:9001 |

Local MinIO only enables anonymous downloads for the `outputs/` prefix. Raw uploads under `raw/` are not public.

Scale workers:

```bash
docker compose up --scale worker=3
```

Run the local benchmark harness:

```bash
./scripts/benchmark.sh quick
./scripts/benchmark.sh portfolio

Run the worker-crash recovery check:

```bash
./scripts/failure-injection.sh
```

The recovery harness uses an isolated Compose project, terminates a worker after
the job enters processing, starts a replacement worker, and verifies that the
job completes with a second attempt and exactly one retry increment.
```

The quick profile validates the harness with short synthetic media. The portfolio profile compares sequential versus parallel processing across one, two, and four workers using fixed 30-second 1080p inputs. Reports are machine-specific and are written to the ignored `benchmark-results/` directory; do not copy their numbers into a resume until the run completes successfully.

The processing contract is at-least-once attempt execution with at-most-once visible completion. PostgreSQL stores durable job and attempt state; Redis is a recoverable scheduling index. Renewable attempt leases and conditional transitions prevent stale workers from publishing completion after recovery.

---

## API

### Upload a video

```
POST /uploads
Content-Type: multipart/form-data

file      required   raw video file
priority  optional   integer, default 0 (higher = processed first)
```

```json
202 Accepted
{ "job_id": "abc-123", "status": "queued" }
```

### Check job status

```
GET /jobs/{job_id}
```

```json
200 OK
{
  "job_id": "abc-123",
  "status": "completed",
  "retry_count": 0,
  "priority": 0,
  "created_at": "...",
  "updated_at": "...",
  "outputs": [
    { "type": "video_360p",  "cdn_url": "...", "file_size": 1024 },
    { "type": "video_720p",  "cdn_url": "...", "file_size": 2048 },
    { "type": "video_1080p", "cdn_url": "...", "file_size": 4096 },
    { "type": "thumbnail",   "cdn_url": "...", "file_size": 128  }
  ]
}
```

### Get outputs

```
GET /jobs/{job_id}/outputs
```

Returns CDN URLs for all processed outputs. Returns `409` if job is not yet completed.

### Health check

```
GET /healthz
```

Returns `200 OK` for load balancer and container health checks.

---

## Environment variables

| Variable | Description | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | — |
| `REDIS_URL` | Redis connection string | — |
| `STORAGE_ENDPOINT` | MinIO / S3 endpoint | — |
| `STORAGE_BUCKET` | Bucket name | — |
| `STORAGE_ACCESS_KEY` | Access key for local MinIO/static-key deployments. Leave unset on ECS when using task roles. | — |
| `STORAGE_SECRET_KEY` | Secret key for local MinIO/static-key deployments. Leave unset on ECS when using task roles. | — |
| `CDN_BASE_URL` | Base URL for CDN output links | — |
| `WORKER_MAX_RETRIES` | Max retry attempts per job | `3` |
| `WORKER_HEARTBEAT_INTERVAL` | Heartbeat frequency in seconds | `10` |
| `REAPER_SWEEP_INTERVAL` | Reaper sweep frequency in seconds | `30` |

---

## Project structure

```
transcodex/
  api/              API server
  worker/           Worker binary
  reaper/           Reaper binary
  frontend/
    demo/           Upload demo (React)
    dashboard/      Observability dashboard (React)
  docker-compose.yml
  .env.example
```

---

## Deployment

AWS deployment runbook: [docs/AWS-deployment.md](docs/AWS-deployment.md)

Recommended production stack:

- **ECS/Fargate** — API, worker, and reaper containers deployed as separate services
- **ALB** — API ingress and health checks (`GET /healthz`)
- **RDS** — PostgreSQL
- **ElastiCache** — Redis queue and locks
- **S3** — private raw uploads and processed outputs
- **CloudFront** — CDN routing for outputs, API paths, and frontends
- **ECR** — Docker image registry
- **Secrets Manager / SSM** — runtime configuration and secrets

Workers should scale independently from the API. The reaper can run as one small service, or as multiple replicas guarded by the Redis leadership lock.

---

## License

MIT
