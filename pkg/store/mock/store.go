// Package mock provides test doubles for the store package.
package mock

import (
	"context"
	"sync"

	"github.com/censys/scan-takehome/pkg/store"
)

// Store is a thread-safe in-memory fake that implements store.Store.
// Set Err to make Upsert return a canned error (simulating DB failures).
type Store struct {
	mu      sync.Mutex
	records []store.ScanRecord
	Err     error
}

// Upsert records the scan or returns the canned error.
func (m *Store) Upsert(_ context.Context, rec store.ScanRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.records = append(m.records, rec)
	return nil
}

// Close is a no-op.
func (m *Store) Close() error { return nil }

// UpsertCount returns how many successful upserts were recorded.
func (m *Store) UpsertCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.records)
}

// LastRecord returns the most recently upserted record.
// Panics if no records exist.
func (m *Store) LastRecord() store.ScanRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.records[len(m.records)-1]
}

// Records returns a copy of all upserted records.
func (m *Store) Records() []store.ScanRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]store.ScanRecord, len(m.records))
	copy(out, m.records)
	return out
}

// Compile-time interface check.
var _ store.Store = (*Store)(nil)
