# Tickets

Each ticket is a self-contained unit of work. Complete in order.

---

## Ticket 1: Monolithic consumer

**Depends on:** —

Implement a working consumer in `cmd/consumer/main.go` with all logic inline:

- Create Pub/Sub client + subscription, configure `ReceiveSettings` for backpressure.
- `ProcessMessage(ctx, *pubsub.Message)`: unmarshal JSON → switch on `DataVersion` (V1: base64-decode, V2: direct string) → upsert into SQLite with `ON CONFLICT ... WHERE excluded.timestamp > scans.timestamp` → ack/nack.
- Ack malformed (permanent), nack unknown versions (redeliver), nack DB errors (transient).
- SIGINT/SIGTERM graceful shutdown.

**AC:** Consumer pulls from `scan-sub`, writes to SQLite, handles V1/V2, doesn't overwrite newer data, shuts down cleanly, `go build ./cmd/consumer` passes.

---

## Ticket 2: Extract `Store` interface + SQLite impl

**Depends on:** Ticket 1

- `pkg/store/store.go`: `ScanRecord` struct + `Store` interface (`Upsert`, `Close`).
- `pkg/store/sqlite.go`: `SQLiteStore` with `MaxOpenConns(1)`, WAL mode, `INSERT ... ON CONFLICT` upsert, compile-time interface check.
- Update `main.go` to use `store.NewSQLiteStore(...)`. No SQL or `database/sql` left in `main.go`.

**AC:** Store package owns all persistence. Consumer behavior unchanged.

---

## Ticket 3: Extract `parser` package

**Depends on:** Ticket 2

- `pkg/parser/parser.go`: `Parse(data []byte) (store.ScanRecord, error)`.
- Sentinel errors: `ErrMalformed` (ACK — permanent), `ErrUnknownVersion` (NACK — redeliver).
- V1: base64-decode `response_bytes_utf8`. V2: read `response_str`. Unknown: return `ErrUnknownVersion`.
- Update `ProcessMessage` in `main.go` to call `parser.Parse(msg.Data)`.

**AC:** Parser package owns all format logic. No `encoding/base64` or `encoding/json` in `main.go`. Behavior unchanged.

---

## Ticket 4: Extract `subscriber` package

**Depends on:** Ticket 1

- `pkg/subscriber/subscriber.go`: `Subscriber` struct wrapping `*pubsub.Client` + `*pubsub.Subscription`.
- `New(ctx, Config)` creates client, applies `ReceiveSettings`.
- `Listen(ctx, handler func(ctx, []byte, ack, nack))` calls `sub.Receive`, adapting `*pubsub.Message` to generic handler signature — decouples downstream code from Pub/Sub.
- `Close()` tears down the client.
- Update `main.go` — no more `cloud.google.com/go/pubsub` import.

**AC:** Subscriber package owns all Pub/Sub interaction. Handler signature is transport-agnostic. Behavior unchanged.

---

## Ticket 5: Extract `ingestor` package

**Depends on:** Tickets 2, 3, 4

- `pkg/ingestor/ingestor.go`: `Ingestor` struct holding `store.Store`.
- `ProcessMessage(ctx, []byte, ack, nack)` as a method: parse → upsert → ack. Ack/nack policy documented in godoc.
- `main.go` becomes pure wiring: `ig := ingestor.New(scanStore)`, pass `ig.ProcessMessage` to `sub.Listen`.

**AC:** `main.go` has zero business logic. Ingestor is independently testable. Behavior unchanged.

---

## Ticket 6: Extract `config` file

**Depends on:** Ticket 5

- `cmd/consumer/config.go`: `Config` struct + `LoadConfig()` reading env vars with defaults.
- Helpers: `envOrDefault`, `envOrDefaultInt` (warns on bad int).
- No `os.Getenv` in `main.go`.

**AC:** Config is self-contained. Defaults are explicit. `main.go` just calls `LoadConfig()`.

---

## Ticket 7: Dockerfile + docker-compose

**Depends on:** Ticket 6

- `cmd/consumer/Dockerfile`: multi-stage build → `debian:bookworm-slim`.
- `docker-compose.yml`: add `consumer` service with `PUBSUB_EMULATOR_HOST`, volume for SQLite, `depends_on` scanner + pubsub.

**AC:** `docker compose up` runs the full stack. Consumer processes messages from the emulator.

---

## Ticket 8: Graceful shutdown

**Depends on:** Ticket 5

- Catch SIGINT/SIGTERM via `signal.Notify`, cancel the root context.
- `sub.Receive` drains in-flight messages and returns when ctx is cancelled.
- `defer scanStore.Close()` ensures DB connection is released.
- Log the received signal and clean exit.

**AC:** Consumer finishes processing in-flight messages before exiting. No data loss on SIGTERM. Works correctly in Docker (`docker stop`).

---

## Ticket 9: README documentation

**Depends on:** Ticket 8

Add "My Solution" section covering: architecture overview, at-least-once delivery, atomic out-of-order handling, V1/V2 formats, SQLite rationale, Store interface, graceful shutdown, backpressure config.

**AC:** README explains key design decisions and trade-offs.
