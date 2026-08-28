#!/usr/bin/env bash
set -euo pipefail

run_id="${1:?usage: aggregate-benchmarks.sh RUN_ID [OUTPUT]}"
root_dir="$(cd "$(dirname "$0")/.." && pwd)"
result_dir="$root_dir/benchmark-results"
output="${2:-$result_dir/${run_id}-summary.md}"
found=0
shopt -s nullglob
files=("$result_dir"/"$run_id"-sequential-*.json "$result_dir"/"$run_id"-parallel-*.json)

{
  echo "# Transcodex Benchmark"
  echo
  echo "- Run: \`$run_id\`"
  echo "- Results are grouped by isolated Compose stack; warm-up runs are excluded."
  echo
  echo "| Mode | Workers | Jobs | Repetition | Completed | Success | Jobs/min | Total duration | Queue p95 | FFmpeg p95 | Total p95 |"
  echo "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"
  for file in "${files[@]}"; do
    found=1
    filename="${file##*/}"
    filename="${filename%.json}"
    IFS='-' read -r _ mode worker_count job_count repetition <<< "$filename"
    jq -r --arg mode "$mode" --arg workers "$worker_count" --arg jobs "$job_count" --arg repetition "$repetition" \
      '"| \($mode) | \($workers) | \($jobs) | \($repetition) | \(.summary.completed)/\(.jobs) | \(.summary.success_rate * 100 | round)% | \(.summary.throughput_per_minute * 100 | round / 100) | \((.finished_at | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601) - (.started_at | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601))s | \(.summary.queue_wait_p95_ms // "n/a") ms | \(.summary.processing_p95_ms // "n/a") ms | \(.summary.total_p95_ms // "n/a") ms |"' "$file"
  done
  if [[ "$found" -eq 0 ]]; then
    echo "No benchmark reports found for $run_id" >&2
    exit 1
  fi
  echo
  echo "## Worker scaling and scheduling comparisons"
  echo
  echo "| Jobs | Workers | Sequential jobs/min | Parallel jobs/min | Parallel speedup | Sequential scaling | Parallel scaling |"
  echo "|---:|---:|---:|---:|---:|---:|---:|"
  dimensions=()
  for file in "${files[@]}"; do
    dimensions+=("$(jq -r '(.jobs|tostring) + ":" + (.workers|tostring)' "$file")")
  done
  job_counts=()
  while IFS= read -r value; do
    [[ -n "$value" ]] && job_counts+=("$value")
  done < <(printf '%s\n' "${dimensions[@]}" | cut -d: -f1 | sort -n -u)
  worker_counts=()
  while IFS= read -r value; do
    [[ -n "$value" ]] && worker_counts+=("$value")
  done < <(printf '%s\n' "${dimensions[@]}" | cut -d: -f2 | sort -n -u)
  for job_count in "${job_counts[@]}"; do
    for worker_count in "${worker_counts[@]}"; do
      sequential="$(jq -s -r --argjson workers "$worker_count" --argjson jobs "$job_count" '[.[] | select(.mode == "sequential" and .workers == $workers and .jobs == $jobs) | .summary.throughput_per_minute] | if length == 0 then "n/a" else (add / length) end' "${files[@]}" 2>/dev/null || true)"
      parallel="$(jq -s -r --argjson workers "$worker_count" --argjson jobs "$job_count" '[.[] | select(.mode == "parallel" and .workers == $workers and .jobs == $jobs) | .summary.throughput_per_minute] | if length == 0 then "n/a" else (add / length) end' "${files[@]}" 2>/dev/null || true)"
      sequential_scale="n/a"
      parallel_scale="n/a"
      speedup="n/a"
      if [[ "$sequential" != "n/a" && "$parallel" != "n/a" ]]; then
        speedup="$(awk -v seq="$sequential" -v par="$parallel" 'BEGIN { printf "%.2fx", par / seq }')"
      fi
      if [[ "$worker_count" -gt 1 ]]; then
        base_sequential="$(jq -s -r --argjson jobs "$job_count" '[.[] | select(.mode == "sequential" and .workers == 1 and .jobs == $jobs) | .summary.throughput_per_minute] | if length == 0 then "n/a" else (add / length) end' "${files[@]}" 2>/dev/null || true)"
        base_parallel="$(jq -s -r --argjson jobs "$job_count" '[.[] | select(.mode == "parallel" and .workers == 1 and .jobs == $jobs) | .summary.throughput_per_minute] | if length == 0 then "n/a" else (add / length) end' "${files[@]}" 2>/dev/null || true)"
        if [[ "$sequential" != "n/a" && "$base_sequential" != "n/a" ]]; then
          sequential_scale="$(awk -v current="$sequential" -v base="$base_sequential" 'BEGIN { printf "%.2fx", current / base }')"
        fi
        if [[ "$parallel" != "n/a" && "$base_parallel" != "n/a" ]]; then
          parallel_scale="$(awk -v current="$parallel" -v base="$base_parallel" 'BEGIN { printf "%.2fx", current / base }')"
        fi
      fi
      echo "| $job_count | $worker_count | $sequential | $parallel | $speedup | $sequential_scale | $parallel_scale |"
    done
  done
  echo
  echo "Speedup and scaling values are averages across available repetitions; use the raw reports for spread and p50/p95 latency."
} > "$output"

echo "Aggregate report written to $output"
