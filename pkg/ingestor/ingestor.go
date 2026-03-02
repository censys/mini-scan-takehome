package ingestor

import (
	"context"
	"errors"
	"log"

	"github.com/censys/scan-takehome/pkg/parser"
	"github.com/censys/scan-takehome/pkg/store"
)

// Ingestor processes scan messages through the Parse -> Upsert pipeline.
// It holds a Store dependency and exposes ProcessMessage as a method,
// making it easy to test without a live Pub/Sub connection.
type Ingestor struct {
	store store.Store
}

// New creates an Ingestor backed by the given Store.
func New(s store.Store) *Ingestor {
	return &Ingestor{store: s}
}

// ProcessMessage runs the pipeline for a single message: Parse -> Upsert -> Ack.
//
// Accepts raw data bytes and explicit ack/nack callbacks so it can be
// exercised in unit tests without a live Pub/Sub connection.
//
// Ack/Nack policy:
//   - Malformed (bad JSON, missing fields): ACK - permanent failure, retrying won't help.
//   - Unknown version (e.g. V3): NACK - valid data, unsupported yet; redeliver or dead-letter.
//   - DB error: NACK - transient; safe to retry due to idempotent upsert.
func (ig *Ingestor) ProcessMessage(ctx context.Context, data []byte, ack func(), nack func()) {
	record, err := parser.Parse(data)
	if err != nil {
		if errors.Is(err, parser.ErrUnknownVersion) {
			log.Printf("WARN: %v | payload: %s", err, string(data))
			nack()
			return
		}
		log.Printf("ERROR: dropping malformed message: %v", err)
		ack()
		return
	}

	if err := ig.store.Upsert(ctx, record); err != nil {
		log.Printf("ERROR: db upsert failed, nacking for retry: %v", err)
		nack()
		return
	}

	ack()
}
