package store_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/censys/scan-takehome/pkg/store"
	_ "modernc.org/sqlite"
)

// newTestStore creates a SQLiteStore backed by a temp file and returns it
// along with a direct *sql.DB handle for assertions. Both are cleaned up
// when the test finishes.
func newTestStore(t *testing.T) (*store.SQLiteStore, *sql.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	// Open a second connection for read-side assertions.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open assertion db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return s, db
}

// readRow returns the stored record for the given key, or fails the test.
func readRow(t *testing.T, db *sql.DB, ip string, port uint32, service string) store.ScanRecord {
	t.Helper()
	var rec store.ScanRecord
	err := db.QueryRow(
		`SELECT ip, port, service, timestamp, response FROM scans WHERE ip = ? AND port = ? AND service = ?`,
		ip, port, service,
	).Scan(&rec.IP, &rec.Port, &rec.Service, &rec.Timestamp, &rec.Response)
	if err != nil {
		t.Fatalf("readRow(%s, %d, %s): %v", ip, port, service, err)
	}
	return rec
}

// countRows returns the total number of rows in the scans table.
func countRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM scans`).Scan(&n); err != nil {
		t.Fatalf("countRows: %v", err)
	}
	return n
}

func TestNewSQLiteStore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dbPath  string
		wantErr bool
	}{
		{
			name:   "creates DB at valid path",
			dbPath: filepath.Join(t.TempDir(), "new.db"),
		},
		{
			name:    "fails on invalid path",
			dbPath:  "/nonexistent/dir/test.db",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, err := store.NewSQLiteStore(tt.dbPath)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer s.Close()

			if _, statErr := os.Stat(tt.dbPath); os.IsNotExist(statErr) {
				t.Fatal("expected db file to be created")
			}
		})
	}
}

func TestUpsert(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		records      []store.ScanRecord
		wantRowCount int
		wantRecord   store.ScanRecord // expected state of first key after all upserts
	}{
		{
			name: "insert single record",
			records: []store.ScanRecord{
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "OK"},
			},
			wantRowCount: 1,
			wantRecord:   store.ScanRecord{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "OK"},
		},
		{
			name: "newer timestamp wins",
			records: []store.ScanRecord{
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "old"},
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 2000, Response: "new"},
			},
			wantRowCount: 1,
			wantRecord:   store.ScanRecord{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 2000, Response: "new"},
		},
		{
			name: "older timestamp dropped",
			records: []store.ScanRecord{
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 2000, Response: "new"},
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "old"},
			},
			wantRowCount: 1,
			wantRecord:   store.ScanRecord{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 2000, Response: "new"},
		},
		{
			name: "same timestamp no change",
			records: []store.ScanRecord{
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "first"},
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "second"},
			},
			wantRowCount: 1,
			wantRecord:   store.ScanRecord{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "first"},
		},
		{
			name: "different keys coexist",
			records: []store.ScanRecord{
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1, Response: "a"},
				{IP: "10.0.0.1", Port: 443, Service: "HTTPS", Timestamp: 1, Response: "b"},
				{IP: "10.0.0.2", Port: 80, Service: "HTTP", Timestamp: 1, Response: "c"},
			},
			wantRowCount: 3,
			wantRecord:   store.ScanRecord{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1, Response: "a"},
		},
		{
			name: "idempotent insert",
			records: []store.ScanRecord{
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "OK"},
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "OK"},
				{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "OK"},
			},
			wantRowCount: 1,
			wantRecord:   store.ScanRecord{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "OK"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s, db := newTestStore(t)
			ctx := context.Background()

			for i, r := range tt.records {
				if err := s.Upsert(ctx, r); err != nil {
					t.Fatalf("Upsert #%d: %v", i, err)
				}
			}

			if n := countRows(t, db); n != tt.wantRowCount {
				t.Errorf("row count = %d, want %d", n, tt.wantRowCount)
			}

			got := readRow(t, db, tt.wantRecord.IP, tt.wantRecord.Port, tt.wantRecord.Service)
			if got != tt.wantRecord {
				t.Errorf("got %+v, want %+v", got, tt.wantRecord)
			}
		})
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "close.db")

	s, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Operations after close should fail.
	err = s.Upsert(context.Background(), store.ScanRecord{
		IP: "1.2.3.4", Port: 80, Service: "HTTP", Timestamp: 1, Response: "x",
	})
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}

// TestUpsert_ConcurrentWrites verifies that concurrent upserts complete
// without errors and that the highest timestamp wins. Run with -race.
func TestUpsert_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	s, db := newTestStore(t)
	ctx := context.Background()

	const numWorkers = 10

	var wg sync.WaitGroup
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			rec := store.ScanRecord{
				IP: "10.0.0.1", Port: 80, Service: "HTTP",
				Timestamp: int64(workerID),
				Response:  fmt.Sprintf("w%d", workerID),
			}
			if err := s.Upsert(ctx, rec); err != nil {
				t.Errorf("worker %d: %v", workerID, err)
			}
		}(w)
	}
	wg.Wait()

	// The shared key must hold the highest timestamp written.
	got := readRow(t, db, "10.0.0.1", 80, "HTTP")
	if got.Timestamp != numWorkers-1 {
		t.Errorf("timestamp = %d, want %d (highest wins)", got.Timestamp, numWorkers-1)
	}
	if n := countRows(t, db); n != 1 {
		t.Errorf("row count = %d, want 1", n)
	}
}
