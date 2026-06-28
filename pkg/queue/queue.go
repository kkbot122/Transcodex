package queue

import "time"

const (
	Name          = "job_queue"
	QueuedJobsSet = "queued_jobs"
)

func PriorityScore(priority int, enqueuedAt time.Time) float64 {
	return float64(priority)*1_000_000_000_000_000 - float64(enqueuedAt.UnixMilli())
}
