#!/usr/bin/env bash
set -euo pipefail

profile="${1:-quick}"
root_dir="$(cd "$(dirname "$0")/.." && pwd)"
result_dir="$root_dir/benchmark-results"
fixture_dir="$root_dir/.benchmark"
mkdir -p "$result_dir" "$fixture_dir"

case "$profile" in
  quick) duration=5; size=1280x720; jobs=(3); workers=(1 2); repetitions=1 ;;
  portfolio) duration=30; size=1920x1080; jobs=(10 25 50); workers=(1 2 4); repetitions=3 ;;
  *) echo "usage: $0 quick|portfolio" >&2; exit 2 ;;
esac

docker compose run --rm -T --entrypoint ffmpeg -v "$fixture_dir:/benchmark" worker \
  -y -f lavfi -i "testsrc2=size=$size:rate=30" -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -t "$duration" -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac /benchmark/input.mp4

run_id="$(date -u +%Y%m%dt%H%M%S)"
for mode in sequential parallel; do
  for worker_count in "${workers[@]}"; do
    for job_count in "${jobs[@]}"; do
      for repetition in $(seq 1 "$repetitions"); do
        project="transcodex-bench-${run_id}-${mode}-${worker_count}-${job_count}-${repetition}"
        output="$result_dir/${run_id}-${mode}-${worker_count}-${job_count}-${repetition}.json"
        markdown="$result_dir/${run_id}-${mode}-${worker_count}-${job_count}-${repetition}.md"
        api_port=$((18080 + (worker_count * 100) + job_count + repetition))
        postgres_port=$((15000 + (worker_count * 100) + job_count + repetition))
        redis_port=$((16000 + (worker_count * 100) + job_count + repetition))
        minio_port=$((19000 + (worker_count * 100) + job_count + repetition))
        minio_console_port=$((20000 + (worker_count * 100) + job_count + repetition))
        compose_env=(TRANSCODEX_API_PORT="$api_port" TRANSCODEX_POSTGRES_PORT="$postgres_port" TRANSCODEX_REDIS_PORT="$redis_port" TRANSCODEX_MINIO_PORT="$minio_port" TRANSCODEX_MINIO_CONSOLE_PORT="$minio_console_port")
        WORKER_PROCESSING_MODE="$mode" env "${compose_env[@]}" docker compose -p "$project" up -d --build --scale worker="$worker_count" api reaper worker
        env "${compose_env[@]}" go run ./cmd/benchmark --base-url "http://localhost:$api_port" --input "$fixture_dir/input.mp4" \
          --jobs 1 --mode "$mode" --workers "$worker_count" --run-id "${run_id}-warmup-${mode}-${worker_count}-${job_count}-${repetition}" --timeout 10m --output /tmp/transcodex-warmup.json
        env "${compose_env[@]}" go run ./cmd/benchmark --base-url "http://localhost:$api_port" --input "$fixture_dir/input.mp4" \
          --jobs "$job_count" --mode "$mode" --workers "$worker_count" --run-id "$run_id" --output "$output" --markdown "$markdown"
        docker compose -p "$project" down -v --remove-orphans
      done
    done
  done
done

echo "Benchmark JSON reports written to $result_dir"
