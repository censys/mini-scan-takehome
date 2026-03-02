# Testing Strategy

## Running Tests

```bash
# all tests
go test ./...

# with race detector (recommended)
go test -race ./...

# single package
go test -race ./pkg/store/...
```

## Overview

Tests are organised per-package using the standard `testing` library—no third-party assertion frameworks. Every test file follows a handful of consistent patterns described below.

## Patterns

### Table-driven tests

All test functions use the table-driven style: a slice of named cases iterated with `t.Run`. Each sub-test calls `t.Parallel()` so cases within the same function run concurrently

### Mock store

`pkg/store/mock` provides a thread-safe in-memory `Store` implementation. It records every upsert and lets callers inject errors via a canned `Err` field, which is enough to cover both happy-path and failure scenarios without touching SQLite.

## Package Breakdown

### `pkg/parser` — message parsing

Pure function tests. Each case feeds a `[]byte` payload into `parser.Parse` and asserts either the returned `ScanRecord` or a sentinel error (`ErrMalformed`, `ErrUnknownVersion`). Covers:

- V1 (base64-encoded) and V2 (plain string) happy paths
- Edge cases: empty responses, max-value fields
- Error paths: bad JSON, unknown/negative/zero versions, wrong types, missing keys

### `pkg/ingestor` — message processing & ack/nack

Uses the mock store and an `ackTracker` to verify that `ProcessMessage`:

- Upserts correctly for valid V1/V2 messages and acks
- Acks on permanent parse failures (malformed JSON) so the message is not redelivered
- Nacks on retryable failures (unknown version, DB error) so the message can be retried

### `pkg/store` — SQLite persistence

Tests the real SQLite implementation (via temp files, no mocks). A second `*sql.DB` connection is opened for read-side assertions independent of the store under test.

Key areas:

| Test | What it verifies |
|---|---|
| `TestNewSQLiteStore` | DB file creation and error on invalid paths |
| `TestUpsert` | Insert, newer-wins, older-dropped, same-timestamp idempotency, multi-key coexistence |
| `TestClose` | Operations after `Close()` return errors |
| `TestUpsert_ConcurrentWrites` | 10 goroutines upsert to the same key; highest timestamp wins, no races (run with `-race`) |

### `cmd/consumer` — configuration

Validates `envOrDefault`, `envOrDefaultInt`, and `LoadConfig` against various environment variable combinations using `t.Setenv`.

## What's Not Tested (and why)

- **`pkg/subscriber`** — thin wrapper around the Google Pub/Sub client. Tested indirectly via Docker Compose integration; unit-testing it would require a Pub/Sub emulator or extensive mocking for little value.
- **`cmd/scanner`** and **`cmd/consumer/main.go`** — wiring-only entry points. Verified by running `docker-compose up` end-to-end.
