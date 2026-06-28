package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kisna/transcodex/worker/internal/worker"
)

func main() {
	config := worker.LoadConfig()

	processCtx, cancelProcessing := context.WithCancel(context.Background())
	defer cancelProcessing()

	app, err := worker.New(processCtx, config)
	if err != nil {
		log.Fatalf("start worker: %v", err)
	}
	defer app.Close()

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
		log.Printf("received %s, stopping poll loop and draining current job", sig)
		stopPolling()
	}

	drainTimer := time.NewTimer(config.ShutdownGracePeriod)
	defer drainTimer.Stop()

	select {
	case <-done:
		log.Printf("worker drained cleanly")
	case <-drainTimer.C:
		log.Printf("drain timeout reached, canceling current job")
		cancelProcessing()
		<-done
	}
}
