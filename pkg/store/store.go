package store

import "context"

// ScanRecord is the normalized, version-agnostic result ready for persistence.
type ScanRecord struct {
	IP        string
	Port      uint32
	Service   string
	Timestamp int64
	Response  string
}

// Store abstracts the persistence layer. Swap SQLite for any backend by
// implementing this interface — no changes needed in the consumer.
type Store interface {
	// Upsert atomically inserts or updates a record, keeping only the newest
	// timestamp for each (ip, port, service) key.
	Upsert(ctx context.Context, record ScanRecord) error
	Close() error
}
