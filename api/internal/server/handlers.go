package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

var errUploadTooLarge = errors.New("upload exceeds size limit")

func (s *Server) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.config.UploadSizeLimit+(1<<20))

	reader, err := c.Request.MultipartReader()
	if err != nil {
		respondError(c, http.StatusBadRequest, "multipart form required")
		return
	}

	jobID := uuid.NewString()
	priority := 0
	fileUploaded := false
	inputFile := ""

	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			respondError(c, http.StatusBadRequest, "invalid multipart form")
			return
		}

		switch part.FormName() {
		case "priority":
			parsed, err := readPriority(part)
			if err != nil {
				if fileUploaded {
					s.removeStoredObject(c.Request.Context(), inputFile)
				}
				respondError(c, http.StatusBadRequest, "priority must be an integer")
				return
			}
			priority = parsed
		case "file":
			if fileUploaded {
				respondError(c, http.StatusBadRequest, "only one file is supported")
				return
			}

			key, err := s.storeUpload(c.Request.Context(), jobID, part)
			if err != nil {
				status := http.StatusBadRequest
				message := err.Error()
				if errors.Is(err, errUploadTooLarge) {
					message = "file exceeds upload size limit"
				} else if !errors.Is(err, errInvalidVideoType) {
					status = http.StatusInternalServerError
					message = "store upload failed"
				}
				respondError(c, status, message)
				return
			}
			inputFile = key
			fileUploaded = true
		default:
			_, _ = io.Copy(io.Discard, part)
		}
	}

	if !fileUploaded {
		respondError(c, http.StatusBadRequest, "file is required")
		return
	}

	if err := s.createQueuedJob(c.Request.Context(), jobID, inputFile, priority); err != nil {
		s.removeStoredObject(c.Request.Context(), inputFile)
		respondError(c, http.StatusInternalServerError, "create job failed")
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"job_id": jobID, "status": statusQueued})
}

func (s *Server) getJob(c *gin.Context) {
	job, outputs, err := s.fetchJobWithOutputs(c.Request.Context(), c.Param("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(c, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "fetch job failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"job_id":      job.ID,
		"status":      job.Status,
		"retry_count": job.RetryCount,
		"priority":    job.Priority,
		"created_at":  job.CreatedAt,
		"updated_at":  job.UpdatedAt,
		"outputs":     outputs,
	})
}

