# Transcodex

Transcodex is a distributed video processing pipeline. It accepts raw video uploads, creates transcoding jobs, processes videos into multiple resolutions with thumbnails, and serves finished outputs through object storage/CDN URLs.

The project is structured as a small backend infrastructure system:

- `api/` — Go API server for uploads, job status, and internal stats
- `worker/` — Go worker process for FFmpeg-based processing
- `reaper/` — Go recovery process for dead workers and orphaned jobs
- `frontend/demo/` — React upload demo
- `frontend/dashboard/` — React observability dashboard
- `docker-compose.yml` — local service orchestration

## Local Setup

```bash
cp .env.example .env
docker compose up
```

The full implementation is tracked in `docs/Tasks.md`.
