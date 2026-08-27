#!/usr/bin/env bash
set -euo pipefail

root_dir="$(cd "$(dirname "$0")/.." && pwd)"
project="transcodex-failure-$(date -u +%Y%m%dt%H%M%S)"
fixture_dir="$root_dir/.failure-injection"
api_port=18180
postgres_port=15432
redis_port=16379
minio_port=19000
minio_console_port=19001
base_url="http://localhost:$api_port"

cleanup() {
	TRANSCODEX_API_PORT="$api_port" TRANSCODEX_POSTGRES_PORT="$postgres_port" \
	TRANSCODEX_REDIS_PORT="$redis_port" TRANSCODEX_MINIO_PORT="$minio_port" \
	TRANSCODEX_MINIO_CONSOLE_PORT="$minio_console_port" \
		docker compose -p "$project" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

mkdir -p "$fixture_dir"
docker compose run --rm -T --entrypoint ffmpeg -v "$fixture_dir:/benchmark" worker \
	-y -f lavfi -i 'testsrc2=size=1280x720:rate=30' -f lavfi -i 'sine=frequency=1000:sample_rate=48000' \
	-t 15 -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac /benchmark/input.mp4 >/dev/null 2>&1

TRANSCODEX_API_PORT="$api_port" TRANSCODEX_POSTGRES_PORT="$postgres_port" \
TRANSCODEX_REDIS_PORT="$redis_port" TRANSCODEX_MINIO_PORT="$minio_port" \
TRANSCODEX_MINIO_CONSOLE_PORT="$minio_console_port" \
WORKER_LEASE_TTL_SECONDS=5 REAPER_SWEEP_INTERVAL=2 REAPER_LEADERSHIP_LOCK_TTL_SECONDS=10 \
	WORKER_MAX_RETRIES=3 docker compose -p "$project" up -d --build api reaper worker >/dev/null

job_id="$(curl -fsS -F "file=@$fixture_dir/input.mp4" "$base_url/uploads" | jq -r '.job_id')"
echo "submitted job=$job_id project=$project"

for _ in $(seq 1 60); do
	status="$(curl -fsS "$base_url/jobs/$job_id" | jq -r '.status')"
	if [[ "$status" == "processing" ]]; then
		break
	fi
	sleep 0.5
done
if [[ "${status:-}" != "processing" ]]; then
	echo "job never entered processing (status=${status:-unknown})" >&2
	exit 1
fi

echo "injecting worker termination"
TRANSCODEX_API_PORT="$api_port" TRANSCODEX_POSTGRES_PORT="$postgres_port" \
	TRANSCODEX_REDIS_PORT="$redis_port" TRANSCODEX_MINIO_PORT="$minio_port" \
	TRANSCODEX_MINIO_CONSOLE_PORT="$minio_console_port" \
	docker compose -p "$project" rm -sf worker >/dev/null
TRANSCODEX_API_PORT="$api_port" TRANSCODEX_POSTGRES_PORT="$postgres_port" \
	TRANSCODEX_REDIS_PORT="$redis_port" TRANSCODEX_MINIO_PORT="$minio_port" \
	TRANSCODEX_MINIO_CONSOLE_PORT="$minio_console_port" \
	WORKER_LEASE_TTL_SECONDS=5 REAPER_SWEEP_INTERVAL=2 REAPER_LEADERSHIP_LOCK_TTL_SECONDS=10 \
	WORKER_MAX_RETRIES=3 docker compose -p "$project" up -d --scale worker=1 worker >/dev/null

for _ in $(seq 1 120); do
	payload="$(curl -fsS "$base_url/jobs/$job_id")"
	status="$(jq -r '.status' <<<"$payload")"
	if [[ "$status" == "completed" || "$status" == "dead" ]]; then
		attempts="$(jq -r '.timings.attempt_count // 0' <<<"$payload")"
		retries="$(jq -r '.retry_count // 0' <<<"$payload")"
		echo "terminal status=$status attempts=$attempts retries=$retries"
		if [[ "$status" != "completed" || "$attempts" -lt 2 || "$retries" -lt 1 ]]; then
			echo "worker crash did not produce a replacement attempt" >&2
			exit 1
		fi
		exit 0
	fi
	sleep 1
done

echo "job did not recover before timeout" >&2
exit 1
