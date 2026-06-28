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
	defaultOrphanJobThreshold  = 5 * time.Minute
	defaultStaleQueueThreshold = 30 * time.Second
)

type Config struct {
	DatabaseURL         string
	RedisURL            string
	SweepInterval       time.Duration
	LeadershipLockTTL   time.Duration
	DeadWorkerThreshold time.Duration
	OrphanJobThreshold  time.Duration
	StaleQueueThreshold time.Duration
}

func LoadConfig() Config {
	return Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		SweepInterval:       envSeconds("REAPER_SWEEP_INTERVAL", defaultSweepInterval),
		LeadershipLockTTL:   envSeconds("REAPER_LEADERSHIP_LOCK_TTL_SECONDS", defaultLeadershipLockTTL),
		DeadWorkerThreshold: envSeconds("REAPER_DEAD_WORKER_THRESHOLD_SECONDS", defaultDeadWorkerThreshold),
		OrphanJobThreshold:  envSeconds("REAPER_ORPHAN_JOB_THRESHOLD_SECONDS", defaultOrphanJobThreshold),
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
