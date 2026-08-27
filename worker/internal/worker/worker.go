package worker

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kisna/transcodex/pkg/postgres"
	"github.com/kisna/transcodex/pkg/queue"
	redisclient "github.com/kisna/transcodex/pkg/redis"
	"github.com/kisna/transcodex/pkg/storage"
	"github.com/redis/go-redis/v9"
)

type Worker struct {
	id      string
	config  Config
	db      *pgxpool.Pool
	redis   *redis.Client
	storage *storage.Client
}

func New(ctx context.Context, config Config) (*Worker, error) {
	db, err := postgres.NewPool(ctx, config.DatabaseURL)
	if err != nil {
		return nil, err
	}

	redisConn, err := redisclient.NewClient(ctx, config.RedisURL)
	if err != nil {
		db.Close()
		return nil, err
	}

	storageClient, err := storage.NewClient(ctx, storage.Config{
		Endpoint:  config.StorageEndpoint,
		AccessKey: config.StorageAccessKey,
		SecretKey: config.StorageSecretKey,
		Bucket:    config.StorageBucket,
	})
	if err != nil {
		_ = redisConn.Close()
		db.Close()
		return nil, err
	}

	worker := &Worker{
		id:      uuid.NewString(),
		config:  config,
		db:      db,
		redis:   redisConn,
		storage: storageClient,
	}

	if err := worker.register(ctx); err != nil {
		worker.Close()
		return nil, err
	}

	return worker, nil
}

func (w *Worker) Close() {
	if w.redis != nil {
		_ = w.redis.Close()
	}
	if w.db != nil {
		w.db.Close()
	}
}

func (w *Worker) Run(stopPollingCtx context.Context, processCtx context.Context) {
	slog.Info("worker started", "worker_id", w.id)

	heartbeatCtx, stopHeartbeat := context.WithCancel(processCtx)
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		w.runHeartbeat(heartbeatCtx)
	}()
	defer func() {
		stopHeartbeat()
		<-heartbeatDone
	}()

	for {
		select {
		case <-stopPollingCtx.Done():
			return
		default:
		}

		msg, err := w.poll(stopPollingCtx)
		if errors.Is(err, redis.Nil) || msg == nil {
			sleepOrDone(stopPollingCtx, w.config.PollBackoff)
			continue
		}
		if err != nil {
			slog.Error("poll queue", "worker_id", w.id, "error", err)
			sleepOrDone(stopPollingCtx, w.config.PollBackoff)
			continue
		}

		if err := w.processJob(processCtx, *msg); err != nil {
			slog.Error("process job", "worker_id", w.id, "job_id", msg.JobID, "error", err)
		}
	}
}

func (w *Worker) register(ctx context.Context) error {
	_, err := w.db.Exec(ctx, `
		INSERT INTO workers (id, status, current_job, last_heartbeat)
		VALUES ($1, 'idle', NULL, now())
		ON CONFLICT (id) DO UPDATE
		SET status = 'idle',
			current_job = NULL,
			last_heartbeat = now()
	`, w.id)
	return err
}

func (w *Worker) runHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(w.config.HeartbeatInterval)
	defer ticker.Stop()

	if err := w.heartbeat(ctx); err != nil {
		slog.Error("heartbeat", "worker_id", w.id, "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.heartbeat(ctx); err != nil {
				slog.Error("heartbeat", "worker_id", w.id, "error", err)
			}
		}
	}
}

func (w *Worker) heartbeat(ctx context.Context) error {
	_, err := w.db.Exec(ctx, `
		UPDATE workers
		SET last_heartbeat = now()
		WHERE id = $1
	`, w.id)
	return err
}

func (w *Worker) poll(ctx context.Context) (*QueueMessage, error) {
	jobID, err := queue.Pop(ctx, w.redis)
	if err != nil {
		return nil, err
	}

	job, err := w.fetchJob(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &QueueMessage{JobID: job.ID, InputFile: job.InputFile, Priority: job.Priority}, nil
}

func sleepOrDone(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
