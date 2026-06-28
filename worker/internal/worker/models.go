package worker

import "time"

const (
	queueName = "job_queue"

	statusQueued     = "queued"
	statusProcessing = "processing"
	statusCompleted  = "completed"
	statusDead       = "dead"

	workerStatusIdle = "idle"
	workerStatusBusy = "busy"

	outputType360       = "video_360p"
	outputType720       = "video_720p"
	outputType1080      = "video_1080p"
	outputTypeThumbnail = "thumbnail"
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

type Output struct {
	Type        string
	LocalPath   string
	ObjectKey   string
	CDNURL      string
	FileSize    int64
	ContentType string
}

type ffmpegTask struct {
	OutputType  string
	FileName    string
	Args        func(inputPath, outputPath string) []string
	ContentType string
}
