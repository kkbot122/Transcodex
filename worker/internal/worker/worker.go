package worker

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kisna/transcodex/pkg/postgres"
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
	log.Printf("worker %s started", w.id)

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
			log.Printf("poll queue: %v", err)
			sleepOrDone(stopPollingCtx, w.config.PollBackoff)
			continue
		}

		if err := w.processJob(processCtx, *msg); err != nil {
			log.Printf("process job %s: %v", msg.JobID, err)
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
		log.Printf("heartbeat: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := w.heartbeat(ctx); err != nil {
				log.Printf("heartbeat: %v", err)
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
	items, err := w.redis.ZPopMax(ctx, queueName, 1).Result()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, redis.Nil
	}

	member, ok := items[0].Member.(string)
	if !ok {
		return nil, errors.New("queue member is not a string")
	}

	var msg QueueMessage
	if err := json.Unmarshal([]byte(member), &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func sleepOrDone(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
