package worker

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHeartbeatInterval = 10 * time.Second
	defaultPollBackoff       = 2 * time.Second
	defaultLockTTL           = 5 * time.Minute
	defaultShutdownGrace     = 30 * time.Second
	defaultWorkerRetries     = 3
)

type Config struct {
	DatabaseURL         string
	RedisURL            string
	StorageEndpoint     string
	StorageBucket       string
	StorageAccessKey    string
	StorageSecretKey    string
	CDNBaseURL          string
	HeartbeatInterval   time.Duration
	PollBackoff         time.Duration
	LockTTL             time.Duration
	ShutdownGracePeriod time.Duration
	WorkerMaxRetries    int
}

func LoadConfig() Config {
	return Config{
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            os.Getenv("REDIS_URL"),
		StorageEndpoint:     os.Getenv("STORAGE_ENDPOINT"),
		StorageBucket:       os.Getenv("STORAGE_BUCKET"),
		StorageAccessKey:    os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecretKey:    os.Getenv("STORAGE_SECRET_KEY"),
		CDNBaseURL:          strings.TrimRight(os.Getenv("CDN_BASE_URL"), "/"),
		HeartbeatInterval:   envSeconds("WORKER_HEARTBEAT_INTERVAL", defaultHeartbeatInterval),
		PollBackoff:         defaultPollBackoff,
		LockTTL:             defaultLockTTL,
		ShutdownGracePeriod: envSeconds("WORKER_SHUTDOWN_GRACE_SECONDS", defaultShutdownGrace),
		WorkerMaxRetries:    envInt("WORKER_MAX_RETRIES", defaultWorkerRetries),
	}
}

func envInt(key string, fallback int) int {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil {
		return fallback
	}
	return value
}

func envSeconds(key string, fallback time.Duration) time.Duration {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}
