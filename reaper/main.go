package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

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

	app.Run(ctx)
}
