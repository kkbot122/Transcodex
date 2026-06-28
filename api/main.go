package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/kisna/transcodex/api/internal/server"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	config := server.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := server.New(ctx, config)
	if err != nil {
		slog.Error("start api", "error", err)
		os.Exit(1)
	}
	defer app.Close()

	httpServer := &http.Server{
		Addr:              config.Addr,
		Handler:           app.Router(),
		ReadHeaderTimeout: config.ReadHeaderTimeout,
	}

	go func() {
		slog.Info("api listening", "addr", config.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("listen", "addr", config.Addr, "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), config.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("shutdown api", "error", err)
	}
}
