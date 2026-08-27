package reaper

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/kisna/transcodex/pkg/queue"
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
			if err := r.requeueJob(ctx, *worker.CurrentJob, "", "worker_death"); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *Reaper) recoverOrphanedJobs(ctx context.Context) error {
	rows, err := r.db.Query(ctx, `
		SELECT a.job_id::text, a.id::text
		FROM job_attempts a
		WHERE a.status = 'running'
			AND a.lease_expires_at < now() - ($1::double precision * interval '1 second')
	`, r.config.LeaseExpiryGrace.Seconds())
	if err != nil {
		return err
	}
	defer rows.Close()

	type expiredAttempt struct{ jobID, attemptID string }
	attempts := []expiredAttempt{}
	for rows.Next() {
		var attempt expiredAttempt
		if err := rows.Scan(&attempt.jobID, &attempt.attemptID); err != nil {
			return err
		}
		attempts = append(attempts, attempt)
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, attempt := range attempts {
		if err := r.requeueJob(ctx, attempt.jobID, attempt.attemptID, "lease_expired"); err != nil {
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

		if err := r.enqueueJob(ctx, jobID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reaper) queueContainsJob(ctx context.Context, jobID string) (bool, error) {
	return r.redis.SIsMember(ctx, queue.QueuedJobsSet, jobID).Result()
}

func (r *Reaper) requeueJob(ctx context.Context, jobID, attemptID, reason string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var status string
	var currentAttempt *string
	var retryCount, maxRetries, priority int
	if err := tx.QueryRow(ctx, `
		SELECT status::text, current_attempt_id::text, retry_count, max_retries, priority
		FROM jobs WHERE id = $1 FOR UPDATE
	`, jobID).Scan(&status, &currentAttempt, &retryCount, &maxRetries, &priority); err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	if status != "processing" || currentAttempt == nil || (attemptID != "" && *currentAttempt != attemptID) {
		return nil
	}
	if currentAttempt != nil {
		if _, err := tx.Exec(ctx, `UPDATE job_attempts SET status = 'expired', finished_at = now(), failure_category = $2 WHERE id = $1 AND status = 'running'`, *currentAttempt, reason); err != nil {
			return err
		}
	}
	if retryCount >= maxRetries {
		if _, err := tx.Exec(ctx, `UPDATE jobs SET status = 'dead', current_attempt_id = NULL WHERE id = $1`, jobID); err != nil {
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	}
	if _, err := tx.Exec(ctx, `
		UPDATE jobs
		SET retry_count = retry_count + 1,
			status = 'queued', current_attempt_id = NULL, queue_entered_at = now()
		WHERE id = $1
	`, jobID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if err := queue.Enqueue(ctx, r.redis, jobID, priority); err != nil {
		return err
	}
	slog.Info("job requeued", "reaper_id", r.id, "job_id", jobID, "reason", reason, "retry_count", retryCount+1, "priority", priority)
	return nil
}

func (r *Reaper) enqueueJob(ctx context.Context, jobID string) error {
	job, err := r.fetchJob(ctx, jobID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil
		}
		return err
	}
	return queue.Enqueue(ctx, r.redis, job.ID, job.Priority)
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
