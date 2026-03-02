package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/censys/scan-takehome/pkg/ingestor"
	"github.com/censys/scan-takehome/pkg/store"
	"github.com/censys/scan-takehome/pkg/subscriber"
)

func main() {
	cfg := LoadConfig()

	scanStore, err := store.NewSQLiteStore(cfg.DBPath)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer scanStore.Close()

	ig := ingestor.New(scanStore)

	// Cancel on SIGINT/SIGTERM; causes sub.Receive to drain in-flight messages and return.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("received signal %v, shutting down gracefully...", sig)
		cancel()
	}()

	sub, err := subscriber.New(ctx, subscriber.Config{
		ProjectID:           cfg.ProjectID,
		SubscriptionID:      cfg.SubscriptionID,
		MaxOutstanding:      cfg.MaxOutstanding,
		MaxOutstandingBytes: cfg.MaxOutstandingBytes,
		NumGoroutines:       cfg.NumGoroutines,
	})
	if err != nil {
		log.Fatalf("failed to create subscriber: %v", err)
	}
	defer sub.Close()

	log.Printf("consumer starting: project=%s sub=%s db=%s",
		cfg.ProjectID, cfg.SubscriptionID, cfg.DBPath)

	// Blocks until ctx is cancelled; dispatches each message concurrently.
	if err := sub.Listen(ctx, ig.ProcessMessage); err != nil && ctx.Err() == nil {
		log.Fatalf("receive error: %v", err)
	}

	log.Println("consumer shut down cleanly")
}

