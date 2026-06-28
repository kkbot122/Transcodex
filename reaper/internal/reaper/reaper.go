package reaper

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/kisna/transcodex/pkg/postgres"
	redisclient "github.com/kisna/transcodex/pkg/redis"
	"github.com/redis/go-redis/v9"
)

type Reaper struct {
	id     string
	config Config
	db     *pgxpool.Pool
	redis  *redis.Client
}

func New(ctx context.Context, config Config) (*Reaper, error) {
	if config.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if config.RedisURL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}

	db, err := postgres.NewPool(ctx, config.DatabaseURL)
	if err != nil {
		return nil, err
	}

	redisConn, err := redisclient.NewClient(ctx, config.RedisURL)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Reaper{
		id:     uuid.NewString(),
		config: config,
		db:     db,
		redis:  redisConn,
	}, nil
}

func (r *Reaper) Close() {
	if r.redis != nil {
		_ = r.redis.Close()
	}
	if r.db != nil {
		r.db.Close()
	}
}

func (r *Reaper) Run(ctx context.Context) {
	slog.Info("reaper started", "reaper_id", r.id)
	for {
		if ctx.Err() != nil {
			slog.Info("reaper stopped", "reaper_id", r.id)
			return
		}

		r.sweepWithLeadership(ctx)

		timer := time.NewTimer(r.config.SweepInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			slog.Info("reaper stopped", "reaper_id", r.id)
			return
		case <-timer.C:
		}
	}
}

func (r *Reaper) sweepWithLeadership(ctx context.Context) {
	locked, err := r.acquireLeadership(ctx)
	if err != nil {
		slog.Error("acquire reaper leadership", "reaper_id", r.id, "error", err)
		return
	}
	if !locked {
		slog.Info("another reaper holds leadership, skipping sweep", "reaper_id", r.id)
		return
	}
	stopRefresh := r.refreshLeadershipUntilDone(context.Background())
	defer stopRefresh()
	defer r.releaseLeadership(context.Background())

	if err := r.Sweep(ctx); err != nil {
		slog.Error("sweep failed", "reaper_id", r.id, "error", err)
	}
}

func (r *Reaper) acquireLeadership(ctx context.Context) (bool, error) {
	return r.redis.SetNX(ctx, reaperLockKey, r.id, r.config.LeadershipLockTTL).Result()
}

func (r *Reaper) refreshLeadershipUntilDone(ctx context.Context) func() {
	done := make(chan struct{})
	period := r.config.LeadershipLockTTL / 3
	if period <= 0 {
		period = time.Second
	}

	go func() {
		ticker := time.NewTicker(period)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				if err := r.refreshLeadership(ctx); err != nil {
					slog.Error("refresh reaper leadership", "reaper_id", r.id, "error", err)
				}
			}
		}
	}()

	return func() {
		close(done)
	}
}

func (r *Reaper) refreshLeadership(ctx context.Context) error {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("EXPIRE", KEYS[1], ARGV[2])
end
return 0
`
	return r.redis.Eval(ctx, script, []string{reaperLockKey}, r.id, int(r.config.LeadershipLockTTL.Seconds())).Err()
}

func (r *Reaper) releaseLeadership(ctx context.Context) {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	if err := r.redis.Eval(ctx, script, []string{reaperLockKey}, r.id).Err(); err != nil {
		slog.Error("release reaper leadership", "reaper_id", r.id, "error", err)
	}
}
