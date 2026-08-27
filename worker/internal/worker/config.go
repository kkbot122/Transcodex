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
	defaultLeaseTTL          = 5 * time.Minute
	defaultShutdownGrace     = 30 * time.Second
	defaultWorkerRetries     = 3
	defaultTempDir           = ""
	defaultOutputPrefix      = "outputs"
	defaultOutputCache       = "public, max-age=86400, immutable"
	defaultFFmpegPath        = "ffmpeg"
	defaultProcessingMode    = processingModeParallel
)

type Config struct {
	DatabaseURL         string
	RedisURL            string
	StorageEndpoint     string
	StorageBucket       string
	StorageAccessKey    string
	StorageSecretKey    string
	CDNBaseURL          string
	TempDir             string
	OutputPrefix        string
	OutputCacheControl  string
	FFmpegPath          string
	ProcessingMode      processingMode
	HeartbeatInterval   time.Duration
	LeaseTTL            time.Duration
	PollBackoff         time.Duration
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
		TempDir:             env("WORKER_TEMP_DIR", defaultTempDir),
		OutputPrefix:        strings.Trim(env("OUTPUT_PREFIX", defaultOutputPrefix), "/"),
		OutputCacheControl:  env("OUTPUT_CACHE_CONTROL", defaultOutputCache),
		FFmpegPath:          env("FFMPEG_PATH", defaultFFmpegPath),
		ProcessingMode:      loadProcessingMode(),
		HeartbeatInterval:   envSeconds("WORKER_HEARTBEAT_INTERVAL", defaultHeartbeatInterval),
		LeaseTTL:            envSeconds("WORKER_LEASE_TTL_SECONDS", defaultLeaseTTL),
		PollBackoff:         envSeconds("WORKER_POLL_BACKOFF_SECONDS", defaultPollBackoff),
		ShutdownGracePeriod: envSeconds("WORKER_SHUTDOWN_GRACE_SECONDS", defaultShutdownGrace),
		WorkerMaxRetries:    envInt("WORKER_MAX_RETRIES", defaultWorkerRetries),
	}
}

func loadProcessingMode() processingMode {
	mode := processingMode(env("WORKER_PROCESSING_MODE", string(defaultProcessingMode)))
	if mode != processingModeParallel && mode != processingModeSequential {
		return defaultProcessingMode
	}
	return mode
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
