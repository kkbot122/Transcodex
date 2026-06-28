package storage

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	UseSSL    bool
}

type Client struct {
	*minio.Client
	Bucket string
}

func NewClient(ctx context.Context, config Config) (*Client, error) {
	if config.Endpoint == "" {
		return nil, fmt.Errorf("STORAGE_ENDPOINT is required")
	}
	if config.AccessKey == "" {
		return nil, fmt.Errorf("STORAGE_ACCESS_KEY is required")
	}
	if config.SecretKey == "" {
		return nil, fmt.Errorf("STORAGE_SECRET_KEY is required")
	}
	if config.Bucket == "" {
		return nil, fmt.Errorf("STORAGE_BUCKET is required")
	}

	endpoint, useSSL, err := normalizeEndpoint(config.Endpoint, config.UseSSL)
	if err != nil {
		return nil, err
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("create storage client: %w", err)
	}

	exists, err := client.BucketExists(ctx, config.Bucket)
	if err != nil {
		return nil, fmt.Errorf("check storage bucket: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("storage bucket %q does not exist", config.Bucket)
	}

	return &Client{Client: client, Bucket: config.Bucket}, nil
}

func normalizeEndpoint(rawEndpoint string, fallbackSSL bool) (string, bool, error) {
	endpoint := strings.TrimSpace(rawEndpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", false, fmt.Errorf("parse storage endpoint: %w", err)
	}

	if parsed.Scheme == "" {
		return endpoint, fallbackSSL, nil
	}
	if parsed.Host == "" {
		return "", false, fmt.Errorf("storage endpoint is missing host")
	}

	switch parsed.Scheme {
	case "http":
		return parsed.Host, false, nil
	case "https":
		return parsed.Host, true, nil
	default:
		return "", false, fmt.Errorf("unsupported storage endpoint scheme %q", parsed.Scheme)
	}
}
