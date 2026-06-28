# transcodex

A distributed video processing pipeline. Upload a raw video, get back multiple resolutions and a thumbnail — delivered via CDN.

Built as a backend infrastructure service. Any application can integrate via a simple upload API and poll for job completion.

---

## What it does

- Accepts raw video uploads via REST API
- Transcodes to 360p, 720p, and 1080p in parallel
- Extracts a thumbnail
- Tracks job state through a full lifecycle — queued → processing → completed or dead
- Retries failed jobs automatically, recovers from worker crashes
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

A reaper process sweeps every 30 seconds to detect dead workers, orphaned jobs, and queued jobs missing from Redis, requeuing them automatically.

---

## Stack

| Layer | Technology |
|---|---|
| API + Workers + Reaper | Go |
| Queue | Redis (sorted set, priority-based) |
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

Verify Redis sorted-set queue behavior:

```bash
docker compose exec redis redis-cli ZADD job_queue 100 '{"job_id":"demo-high","priority":100}'
docker compose exec redis redis-cli ZADD job_queue 10 '{"job_id":"demo-low","priority":10}'
docker compose exec redis redis-cli ZPOPMAX job_queue
```

Services:

| Service | URL |
|---|---|
| API server | http://localhost:8080 |
| Demo frontend | http://localhost:3000 |
| Observability dashboard | http://localhost:3001 |
| MinIO console | http://localhost:9001 |

Scale workers:

```bash
docker compose up --scale worker=3
```

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

---

## Environment variables

| Variable | Description | Default |
|---|---|---|
| `DATABASE_URL` | PostgreSQL connection string | — |
| `REDIS_URL` | Redis connection string | — |
| `STORAGE_ENDPOINT` | MinIO / S3 endpoint | — |
| `STORAGE_BUCKET` | Bucket name | — |
| `STORAGE_ACCESS_KEY` | Access key | — |
| `STORAGE_SECRET_KEY` | Secret key | — |
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

Deployed on AWS. Production stack:

- **EC2** — API server and workers
- **RDS** — PostgreSQL
- **ElastiCache** — Redis
- **S3** — video storage
- **CloudFront** — CDN delivery
- **ALB** — load balancer

---

## License

MIT
