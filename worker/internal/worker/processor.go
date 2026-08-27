package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kisna/transcodex/pkg/queue"
	"github.com/minio/minio-go/v7"
	"golang.org/x/sync/errgroup"
)

var errJobAlreadyClaimed = errors.New("job is not queued")
var errStaleAttempt = errors.New("attempt is no longer current")

func (w *Worker) processJob(ctx context.Context, msg QueueMessage) error {
	attemptID, claimed, err := w.markProcessing(ctx, msg.JobID)
	if err != nil {
		slog.Error("claim job", "worker_id", w.id, "job_id", msg.JobID, "error", err)
		return err
	}
	if !claimed {
		return errJobAlreadyClaimed
	}

	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leaseDone := make(chan struct{})
	go func() {
		defer close(leaseDone)
		w.renewLeaseUntilDone(jobCtx, attemptID, cancel)
	}()
	defer func() {
		cancel()
		<-leaseDone
	}()

	if err := w.setWorkerBusy(ctx, msg.JobID); err != nil {
		w.handleFailure(ctx, msg.JobID, attemptID, err)
		return err
	}
	defer func() {
		if err := w.setWorkerIdle(context.Background()); err != nil {
			slog.Error("set worker idle", "worker_id", w.id, "job_id", msg.JobID, "error", err)
		}
	}()

	if err := w.runJob(jobCtx, msg, attemptID); err != nil {
		if errors.Is(err, errStaleAttempt) {
			return nil
		}
		w.handleFailure(ctx, msg.JobID, attemptID, err)
		return err
	}

	return nil
}

func (w *Worker) runJob(ctx context.Context, msg QueueMessage, attemptID string) error {
	tempRoot := w.config.TempDir
	if tempRoot == "" {
		tempRoot = os.TempDir()
	}
	workDir := filepath.Join(tempRoot, msg.JobID)
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		return fmt.Errorf("create work dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(workDir); err != nil {
			slog.Error("cleanup work dir", "worker_id", w.id, "job_id", msg.JobID, "work_dir", workDir, "error", err)
		}
	}()

	inputPath := filepath.Join(workDir, "raw.mp4")
	if err := w.setAttemptPhase(ctx, attemptID, "downloading"); err != nil {
		return err
	}
	started := time.Now()
	if err := w.downloadInput(ctx, msg.InputFile, inputPath); err != nil {
		return err
	}
	if err := w.recordAttemptDuration(ctx, attemptID, "download_duration_ms", time.Since(started)); err != nil {
		return err
	}

	if err := w.setAttemptPhase(ctx, attemptID, "processing"); err != nil {
		return err
	}
	started = time.Now()
	outputs, err := runFFmpegJobs(ctx, w.config.FFmpegPath, inputPath, workDir, w.config.ProcessingMode)
	if err != nil {
		return err
	}
	if err := w.recordAttemptDuration(ctx, attemptID, "processing_duration_ms", time.Since(started)); err != nil {
		return err
	}

	if err := w.setAttemptPhase(ctx, attemptID, "uploading"); err != nil {
		return err
	}
	started = time.Now()
	if err := w.uploadOutputs(ctx, msg.JobID, attemptID, outputs); err != nil {
		return err
	}
	if err := w.recordAttemptDuration(ctx, attemptID, "upload_duration_ms", time.Since(started)); err != nil {
		return err
	}

	if err := w.setAttemptPhase(ctx, attemptID, "completing"); err != nil {
		return err
	}
	if err := w.completeJob(ctx, msg.JobID, attemptID, outputs); err != nil {
		return err
	}

	slog.Info("job completed", "worker_id", w.id, "job_id", msg.JobID)
	return nil
}

