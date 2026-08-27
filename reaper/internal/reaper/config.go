package reaper

import (
	"os"
	"strconv"
	"time"
)

const (
	defaultSweepInterval       = 30 * time.Second
	defaultLeadershipLockTTL   = 60 * time.Second
	defaultDeadWorkerThreshold = 30 * time.Second
	defaultLeaseExpiryGrace    = 0 * time.Second
	defaultStaleQueueThreshold = 30 * time.Second
)

type Config struct {
	DatabaseURL         string
	RedisURL            string
	SweepInterval       time.Duration
	LeadershipLockTTL   time.Duration
	DeadWorkerThreshold time.Duration
	LeaseExpiryGrace    time.Duration
	StaleQueueThreshold time.Duration
}

func LoadConfig() Config {
	sweepInterval := envSeconds("REAPER_SWEEP_INTERVAL", defaultSweepInterval)
	leadershipLockTTL := envSeconds("REAPER_LEADERSHIP_LOCK_TTL_SECONDS", defaultLeadershipLockTTL)
	if leadershipLockTTL <= sweepInterval {
		leadershipLockTTL = sweepInterval * 2
	}

	return Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		SweepInterval:       sweepInterval,
		LeadershipLockTTL:   leadershipLockTTL,
		DeadWorkerThreshold: envSeconds("REAPER_DEAD_WORKER_THRESHOLD_SECONDS", defaultDeadWorkerThreshold),
		LeaseExpiryGrace:    envSeconds("REAPER_LEASE_EXPIRY_GRACE_SECONDS", defaultLeaseExpiryGrace),
		StaleQueueThreshold: envSeconds("REAPER_STALE_QUEUE_THRESHOLD_SECONDS", defaultStaleQueueThreshold),
	}
}

func envSeconds(key string, fallback time.Duration) time.Duration {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}
