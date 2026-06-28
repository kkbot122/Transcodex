package server

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAddr              = ":8080"
	defaultUploadLimit       = 500 << 20
	defaultWorkerRetries     = 3
	defaultStatsStreamTick   = 5 * time.Second
	defaultReadHeaderTimeout = 10 * time.Second
	defaultShutdownTimeout   = 10 * time.Second
	defaultUploadPrefix      = "raw"
)

type Config struct {
	Addr               string
	DatabaseURL        string
	RedisURL           string
	StorageEndpoint    string
	StorageBucket      string
	StorageAccessKey   string
	StorageSecretKey   string
	CDNBaseURL         string
	UploadPrefix       string
	UploadSizeLimit    int64
	WorkerMaxRetries   int
	StatsStreamPeriod  time.Duration
	ReadHeaderTimeout  time.Duration
	ShutdownTimeout    time.Duration
	CORSAllowedOrigins []string
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
		UploadPrefix:      strings.Trim(env("UPLOAD_PREFIX", defaultUploadPrefix), "/"),
		UploadSizeLimit:   envInt64("UPLOAD_SIZE_LIMIT_BYTES", defaultUploadLimit),
		WorkerMaxRetries:  envInt("WORKER_MAX_RETRIES", defaultWorkerRetries),
		StatsStreamPeriod: envSeconds("STATS_STREAM_INTERVAL_SECONDS", defaultStatsStreamTick),
		ReadHeaderTimeout: envSeconds("API_READ_HEADER_TIMEOUT_SECONDS", defaultReadHeaderTimeout),
		ShutdownTimeout:   envSeconds("API_SHUTDOWN_TIMEOUT_SECONDS", defaultShutdownTimeout),
		CORSAllowedOrigins: envList("CORS_ALLOWED_ORIGINS", []string{
			"http://localhost:3000",
			"http://127.0.0.1:3000",
			"http://localhost:3001",
			"http://127.0.0.1:3001",
			"http://localhost:5173",
			"http://127.0.0.1:5173",
		}),
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

func envSeconds(key string, fallback time.Duration) time.Duration {
	value, err := strconv.Atoi(os.Getenv(key))
	if err != nil || value <= 0 {
		return fallback
	}
	return time.Duration(value) * time.Second
}

func envList(key string, fallback []string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value != "" {
			values = append(values, value)
		}
	}
	if len(values) == 0 {
		return fallback
	}
	return values
}
