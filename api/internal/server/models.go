package server

import "time"

const (
	queueName = "job_queue"

	statusQueued     = "queued"
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusDead       = "dead"
)

type QueueMessage struct {
	JobID      string    `json:"job_id"`
	InputFile  string    `json:"input_file"`
	Priority   int       `json:"priority"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

type Job struct {
	ID         string    `json:"job_id"`
	Status     string    `json:"status"`
	RetryCount int       `json:"retry_count"`
	MaxRetries int       `json:"max_retries,omitempty"`
	Priority   int       `json:"priority"`
	InputFile  string    `json:"input_file,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type JobOutput struct {
	ID        string    `json:"id,omitempty"`
	JobID     string    `json:"job_id,omitempty"`
	Type      string    `json:"type"`
	CDNURL    string    `json:"cdn_url"`
	FileSize  int64     `json:"file_size"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type Worker struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	CurrentJob    *string   `json:"current_job"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
}

type Stats struct {
	QueueDepth       int64            `json:"queue_depth"`
	ThroughputPerMin int64            `json:"throughput_per_min"`
	Workers          map[string]int64 `json:"workers"`
	Jobs             map[string]int64 `json:"jobs"`
}
