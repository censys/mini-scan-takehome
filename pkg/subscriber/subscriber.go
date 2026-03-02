package subscriber

import (
	"context"
	"fmt"
	"log"

	"cloud.google.com/go/pubsub"
)

// Config groups the tuneable knobs for the Pub/Sub subscriber.
type Config struct {
	ProjectID           string
	SubscriptionID      string
	MaxOutstanding      int
	MaxOutstandingBytes int
	NumGoroutines       int
}

// Subscriber wraps a Pub/Sub client and subscription, decoupling the
// pull-and-dispatch loop from message processing logic.
type Subscriber struct {
	client *pubsub.Client
	sub    *pubsub.Subscription
	cfg    Config
}

// New creates a Pub/Sub client, looks up the subscription, and applies
// receive settings. The caller owns the context lifetime used for dialling.
func New(ctx context.Context, cfg Config) (*Subscriber, error) {
	client, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("create pubsub client: %w", err)
	}

	sub := client.Subscription(cfg.SubscriptionID)

	// Backpressure: stop prefetching once either the message count or
	// byte limit is reached. Prevents unbounded memory growth when DB
	// writes are slower than the pull rate.
	sub.ReceiveSettings = pubsub.ReceiveSettings{
		MaxOutstandingMessages: cfg.MaxOutstanding,
		MaxOutstandingBytes:    cfg.MaxOutstandingBytes,
		NumGoroutines:          cfg.NumGoroutines,
	}

	return &Subscriber{client: client, sub: sub, cfg: cfg}, nil
}

// Listen blocks, pulling messages and dispatching each one to handler.
// It returns when ctx is cancelled (graceful shutdown) or on an
// unexpected Pub/Sub error.
func (s *Subscriber) Listen(ctx context.Context, handler func(ctx context.Context, data []byte, ack func(), nack func())) error {
	log.Printf("subscriber listening: project=%s sub=%s max_outstanding=%d max_bytes=%d goroutines=%d",
		s.cfg.ProjectID, s.cfg.SubscriptionID,
		s.cfg.MaxOutstanding, s.cfg.MaxOutstandingBytes, s.cfg.NumGoroutines)

	return s.sub.Receive(ctx, func(ctx context.Context, msg *pubsub.Message) {
		handler(ctx, msg.Data, msg.Ack, msg.Nack)
	})
}

// Close tears down the underlying Pub/Sub client.
func (s *Subscriber) Close() error {
	return s.client.Close()
}
