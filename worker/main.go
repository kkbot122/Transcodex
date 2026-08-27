package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kisna/transcodex/pkg/metrics"
	"github.com/kisna/transcodex/worker/internal/worker"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	config := worker.LoadConfig()

	processCtx, cancelProcessing := context.WithCancel(context.Background())
	defer cancelProcessing()

	app, err := worker.New(processCtx, config)
	if err != nil {
		slog.Error("start worker", "error", err)
		os.Exit(1)
	}
	defer app.Close()

	metricsServer := &http.Server{Addr: env("WORKER_METRICS_ADDR", ":9090"), Handler: metrics.Handler("worker")}
	go func() {
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("worker metrics server", "error", err)
		}
	}()
	defer func() { _ = metricsServer.Shutdown(context.Background()) }()

	stopPollingCtx, stopPolling := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		app.Run(stopPollingCtx, processCtx)
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	select {
	case <-done:
		return
	case sig := <-signals:
		slog.Info("received shutdown signal", "signal", sig.String())
		stopPolling()
	}

	drainTimer := time.NewTimer(config.ShutdownGracePeriod)
	defer drainTimer.Stop()

	select {
	case <-done:
		slog.Info("worker drained cleanly")
	case <-drainTimer.C:
		slog.Warn("drain timeout reached, canceling current job", "timeout", config.ShutdownGracePeriod)
		cancelProcessing()
		<-done
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
