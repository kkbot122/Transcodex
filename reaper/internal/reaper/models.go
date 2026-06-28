package reaper

import "time"

const (
	queueName     = "job_queue"
	reaperLockKey = "reaper_lock"

	statusQueued = "queued"
	statusDead   = "dead"
)

type QueueMessage struct {
	JobID      string    `json:"job_id"`
	InputFile  string    `json:"input_file"`
	Priority   int       `json:"priority"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

type Job struct {
	ID         string
	Status     string
	RetryCount int
	MaxRetries int
	Priority   int
	InputFile  string
}

type DeadWorker struct {
	ID         string
	CurrentJob *string
}
