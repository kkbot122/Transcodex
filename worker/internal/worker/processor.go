package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kisna/transcodex/pkg/queue"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/errgroup"
)

var errJobAlreadyClaimed = errors.New("job is not queued")

func (w *Worker) processJob(ctx context.Context, msg QueueMessage) error {
	locked, err := w.acquireLock(ctx, msg.JobID)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	if !locked {
		slog.Info("job already locked, skipping", "worker_id", w.id, "job_id", msg.JobID)
		return nil
	}
	defer w.releaseLock(context.Background(), msg.JobID)

	claimed, err := w.markProcessing(ctx, msg.JobID)
	if err != nil {
		w.handleFailure(ctx, msg.JobID, err)
		return err
	}
	if !claimed {
		return errJobAlreadyClaimed
	}

	if err := w.setWorkerBusy(ctx, msg.JobID); err != nil {
		w.handleFailure(ctx, msg.JobID, err)
		return err
	}
	defer func() {
		if err := w.setWorkerIdle(context.Background()); err != nil {
			slog.Error("set worker idle", "worker_id", w.id, "job_id", msg.JobID, "error", err)
		}
	}()

	if err := w.runJob(ctx, msg); err != nil {
		w.handleFailure(ctx, msg.JobID, err)
		return err
	}

	return nil
}

func (w *Worker) runJob(ctx context.Context, msg QueueMessage) error {
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
	if err := w.downloadInput(ctx, msg.InputFile, inputPath); err != nil {
		return err
	}

	outputs, err := runFFmpegJobs(ctx, w.config.FFmpegPath, inputPath, workDir)
	if err != nil {
		return err
	}

	if err := w.uploadOutputs(ctx, msg.JobID, outputs); err != nil {
		return err
	}

	if err := w.completeJob(ctx, msg.JobID, outputs); err != nil {
		return err
	}

	slog.Info("job completed", "worker_id", w.id, "job_id", msg.JobID)
	return nil
}

func (w *Worker) acquireLock(ctx context.Context, jobID string) (bool, error) {
	return w.redis.SetNX(ctx, lockKey(jobID), w.id, w.config.LockTTL).Result()
}

func (w *Worker) releaseLock(ctx context.Context, jobID string) {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	if err := w.redis.Eval(ctx, script, []string{lockKey(jobID)}, w.id).Err(); err != nil {
		slog.Error("release job lock", "worker_id", w.id, "job_id", jobID, "error", err)
	}
}

func lockKey(jobID string) string {
	return "lock:job:" + jobID
}

func (w *Worker) markProcessing(ctx context.Context, jobID string) (bool, error) {
	tag, err := w.db.Exec(ctx, `
		UPDATE jobs
		SET status = 'processing'
		WHERE id = $1 AND status = 'queued'
	`, jobID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
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

func runFFmpegJobs(ctx context.Context, ffmpegPath string, inputPath string, workDir string) ([]Output, error) {
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
	errs := make([]error, len(tasks))
	var wg sync.WaitGroup

	for i, task := range tasks {
		i, task := i, task
		wg.Add(1)
		go func() {
			defer wg.Done()
			outputPath := filepath.Join(workDir, task.FileName)
			if err := runFFmpeg(ctx, ffmpegPath, task.Args(inputPath, outputPath)); err != nil {
				errs[i] = fmt.Errorf("%s: %w", task.OutputType, err)
				return
			}
			info, err := os.Stat(outputPath)
			if err != nil {
				errs[i] = fmt.Errorf("%s stat: %w", task.OutputType, err)
				return
			}
			outputs[i] = Output{
				Type:        task.OutputType,
				LocalPath:   outputPath,
				FileSize:    info.Size(),
				ContentType: task.ContentType,
			}
		}()
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return nil, err
		}
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

func (w *Worker) uploadOutputs(ctx context.Context, jobID string, outputs []Output) error {
	group, ctx := errgroup.WithContext(ctx)
	for i := range outputs {
		i := i
		group.Go(func() error {
			output := &outputs[i]
			key := fmt.Sprintf("%s/%s/%s", w.config.OutputPrefix, jobID, output.Type)
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

func (w *Worker) completeJob(ctx context.Context, jobID string, outputs []Output) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	for _, output := range outputs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO job_outputs (job_id, type, cdn_url, file_size)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (job_id, type) DO UPDATE
			SET cdn_url = EXCLUDED.cdn_url,
				file_size = EXCLUDED.file_size
		`, jobID, output.Type, output.CDNURL, output.FileSize); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET status = 'completed'
		WHERE id = $1
	`, jobID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (w *Worker) handleFailure(ctx context.Context, jobID string, cause error) {
	slog.Error("job failed", "worker_id", w.id, "job_id", jobID, "error", cause)

	job, err := w.fetchJob(ctx, jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("job disappeared before failure handling", "worker_id", w.id, "job_id", jobID)
			return
		}
		slog.Error("fetch failed job", "worker_id", w.id, "job_id", jobID, "error", err)
		return
	}

	if failureActionFor(job) == failureActionMarkDead {
		if err := w.markDead(ctx, job.ID); err != nil {
			slog.Error("mark job dead", "worker_id", w.id, "job_id", job.ID, "error", err)
		}
		slog.Error("job permanently failed", "worker_id", w.id, "job_id", job.ID, "retry_count", job.RetryCount, "max_retries", job.MaxRetries)
		return
	}

	if err := w.requeueJob(ctx, job); err != nil {
		slog.Error("requeue job", "worker_id", w.id, "job_id", job.ID, "error", err)
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

	enqueuedAt := time.Now().UTC()
	payload, err := json.Marshal(QueueMessage{
		JobID:      job.ID,
		InputFile:  job.InputFile,
		Priority:   job.Priority,
		EnqueuedAt: enqueuedAt,
	})
	if err != nil {
		return err
	}

	pipe := w.redis.TxPipeline()
	pipe.ZAdd(ctx, queue.Name, redisZ(queue.PriorityScore(job.Priority, enqueuedAt), payload))
	pipe.SAdd(ctx, queue.QueuedJobsSet, job.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func redisZ(score float64, member []byte) redis.Z {
	return redis.Z{Score: score, Member: member}
}