func (s *Server) getJobOutputs(c *gin.Context) {
	job, err := s.fetchJob(c.Request.Context(), c.Param("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(c, http.StatusNotFound, "job not found")
		return
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "fetch job failed")
		return
	}
	if job.Status != statusCompleted {
		respondError(c, http.StatusConflict, "job not yet completed")
		return
	}

	outputs, err := s.fetchOutputs(c.Request.Context(), job.ID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "fetch outputs failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"outputs": outputs})
}

func (s *Server) getStats(c *gin.Context) {
	stats, err := s.collectStats(c.Request.Context())
	if err != nil {
		respondError(c, http.StatusInternalServerError, "fetch stats failed")
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (s *Server) getWorkers(c *gin.Context) {
	rows, err := s.db.Query(c.Request.Context(), `
		SELECT id::text, status::text, current_job::text, last_heartbeat
		FROM workers
		ORDER BY last_heartbeat DESC
	`)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "fetch workers failed")
		return
	}
	defer rows.Close()

	workers := []Worker{}
	for rows.Next() {
		var worker Worker
		if err := rows.Scan(&worker.ID, &worker.Status, &worker.CurrentJob, &worker.LastHeartbeat); err != nil {
			respondError(c, http.StatusInternalServerError, "scan workers failed")
			return
		}
		workers = append(workers, worker)
	}
	if err := rows.Err(); err != nil {
		respondError(c, http.StatusInternalServerError, "scan workers failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"workers": workers})
}

func (s *Server) getInternalJobs(c *gin.Context) {
	limit := parseLimit(c.Query("limit"), 50, 200)
	status := c.Query("status")
	if status != "" && !isValidJobStatus(status) {
		respondError(c, http.StatusBadRequest, "invalid status")
		return
	}

	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = s.db.Query(c.Request.Context(), `
			SELECT id::text, status::text, retry_count, priority, input_file, created_at, updated_at
			FROM jobs
			ORDER BY updated_at DESC
			LIMIT $1
		`, limit)
	} else {
		rows, err = s.db.Query(c.Request.Context(), `
			SELECT id::text, status::text, retry_count, priority, input_file, created_at, updated_at
			FROM jobs
			WHERE status = $1::job_status
			ORDER BY updated_at DESC
			LIMIT $2
		`, status, limit)
	}
	if err != nil {
		respondError(c, http.StatusInternalServerError, "fetch jobs failed")
		return
	}
	defer rows.Close()

	jobs := []Job{}
	for rows.Next() {
		var job Job
		if err := rows.Scan(&job.ID, &job.Status, &job.RetryCount, &job.Priority, &job.InputFile, &job.CreatedAt, &job.UpdatedAt); err != nil {
			respondError(c, http.StatusInternalServerError, "scan jobs failed")
			return
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		respondError(c, http.StatusInternalServerError, "scan jobs failed")
		return
	}

	c.JSON(http.StatusOK, gin.H{"jobs": jobs})
}

func (s *Server) streamStats(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		respondError(c, http.StatusInternalServerError, "streaming unsupported")
		return
	}

	ticker := time.NewTicker(s.config.StatsStreamPeriod)
	defer ticker.Stop()

	for {
		if err := s.writeStatsEvent(c.Request.Context(), c.Writer, flusher); err != nil {
			return
		}

		select {
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) writeStatsEvent(ctx context.Context, writer io.Writer, flusher http.Flusher) error {
	stats, err := s.collectStats(ctx)
	if err != nil {
		return err
	}

	payload, err := json.Marshal(stats)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(writer, "data: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *Server) collectStats(ctx context.Context) (Stats, error) {
	stats := Stats{
		Workers: map[string]int64{"total": 0, "idle": 0, "busy": 0, "dead": 0},
		Jobs:    map[string]int64{statusQueued: 0, statusProcessing: 0, statusCompleted: 0, statusDead: 0},
	}

	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		depth, err := s.redis.ZCard(ctx, queueName).Result()
		if err == nil {
			stats.QueueDepth = depth
		}
		return err
	})
	group.Go(func() error {
		rows, err := s.db.Query(ctx, `SELECT status::text, count(*) FROM jobs GROUP BY status`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err != nil {
				return err
			}
			stats.Jobs[status] = count
		}
		return rows.Err()
	})
	group.Go(func() error {
		rows, err := s.db.Query(ctx, `SELECT status::text, count(*) FROM workers GROUP BY status`)
		if err != nil {
			return err
		}
		defer rows.Close()
		var total int64
		for rows.Next() {
			var status string
			var count int64
			if err := rows.Scan(&status, &count); err != nil {
				return err
			}
			stats.Workers[status] = count
			total += count
		}
		stats.Workers["total"] = total
		return rows.Err()
	})
	group.Go(func() error {
		return s.db.QueryRow(ctx, `
			SELECT count(*)
			FROM jobs
			WHERE status = 'completed' AND updated_at >= now() - interval '1 minute'
		`).Scan(&stats.ThroughputPerMin)
	})

	return stats, group.Wait()
}

func (s *Server) createQueuedJob(ctx context.Context, jobID, inputFile string, priority int) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		INSERT INTO jobs (id, status, retry_count, max_retries, priority, input_file)
		VALUES ($1, 'queued', 0, $2, $3, $4)
	`, jobID, s.config.WorkerMaxRetries, priority, inputFile); err != nil {
		return err
	}

	enqueuedAt := time.Now().UTC()
	payload, err := json.Marshal(QueueMessage{
		JobID:      jobID,
		InputFile:  inputFile,
		Priority:   priority,
		EnqueuedAt: enqueuedAt,
	})
	if err != nil {
		return err
	}

	if err := s.redis.ZAdd(ctx, queueName, redis.Z{
		Score:  priorityScore(priority, enqueuedAt),
		Member: payload,
	}).Err(); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Server) fetchJob(ctx context.Context, id string) (Job, error) {
	var job Job
	err := s.db.QueryRow(ctx, `
		SELECT id::text, status::text, retry_count, max_retries, priority, input_file, created_at, updated_at
		FROM jobs
		WHERE id = $1
	`, id).Scan(&job.ID, &job.Status, &job.RetryCount, &job.MaxRetries, &job.Priority, &job.InputFile, &job.CreatedAt, &job.UpdatedAt)
	return job, err
}

func (s *Server) fetchJobWithOutputs(ctx context.Context, id string) (Job, []JobOutput, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			j.id::text,
			j.status::text,
			j.retry_count,
			j.max_retries,
			j.priority,
			j.input_file,
			j.created_at,
			j.updated_at,
			o.id::text,
			o.job_id::text,
			o.type::text,
			o.cdn_url,
			o.file_size,
			o.created_at
		FROM jobs j
		LEFT JOIN job_outputs o ON o.job_id = j.id
		WHERE j.id = $1
		ORDER BY o.created_at ASC
	`, id)
	if err != nil {
		return Job{}, nil, err
	}
	defer rows.Close()

	var job Job
	outputs := []JobOutput{}
	found := false
	for rows.Next() {
		found = true
		var output JobOutput
		var outputID *string
		var outputJobID *string
		var outputType *string
		var cdnURL *string
		var fileSize *int64
		var outputCreatedAt *time.Time
		if err := rows.Scan(
			&job.ID,
			&job.Status,
			&job.RetryCount,
			&job.MaxRetries,
			&job.Priority,
			&job.InputFile,
			&job.CreatedAt,
			&job.UpdatedAt,
			&outputID,
			&outputJobID,
			&outputType,
			&cdnURL,
			&fileSize,
			&outputCreatedAt,
		); err != nil {
			return Job{}, nil, err
		}
		if outputID != nil {
			output.ID = *outputID
			output.JobID = *outputJobID
			output.Type = *outputType
			output.CDNURL = *cdnURL
			output.FileSize = *fileSize
			output.CreatedAt = *outputCreatedAt
			outputs = append(outputs, output)
		}
	}
	if err := rows.Err(); err != nil {
		return Job{}, nil, err
	}
	if !found {
		return Job{}, nil, pgx.ErrNoRows
	}
	return job, outputs, nil
}

func (s *Server) fetchOutputs(ctx context.Context, jobID string) ([]JobOutput, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id::text, job_id::text, type::text, cdn_url, file_size, created_at
		FROM job_outputs
		WHERE job_id = $1
		ORDER BY created_at ASC
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	outputs := []JobOutput{}
	for rows.Next() {
		var output JobOutput
		if err := rows.Scan(&output.ID, &output.JobID, &output.Type, &output.CDNURL, &output.FileSize, &output.CreatedAt); err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return outputs, rows.Err()
}

func readPriority(part *multipart.Part) (int, error) {
	body, err := io.ReadAll(io.LimitReader(part, 32))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(body)))
}

