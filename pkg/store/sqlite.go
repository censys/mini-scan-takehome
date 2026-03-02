package store

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// SQLiteStore persists scans to SQLite. Single-writer only — swap to
// PostgreSQL, or another Store implementation via the Store interface if you need parallel writes.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) a SQLite DB and sets up the schema.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// SQLite is single-writer, so one connection avoids pointless lock contention.
	db.SetMaxOpenConns(1)

	// WAL mode lets reads proceed during writes (writes are still serialized).
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Inline schema — good enough for one table. In production you'd use a
	// migration tool (golang-migrate, goose) to handle schema evolution.
	// PK on (ip, port, service) gives us the conflict target for upserts.
	createTable := `
	CREATE TABLE IF NOT EXISTS scans (
		ip        TEXT    NOT NULL,
		port      INTEGER NOT NULL,
		service   TEXT    NOT NULL,
		timestamp INTEGER NOT NULL,
		response  TEXT    NOT NULL,
		PRIMARY KEY (ip, port, service)
	);`

	if _, err := db.Exec(createTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Upsert inserts or updates a scan. The WHERE clause ensures only newer
// timestamps win, so stale out-of-order messages are silently dropped.
// Single atomic statement — no app-level locking needed.
func (s *SQLiteStore) Upsert(ctx context.Context, record ScanRecord) error {
	query := `
	INSERT INTO scans (ip, port, service, timestamp, response)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT (ip, port, service)
	DO UPDATE SET
		response  = excluded.response,
		timestamp = excluded.timestamp
	WHERE excluded.timestamp > scans.timestamp;`

	_, err := s.db.ExecContext(ctx, query,
		record.IP,
		record.Port,
		record.Service,
		record.Timestamp,
		record.Response,
	)
	if err != nil {
		return fmt.Errorf("upsert scan: %w", err)
	}
	return nil
}

// Close shuts down the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Compile-time interface check.
var _ Store = (*SQLiteStore)(nil)
