package server

import "time"

const (
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
	ID         string      `json:"job_id"`
	Status     string      `json:"status"`
	RetryCount int         `json:"retry_count"`
	MaxRetries int         `json:"max_retries,omitempty"`
	Priority   int         `json:"priority"`
	InputFile  string      `json:"input_file,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Timings    *JobTimings `json:"timings,omitempty"`
}

type JobTimings struct {
	QueueWaitMS  *int64 `json:"queue_wait_ms,omitempty"`
	DownloadMS   *int64 `json:"download_ms,omitempty"`
	ProcessingMS *int64 `json:"processing_ms,omitempty"`
	UploadMS     *int64 `json:"upload_ms,omitempty"`
	TotalMS      *int64 `json:"total_ms,omitempty"`
	AttemptCount int64  `json:"attempt_count"`
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
	OldestQueuedAge  *float64         `json:"oldest_queued_age_seconds,omitempty"`
	ActiveLeaseAge   *float64         `json:"active_lease_age_seconds,omitempty"`
	ThroughputPerMin int64            `json:"throughput_per_min"`
	Workers          map[string]int64 `json:"workers"`
	Jobs             map[string]int64 `json:"jobs"`
	Attempts         map[string]int64 `json:"attempts"`
	Latency          LatencyStats     `json:"latency"`
}

type LatencyStats struct {
	QueueWaitP50MS  *int64 `json:"queue_wait_p50_ms,omitempty"`
	QueueWaitP95MS  *int64 `json:"queue_wait_p95_ms,omitempty"`
	ProcessingP50MS *int64 `json:"processing_p50_ms,omitempty"`
	ProcessingP95MS *int64 `json:"processing_p95_ms,omitempty"`
	TotalP50MS      *int64 `json:"total_p50_ms,omitempty"`
	TotalP95MS      *int64 `json:"total_p95_ms,omitempty"`
}