var errInvalidVideoType = errors.New("file must be a video")

func (s *Server) storeUpload(ctx context.Context, jobID string, part *multipart.Part) (string, error) {
	header := make([]byte, 512)
	n, err := io.ReadFull(part, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", err
	}
	header = header[:n]

	contentType := part.Header.Get("Content-Type")
	detectedType := http.DetectContentType(header)
	if strings.HasPrefix(detectedType, "video/") {
		contentType = detectedType
	} else if !(strings.HasPrefix(contentType, "video/") && detectedType == "application/octet-stream") {
		return "", errInvalidVideoType
	}

	filename := filepath.Base(part.FileName())
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		filename = "input"
	}
	key := fmt.Sprintf("raw/%s/%s", jobID, filename)
	body := &sizeLimitedReader{
		reader: io.MultiReader(bytes.NewReader(header), part),
		limit:  s.config.UploadSizeLimit,
	}

	_, err = s.storage.PutObject(ctx, s.storage.Bucket, key, body, -1, minio.PutObjectOptions{
		ContentType: contentType,
		PartSize:    16 << 20,
	})
	if err != nil {
		s.removeStoredObject(ctx, key)
		if errors.Is(err, errUploadTooLarge) {
			return "", errUploadTooLarge
		}
		return "", err
	}

	return key, nil
}

func (s *Server) removeStoredObject(ctx context.Context, key string) {
	if key == "" {
		return
	}
	_ = s.storage.RemoveObject(ctx, s.storage.Bucket, key, minio.RemoveObjectOptions{})
}

type sizeLimitedReader struct {
	reader io.Reader
	limit  int64
	read   int64
}

func (r *sizeLimitedReader) Read(p []byte) (int, error) {
	if r.read >= r.limit {
		var probe [1]byte
		n, err := r.reader.Read(probe[:])
		if n > 0 {
			return 0, errUploadTooLarge
		}
		return 0, err
	}

	remaining := r.limit - r.read
	if int64(len(p)) > remaining {
		p = p[:remaining]
	}

	n, err := r.reader.Read(p)
	r.read += int64(n)
	return n, err
}

func parseLimit(raw string, fallback, max int) int {
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func isValidJobStatus(status string) bool {
	switch status {
	case statusQueued, statusProcessing, statusCompleted, statusDead:
		return true
	default:
		return false
	}
}

func priorityScore(priority int, enqueuedAt time.Time) float64 {
	return float64(priority)*1_000_000_000_000_000 - float64(enqueuedAt.UnixMilli())
}
