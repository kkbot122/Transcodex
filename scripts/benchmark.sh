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

docker compose run --rm -T -v "$fixture_dir:/benchmark" worker \
  ffmpeg -y -f lavfi -i "testsrc2=size=$size:rate=30" -f lavfi -i "sine=frequency=1000:sample_rate=48000" \
  -t "$duration" -c:v libx264 -preset veryfast -pix_fmt yuv420p -c:a aac /benchmark/input.mp4

run_id="$(date -u +%Y%m%dT%H%M%SZ)"
for mode in sequential parallel; do
  for worker_count in "${workers[@]}"; do
    for job_count in "${jobs[@]}"; do
      for repetition in $(seq 1 "$repetitions"); do
        project="transcodex-bench-${run_id}-${mode}-${worker_count}-${job_count}-${repetition}"
        output="$result_dir/${run_id}-${mode}-${worker_count}-${job_count}-${repetition}.json"
        markdown="$result_dir/${run_id}-${mode}-${worker_count}-${job_count}-${repetition}.md"
        WORKER_PROCESSING_MODE="$mode" docker compose -p "$project" up -d --build --scale worker="$worker_count" api reaper worker
        go run ./cmd/benchmark --base-url http://localhost:8080 --input "$fixture_dir/input.mp4" \
          --jobs "$job_count" --mode "$mode" --workers "$worker_count" --run-id "$run_id" --output "$output" --markdown "$markdown"
        docker compose -p "$project" down -v --remove-orphans
      done
    done
  done
done

echo "Benchmark JSON reports written to $result_dir"
