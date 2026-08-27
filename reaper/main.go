package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kisna/transcodex/pkg/metrics"
	"github.com/kisna/transcodex/reaper/internal/reaper"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	config := reaper.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := reaper.New(ctx, config)
	if err != nil {
		slog.Error("start reaper", "error", err)
		os.Exit(1)
	}
	defer app.Close()
	metricsServer := &http.Server{Addr: env("REAPER_METRICS_ADDR", ":9090"), Handler: metrics.Handler("reaper")}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("reaper metrics server", "error", err)
		}
	}()
	defer func() { _ = metricsServer.Shutdown(context.Background()) }()

	app.Run(ctx)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
