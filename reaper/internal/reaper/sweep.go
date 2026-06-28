package reaper

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kisna/transcodex/pkg/queue"
	"github.com/redis/go-redis/v9"
)

func (r *Reaper) Sweep(ctx context.Context) error {
	if err := r.recoverDeadWorkers(ctx); err != nil {
		return fmt.Errorf("recover dead workers: %w", err)
	}
	if err := r.recoverOrphanedJobs(ctx); err != nil {
		return fmt.Errorf("recover orphaned jobs: %w", err)
	}
	if err := r.recoverMissingQueueMessages(ctx); err != nil {
		return fmt.Errorf("recover missing queue messages: %w", err)
	}
	return nil
}

func (r *Reaper) recoverDeadWorkers(ctx context.Context) error {
	rows, err := r.db.Query(ctx, `
		SELECT id::text, current_job::text
		FROM workers
		WHERE status = 'busy'
			AND last_heartbeat < now() - ($1::double precision * interval '1 second')
	`, r.config.DeadWorkerThreshold.Seconds())
	if err != nil {
		return err
	}
	defer rows.Close()

	workers := []DeadWorker{}
	for rows.Next() {
		var worker DeadWorker
		if err := rows.Scan(&worker.ID, &worker.CurrentJob); err != nil {
			return err
		}
		workers = append(workers, worker)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, worker := range workers {
		if _, err := r.db.Exec(ctx, `
			UPDATE workers
			SET status = 'dead',
				current_job = NULL
			WHERE id = $1
		`, worker.ID); err != nil {
			return err
		}

		if worker.CurrentJob != nil {
			if err := r.requeueJob(ctx, *worker.CurrentJob, "worker_death"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reaper) recoverOrphanedJobs(ctx context.Context) error {
	rows, err := r.db.Query(ctx, `
		SELECT id::text
		FROM jobs
		WHERE status = 'processing'
			AND updated_at < now() - ($1::double precision * interval '1 second')
	`, r.config.OrphanJobThreshold.Seconds())
	if err != nil {
		return err
	}
	defer rows.Close()

	jobIDs := []string{}
	for rows.Next() {
		var jobID string
		if err := rows.Scan(&jobID); err != nil {
			return err
		}
		jobIDs = append(jobIDs, jobID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, jobID := range jobIDs {
		if err := r.requeueJob(ctx, jobID, "orphan"); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reaper) recoverMissingQueueMessages(ctx context.Context) error {
	rows, err := r.db.Query(ctx, `
		SELECT id::text
		FROM jobs
		WHERE status = 'queued'
			AND updated_at < now() - ($1::double precision * interval '1 second')
	`, r.config.StaleQueueThreshold.Seconds())
	if err != nil {
		return err
	}
	defer rows.Close()

	jobIDs := []string{}
	for rows.Next() {
		var jobID string
		if err := rows.Scan(&jobID); err != nil {
			return err
		}
		jobIDs = append(jobIDs, jobID)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, jobID := range jobIDs {
		exists, err := r.queueContainsJob(ctx, jobID)
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		if err := r.requeueJob(ctx, jobID, "missing_queue_message"); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reaper) queueContainsJob(ctx context.Context, jobID string) (bool, error) {
	return r.redis.SIsMember(ctx, queue.QueuedJobsSet, jobID).Result()
}

func (r *Reaper) requeueJob(ctx context.Context, jobID string, reason string) error {
	job, err := r.fetchJob(ctx, jobID)
	if err != nil {
		if err == pgx.ErrNoRows {
			slog.Warn("skip requeue missing job", "reaper_id", r.id, "job_id", jobID, "reason", reason)
			return nil
		}
		return err
	}

	if requeueActionFor(job) == requeueActionMarkDead {
		if err := r.markDead(ctx, job.ID); err != nil {
			return err
		}
		slog.Error("job marked dead", "reaper_id", r.id, "job_id", job.ID, "reason", reason, "retry_count", job.RetryCount, "max_retries", job.MaxRetries)
		return nil
	}

	tx, err := r.db.Begin(ctx)
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

	pipe := r.redis.TxPipeline()
	pipe.ZAdd(ctx, queue.Name, redis.Z{
		Score:  queue.PriorityScore(job.Priority, enqueuedAt),
		Member: payload,
	})
	pipe.SAdd(ctx, queue.QueuedJobsSet, job.ID)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}

	slog.Info("job requeued", "reaper_id", r.id, "job_id", job.ID, "reason", reason, "retry_count", job.RetryCount+1, "priority", job.Priority)
	return nil
}

type requeueAction string

const (
	requeueActionRequeue  requeueAction = "requeue"
	requeueActionMarkDead requeueAction = "mark_dead"
)

func requeueActionFor(job Job) requeueAction {
	if job.RetryCount >= job.MaxRetries {
		return requeueActionMarkDead
	}
	return requeueActionRequeue
}

func (r *Reaper) fetchJob(ctx context.Context, jobID string) (Job, error) {
	var job Job
	err := r.db.QueryRow(ctx, `
		SELECT id::text, status::text, retry_count, max_retries, priority, input_file
		FROM jobs
		WHERE id = $1
	`, jobID).Scan(&job.ID, &job.Status, &job.RetryCount, &job.MaxRetries, &job.Priority, &job.InputFile)
	return job, err
}

func (r *Reaper) markDead(ctx context.Context, jobID string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE jobs
		SET status = 'dead'
		WHERE id = $1
	`, jobID)
	return err
}