func (w *Worker) markProcessing(ctx context.Context, jobID string) (string, bool, error) {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var retryCount int
	var queueEnteredAt time.Time
	if err := tx.QueryRow(ctx, `
		SELECT status::text, retry_count, queue_entered_at
		FROM jobs WHERE id = $1 FOR UPDATE
	`, jobID).Scan(&status, &retryCount, &queueEnteredAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		return "", false, err
	}
	if status != statusQueued {
		return "", false, nil
	}
	attemptID := uuid.NewString()
	if _, err := tx.Exec(ctx, `
		INSERT INTO job_attempts (id, job_id, attempt_number, worker_id, queue_entered_at, lease_expires_at, processing_mode)
		VALUES ($1, $2, $3, $4, $5, now() + ($6::double precision * interval '1 second'), $7)
	`, attemptID, jobID, retryCount+1, w.id, queueEnteredAt, w.config.LeaseTTL.Seconds(), w.config.ProcessingMode); err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE jobs SET status = 'processing', current_attempt_id = $2
		WHERE id = $1 AND status = 'queued' AND current_attempt_id IS NULL
	`, jobID, attemptID); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return attemptID, true, nil
}

func (w *Worker) renewLeaseUntilDone(ctx context.Context, attemptID string, cancel context.CancelFunc) {
	period := w.config.LeaseTTL / 3
	if period <= 0 {
		period = time.Second
	}
	ticker := time.NewTicker(period)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			updated, err := w.renewAttemptLease(ctx, attemptID)
			if err != nil {
				slog.Error("renew attempt lease", "worker_id", w.id, "attempt_id", attemptID, "error", err)
				cancel()
				return
			}
			if !updated {
				slog.Warn("attempt lease lost", "worker_id", w.id, "attempt_id", attemptID)
				cancel()
				return
			}
		}
	}
}

func (w *Worker) renewAttemptLease(ctx context.Context, attemptID string) (bool, error) {
	tag, err := w.db.Exec(ctx, `
		UPDATE job_attempts a
		SET last_heartbeat = now(),
			lease_expires_at = now() + ($2::double precision * interval '1 second')
		FROM jobs j
		WHERE a.id = $1 AND a.job_id = j.id
			AND a.status = 'running' AND j.status = 'processing'
			AND j.current_attempt_id = a.id
	`, attemptID, w.config.LeaseTTL.Seconds())
	return tag.RowsAffected() == 1, err
}

func (w *Worker) setAttemptPhase(ctx context.Context, attemptID, phase string) error {
	_, err := w.db.Exec(ctx, `
		UPDATE job_attempts SET current_phase = $2
		WHERE id = $1 AND status = 'running'
	`, attemptID, phase)
	return err
}

func (w *Worker) recordAttemptDuration(ctx context.Context, attemptID, field string, duration time.Duration) error {
	query := ""
	switch field {
	case "download_duration_ms":
		query = "UPDATE job_attempts SET download_duration_ms = $2 WHERE id = $1"
	case "processing_duration_ms":
		query = "UPDATE job_attempts SET processing_duration_ms = $2 WHERE id = $1"
	case "upload_duration_ms":
		query = "UPDATE job_attempts SET upload_duration_ms = $2 WHERE id = $1"
	default:
		return fmt.Errorf("unsupported attempt duration field %q", field)
	}
	_, err := w.db.Exec(ctx, query, attemptID, duration.Milliseconds())
	return err
}

func (w *Worker) setWorkerBusy(ctx context.Context, jobID string) error {
	_, err := w.db.Exec(ctx, `
		UPDATE workers
		SET status = 'busy',
			current_job = $2,
			last_heartbeat = now()
		WHERE id = $1
	`, w.id, jobID)
	return err
}

func (w *Worker) setWorkerIdle(ctx context.Context) error {
	_, err := w.db.Exec(ctx, `
		UPDATE workers
		SET status = 'idle',
			current_job = NULL,
			last_heartbeat = now()
		WHERE id = $1
	`, w.id)
	return err
}

func (w *Worker) downloadInput(ctx context.Context, key string, destination string) error {
	object, err := w.storage.GetObject(ctx, w.storage.Bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return fmt.Errorf("download input: %w", err)
	}
	defer object.Close()

	file, err := os.Create(destination)
	if err != nil {
		return fmt.Errorf("create raw file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, object); err != nil {
		return fmt.Errorf("write raw file: %w", err)
	}
	return nil
}

func runFFmpegJobs(ctx context.Context, ffmpegPath string, inputPath string, workDir string, mode processingMode) ([]Output, error) {
	tasks := []ffmpegTask{
		{
			OutputType:  outputType360,
			FileName:    "360p.mp4",
			ContentType: "video/mp4",
			Args: func(inputPath, outputPath string) []string {
				return []string{"-y", "-i", inputPath, "-vf", "scale=-2:360", "-c:v", "libx264", "-preset", "veryfast", "-c:a", "aac", "-movflags", "+faststart", outputPath}
			},
		},
		{
			OutputType:  outputType720,
			FileName:    "720p.mp4",
			ContentType: "video/mp4",
			Args: func(inputPath, outputPath string) []string {
				return []string{"-y", "-i", inputPath, "-vf", "scale=-2:720", "-c:v", "libx264", "-preset", "veryfast", "-c:a", "aac", "-movflags", "+faststart", outputPath}
			},
		},
		{
			OutputType:  outputType1080,
			FileName:    "1080p.mp4",
			ContentType: "video/mp4",
			Args: func(inputPath, outputPath string) []string {
				return []string{"-y", "-i", inputPath, "-vf", "scale=-2:1080", "-c:v", "libx264", "-preset", "veryfast", "-c:a", "aac", "-movflags", "+faststart", outputPath}
			},
		},
		{
			OutputType:  outputTypeThumbnail,
			FileName:    "thumbnail.jpg",
			ContentType: "image/jpeg",
			Args: func(inputPath, outputPath string) []string {
				return []string{"-y", "-ss", "00:00:01", "-i", inputPath, "-frames:v", "1", "-q:v", "2", outputPath}
			},
		},
	}

	outputs := make([]Output, len(tasks))
	if mode == processingModeSequential {
		for i, task := range tasks {
			outputPath := filepath.Join(workDir, task.FileName)
			if err := runFFmpeg(ctx, ffmpegPath, task.Args(inputPath, outputPath)); err != nil {
				return nil, fmt.Errorf("%s: %w", task.OutputType, err)
			}
			info, err := os.Stat(outputPath)
			if err != nil {
				return nil, fmt.Errorf("%s stat: %w", task.OutputType, err)
			}
			outputs[i] = Output{Type: task.OutputType, LocalPath: outputPath, FileSize: info.Size(), ContentType: task.ContentType}
		}
		return outputs, nil
	}

	group, groupCtx := errgroup.WithContext(ctx)

	for i, task := range tasks {
		i, task := i, task
		group.Go(func() error {
			outputPath := filepath.Join(workDir, task.FileName)
			if err := runFFmpeg(groupCtx, ffmpegPath, task.Args(inputPath, outputPath)); err != nil {
				return fmt.Errorf("%s: %w", task.OutputType, err)
			}
			info, err := os.Stat(outputPath)
			if err != nil {
				return fmt.Errorf("%s stat: %w", task.OutputType, err)
			}
			outputs[i] = Output{
				Type:        task.OutputType,
				LocalPath:   outputPath,
				FileSize:    info.Size(),
				ContentType: task.ContentType,
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return outputs, nil
}

func runFFmpeg(ctx context.Context, ffmpegPath string, args []string) error {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}

func (w *Worker) uploadOutputs(ctx context.Context, jobID, attemptID string, outputs []Output) error {
	group, ctx := errgroup.WithContext(ctx)
	for i := range outputs {
		i := i
		group.Go(func() error {
			output := &outputs[i]
			key := fmt.Sprintf("%s/%s/%s/%s", w.config.OutputPrefix, jobID, attemptID, output.Type)
			file, err := os.Open(output.LocalPath)
			if err != nil {
				return fmt.Errorf("open output %s: %w", output.Type, err)
			}
			defer file.Close()

			_, err = w.storage.PutObject(ctx, w.storage.Bucket, key, file, output.FileSize, minio.PutObjectOptions{
				ContentType:  output.ContentType,
				CacheControl: w.config.OutputCacheControl,
			})
			if err != nil {
				return fmt.Errorf("upload output %s: %w", output.Type, err)
			}

			output.ObjectKey = key
			output.CDNURL = w.cdnURL(key)
			return nil
		})
	}
	return group.Wait()
}

func (w *Worker) cdnURL(key string) string {
	if w.config.CDNBaseURL == "" {
		return "/" + key
	}
	return w.config.CDNBaseURL + "/" + key
}

func (w *Worker) completeJob(ctx context.Context, jobID, attemptID string, outputs []Output) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if tag, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'completed', completed_attempt_id = $2,
			completed_at = now(), current_attempt_id = NULL
		WHERE id = $1 AND status = 'processing' AND current_attempt_id = $2
	`, jobID, attemptID); err != nil {
		return err
	} else if tag.RowsAffected() != 1 {
		return errStaleAttempt
	}

	for _, output := range outputs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_outputs (job_id, attempt_id, type, cdn_url, file_size)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (job_id, type) DO UPDATE
			SET attempt_id = EXCLUDED.attempt_id,
				cdn_url = EXCLUDED.cdn_url,
				file_size = EXCLUDED.file_size
		`, jobID, attemptID, output.Type, output.CDNURL, output.FileSize); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE job_attempts SET status = 'completed', finished_at = now(), current_phase = 'completed'
		WHERE id = $1 AND status = 'running'
	`, attemptID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (w *Worker) handleFailure(ctx context.Context, jobID, attemptID string, cause error) {
	slog.Error("job failed", "worker_id", w.id, "job_id", jobID, "attempt_id", attemptID, "error", cause)
	tx, err := w.db.Begin(ctx)
	if err != nil {
		slog.Error("begin failure transition", "job_id", jobID, "error", err)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var retryCount, maxRetries, priority int
	if err := tx.QueryRow(ctx, `
		SELECT status::text, retry_count, max_retries, priority
		FROM jobs WHERE id = $1 AND current_attempt_id = $2 FOR UPDATE
	`, jobID, attemptID).Scan(&status, &retryCount, &maxRetries, &priority); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return
		}
		slog.Error("read failed job", "job_id", jobID, "attempt_id", attemptID, "error", err)
		return
	}
	if status != statusProcessing {
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE job_attempts SET status = 'failed', finished_at = now(), failure_category = 'processing_failure', failure_detail = left($2, 500)
		WHERE id = $1 AND status = 'running'
	`, attemptID, cause.Error()); err != nil {
		slog.Error("mark attempt failed", "attempt_id", attemptID, "error", err)
		return
	}
	if retryCount >= maxRetries {
		_, err = tx.Exec(ctx, `UPDATE jobs SET status = 'dead', current_attempt_id = NULL WHERE id = $1`, jobID)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE jobs SET retry_count = retry_count + 1, status = 'queued', current_attempt_id = NULL, queue_entered_at = now()
			WHERE id = $1
		`, jobID)
	}
	if err != nil {
		slog.Error("transition failed job", "job_id", jobID, "error", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		slog.Error("commit failed job", "job_id", jobID, "error", err)
		return
	}
	if retryCount < maxRetries {
		if err := queue.Enqueue(ctx, w.redis, jobID, priority); err != nil {
			slog.Error("enqueue retry", "job_id", jobID, "error", err)
		}
	}
}

type failureAction string

const (
	failureActionRequeue  failureAction = "requeue"
	failureActionMarkDead failureAction = "mark_dead"
)

func failureActionFor(job Job) failureAction {
	if job.RetryCount >= job.MaxRetries {
		return failureActionMarkDead
	}
	return failureActionRequeue
}

func (w *Worker) fetchJob(ctx context.Context, jobID string) (Job, error) {
	var job Job
	err := w.db.QueryRow(ctx, `
		SELECT id::text, status::text, retry_count, max_retries, priority, input_file
		FROM jobs
		WHERE id = $1
	`, jobID).Scan(&job.ID, &job.Status, &job.RetryCount, &job.MaxRetries, &job.Priority, &job.InputFile)
	return job, err
}

func (w *Worker) markDead(ctx context.Context, jobID string) error {
	_, err := w.db.Exec(ctx, `
		UPDATE jobs
		SET status = 'dead'
		WHERE id = $1
	`, jobID)
	return err
}

func (w *Worker) requeueJob(ctx context.Context, job Job) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET retry_count = retry_count + 1,
			status = 'queued'
		WHERE id = $1
	`, job.ID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return queue.Enqueue(ctx, w.redis, job.ID, job.Priority)
}
