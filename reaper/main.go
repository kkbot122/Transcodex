package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kisna/transcodex/reaper/internal/reaper"
)

func main() {
	config := reaper.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := reaper.New(ctx, config)
	if err != nil {
		log.Fatalf("start reaper: %v", err)
	}
	defer app.Close()

	app.Run(ctx)
}
