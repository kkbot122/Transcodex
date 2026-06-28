package reaper

import (
	"context"
	"log"
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
	log.Printf("reaper %s started", r.id)
	r.sweepWithLeadership(ctx)

	ticker := time.NewTicker(r.config.SweepInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Printf("reaper %s stopped", r.id)
			return
		case <-ticker.C:
			r.sweepWithLeadership(ctx)
		}
	}
}

func (r *Reaper) sweepWithLeadership(ctx context.Context) {
	locked, err := r.acquireLeadership(ctx)
	if err != nil {
		log.Printf("acquire reaper leadership: %v", err)
		return
	}
	if !locked {
		log.Printf("another reaper holds leadership, skipping sweep")
		return
	}
	defer r.releaseLeadership(context.Background())

	if err := r.Sweep(ctx); err != nil {
		log.Printf("sweep failed: %v", err)
	}
}

func (r *Reaper) acquireLeadership(ctx context.Context) (bool, error) {
	return r.redis.SetNX(ctx, reaperLockKey, r.id, r.config.LeadershipLockTTL).Result()
}

func (r *Reaper) releaseLeadership(ctx context.Context) {
	const script = `
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`
	if err := r.redis.Eval(ctx, script, []string{reaperLockKey}, r.id).Err(); err != nil {
		log.Printf("release reaper leadership: %v", err)
	}
}
