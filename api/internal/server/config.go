package server

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddr            = ":8080"
	defaultUploadLimit     = 500 << 20
	defaultWorkerRetries   = 3
	defaultStatsStreamTick = 5 * time.Second
)

type Config struct {
	Addr              string
	DatabaseURL       string
	RedisURL          string
	StorageEndpoint   string
	StorageBucket     string
	StorageAccessKey  string
	StorageSecretKey  string
	CDNBaseURL        string
	UploadSizeLimit   int64
	WorkerMaxRetries  int
	StatsStreamPeriod time.Duration
}

func LoadConfig() Config {
	return Config{
		Addr:              env("API_ADDR", defaultAddr),
		DatabaseURL:       os.Getenv("DATABASE_URL"),
		RedisURL:          os.Getenv("REDIS_URL"),
		StorageEndpoint:   os.Getenv("STORAGE_ENDPOINT"),
		StorageBucket:     os.Getenv("STORAGE_BUCKET"),
		StorageAccessKey:  os.Getenv("STORAGE_ACCESS_KEY"),
		StorageSecretKey:  os.Getenv("STORAGE_SECRET_KEY"),
		CDNBaseURL:        strings.TrimRight(os.Getenv("CDN_BASE_URL"), "/"),
		UploadSizeLimit:   envInt64("UPLOAD_SIZE_LIMIT_BYTES", defaultUploadLimit),
		WorkerMaxRetries:  envInt("WORKER_MAX_RETRIES", defaultWorkerRetries),
		StatsStreamPeriod: defaultStatsStreamTick,
	}
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

func envInt64(key string, fallback int64) int64 {
	value, err := strconv.ParseInt(os.Getenv(key), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}
