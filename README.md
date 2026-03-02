# Mini-Scan

Hello!

As you've heard by now, Censys scans the internet at an incredible scale. Processing the results necessitates scaling horizontally across thousands of machines. One key aspect of our architecture is the use of distributed queues to pass data between machines.

---

The `docker-compose.yml` file sets up a toy example of a scanner. It spins up a Google Pub/Sub emulator, creates a topic and subscription, and publishes scan results to the topic. It can be run via `docker compose up`.

Your job is to build the data processing side. It should:

1. Pull scan results from the subscription `scan-sub`.
2. Maintain an up-to-date record of each unique `(ip, port, service)`. This should contain when the service was last scanned and a string containing the service's response.

> **_NOTE_**
> The scanner can publish data in two formats, shown below. In both of the following examples, the service response should be stored as: `"hello world"`.
>
> ```javascript
> {
>   // ...
>   "data_version": 1,
>   "data": {
>     "response_bytes_utf8": "aGVsbG8gd29ybGQ="
>   }
> }
>
> {
>   // ...
>   "data_version": 2,
>   "data": {
>     "response_str": "hello world"
>   }
> }
> ```

Your processing application should be able to be scaled horizontally, but this isn't something you need to actually do. The processing application should use `at-least-once` semantics where ever applicable.

You may write this in any languages you choose, but Go would be preferred.

You may use any data store of your choosing, with `sqlite` being one example. Like our own code, we expect the code structure to make it easy to switch data stores.

Please note that Google Pub/Sub is best effort ordering and we want to keep the latest scan. While the example scanner does not publish scans at a rate where this would be an issue, we expect the application to be able to handle extreme out of orderness. Consider what would happen if the application received a scan that is 24 hours old.

cmd/scanner/main.go should not be modified

---

Please upload the code to a publicly accessible GitHub, GitLab or other public code repository account. This README file should be updated, briefly documenting your solution. Like our own code, we expect testing instructions: whether it’s an automated test framework, or simple manual steps.

To help set expectations, we believe you should aim to take no more than 4 hours on this task.

We understand that you have other responsibilities, so if you think you’ll need more than 5 business days, just let us know when you expect to send a reply.

Please don’t hesitate to ask any follow-up questions for clarification.
---

## Getting Started

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose

That's it.

---

### 1. Clone the repository

```bash
git clone <repo-url>
cd mini-scan-takehome
```

---

### 2. Start the full stack

```bash
docker compose up --build
```

This will:
1. Start the Google Pub/Sub emulator on port `8085`
2. Create the `scan-topic` topic and `scan-sub` subscription
3. Build and run the scanner (publishes 1 random scan per second)
4. Build and run the consumer (processes messages and writes to SQLite)

You should see the consumer log line:
```
consumer starting: project=test-project sub=scan-sub db=/data/scans.db ...
```

---

### 3. Query the database

The SQLite database is inside the consumer container. Query it directly:

```bash
# View latest 10 records (with column headers)
docker exec mini-scan-takehome-consumer-1 sqlite3 -header -column /data/scans.db \
  "SELECT * FROM scans LIMIT 10;"

# Count total records
docker exec mini-scan-takehome-consumer-1 sqlite3 /data/scans.db \
  "SELECT COUNT(*) FROM scans;"

# View latest scan per service type
docker exec mini-scan-takehome-consumer-1 sqlite3 -header -column /data/scans.db \
  "SELECT service, MAX(timestamp), response FROM scans GROUP BY service;"
```

---

### 4. Manual testing

#### Test 1 — Newer message is accepted

Publish a message with a far-future timestamp. It should overwrite whatever is in the DB for that `(ip, port, service)`:

```bash
NEW_MSG=$(echo -n '{"ip":"1.2.3.4","port":80,"service":"HTTP","timestamp":9999999999,"data_version":2,"data":{"response_str":"I AM NEW AND SHOULD REPLACE"}}' | base64) && \
curl -s -X POST "http://localhost:8085/v1/projects/test-project/topics/scan-topic:publish" \
  -H "Content-Type: application/json" \
  -d "{\"messages\":[{\"data\":\"$NEW_MSG\"}]}"
```

Wait a second, then verify:

```bash
docker exec mini-scan-takehome-consumer-1 sqlite3 -header -column /data/scans.db \
  "SELECT * FROM scans WHERE ip='1.2.3.4' AND port=80 AND service='HTTP';"
```

Expected: `timestamp=9999999999`, `response=I AM NEW AND SHOULD REPLACE`

---

#### Test 2 — Stale message is rejected

Publish the same `(ip, port, service)` with an ancient timestamp. The DB should not change:

```bash
OLD_MSG=$(echo -n '{"ip":"1.2.3.4","port":80,"service":"HTTP","timestamp":1000,"data_version":2,"data":{"response_str":"I AM OLD AND SHOULD BE REJECTED"}}' | base64) && \
curl -s -X POST "http://localhost:8085/v1/projects/test-project/topics/scan-topic:publish" \
  -H "Content-Type: application/json" \
  -d "{\"messages\":[{\"data\":\"$OLD_MSG\"}]}"
```

Wait a second, then verify:

```bash
docker exec mini-scan-takehome-consumer-1 sqlite3 -header -column /data/scans.db \
  "SELECT * FROM scans WHERE ip='1.2.3.4' AND port=80 AND service='HTTP';"
```

Expected: still `timestamp=9999999999` — the stale message was silently dropped.

---

#### Test 3 — Idempotency (duplicate delivery)

Send the same message 10 times in parallel. Only 1 row should exist:

```bash
MSG=$(echo -n '{"ip":"10.0.0.1","port":443,"service":"DNS","timestamp":5000000000,"data_version":2,"data":{"response_str":"idempotency test"}}' | base64)
for i in $(seq 1 10); do
  curl -s -X POST "http://localhost:8085/v1/projects/test-project/topics/scan-topic:publish" \
    -H "Content-Type: application/json" \
    -d "{\"messages\":[{\"data\":\"$MSG\"}]}" > /dev/null &
done && wait && echo "done"
```

Verify:

```bash
docker exec mini-scan-takehome-consumer-1 sqlite3 -header -column /data/scans.db \
  "SELECT COUNT(*) as row_count, ip, port, service, timestamp, response FROM scans WHERE ip='10.0.0.1' AND port=443;"
```

Expected: `row_count=1`

---

#### Test 4 — Concurrent out-of-order messages (race condition safety)

Send 50 messages for the same key with timestamps 1–50, all in parallel. Timestamp 50 should always win regardless of processing order:

```bash
for i in $(seq 1 50); do
  MSG=$(echo -n "{\"ip\":\"10.0.0.2\",\"port\":8080,\"service\":\"SSH\",\"timestamp\":$i,\"data_version\":2,\"data\":{\"response_str\":\"response from ts=$i\"}}" | base64)
  curl -s -X POST "http://localhost:8085/v1/projects/test-project/topics/scan-topic:publish" \
    -H "Content-Type: application/json" \
    -d "{\"messages\":[{\"data\":\"$MSG\"}]}" > /dev/null &
done && wait && echo "all 50 sent"
```

Wait a few seconds, then verify:

```bash
docker exec mini-scan-takehome-consumer-1 sqlite3 -header -column /data/scans.db \
  "SELECT * FROM scans WHERE ip='10.0.0.2' AND port=8080 AND service='SSH';"
```

Expected: `timestamp=50`, `response=response from ts=50`

---

### 5. Tear down

```bash
docker compose down -v   # -v removes the database volume too
```

---

## My Solution

### Overview

The consumer pulls scan results from the `scan-sub` Pub/Sub subscription, determines the data format (V1 or V2), normalizes the response into a plain string, and writes it to SQLite.

```
Pub/Sub (scan-sub) → Consumer → Normalize → Upsert → SQLite
```

The goal was to keep the design straightforward while handling the harder parts correctly — out-of-order messages, duplicate delivery, and the possibility of running multiple consumers down the road.

---

### How it works

#### Ack after write (at-least-once)

Pub/Sub redelivers a message until it is acked. The consumer only acks after the DB write succeeds. If it crashes between the write and the ack, the message gets redelivered and written again — the upsert is idempotent, so the result is the same.

#### Handling out-of-order messages

Pub/Sub doesn't guarantee ordering, so a scan from 24 hours ago could arrive after one from 5 seconds ago. A read-then-write approach (SELECT the current timestamp, compare in application code, then UPDATE) would be vulnerable to race conditions — two goroutines could read the same row, both decide their scan is newer, and the slower writer silently overwrites the faster one. Instead, the entire check-and-update is pushed into a single atomic SQL statement:

```sql
INSERT INTO scans (ip, port, service, timestamp, response)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT (ip, port, service)
DO UPDATE SET
    response  = excluded.response,
    timestamp = excluded.timestamp
WHERE excluded.timestamp > scans.timestamp;
```

The `WHERE` clause does the heavy lifting — it only overwrites if the incoming scan is newer. A stale message just gets silently dropped. The database handles the comparison atomically, so even if two goroutines (or two consumer instances) try to write to the same key at the same time, the right one wins without any app-level coordination.

#### Primary key on `(ip, port, service)`

The composite primary key on `(ip, port, service)` reflects the natural identity of a scan record. It enforces uniqueness at the database level and is required for the `ON CONFLICT` clause to work correctly. A surrogate ID isn't needed here.

#### V1 vs V2 data formats

The scanner sends two formats — V1 with base64-encoded bytes and V2 with a plain string. The consumer checks `data_version` and normalizes both to a plain string before writing. The stored value ends up the same either way.

#### Why SQLite

SQLite was chosen because it requires no external setup — no server process, no credentials, and it runs well in Docker. For a single consumer process it is more than sufficient. It does serialize writes behind a single-writer lock, so goroutines processing messages concurrently will queue on the write path rather than writing truly in parallel. At this scale that is a non-issue. If parallel write throughput becomes important (e.g., multiple consumer instances hitting the same database), swapping in Postgres is straightforward since the store sits behind an interface.

#### Store interface

The data store is abstracted behind a Go interface. Swapping SQLite for Postgres or another backend means implementing that one interface — consumer code stays the same.

#### Graceful shutdown

The consumer catches SIGINT/SIGTERM, stops pulling new messages, waits for in-flight processing to finish, and closes the DB connection. This matters in Docker and Kubernetes environments where the orchestrator sends SIGTERM before killing the container.

#### Backpressure

The Go Pub/Sub client prefetches up to 1000 messages into memory by default — more than a lightweight SQLite-backed consumer should be buffering. To control this, the consumer configures `ReceiveSettings`:

```go
// config.go — loads from env vars with defaults
cfg := Config{
    MaxOutstanding:      envOrDefaultInt("PUBSUB_MAX_OUTSTANDING", 100),
    MaxOutstandingBytes: envOrDefaultInt("PUBSUB_MAX_OUTSTANDING_BYTES", 104857600), // 100 MB
    NumGoroutines:       envOrDefaultInt("PUBSUB_NUM_GOROUTINES", 5),
}

// subscriber.go — applies them to the Pub/Sub client
sub.ReceiveSettings = pubsub.ReceiveSettings{
    MaxOutstandingMessages: cfg.MaxOutstanding,
    MaxOutstandingBytes:    cfg.MaxOutstandingBytes,
    NumGoroutines:          cfg.NumGoroutines,
}
```

Once either the outstanding message count or byte limit is reached, the client stops pulling from the server. `MaxOutstandingBytes` (default 100 MB) acts as a memory ceiling — even if individual messages are small enough that `MaxOutstandingMessages` hasn't triggered, the byte limit prevents unbounded memory growth from large payloads. Messages remain safely queued on the Pub/Sub side. As the consumer catches up and acks, pulling resumes automatically.

Both values are configurable via environment variables because the right numbers depend on runtime conditions (DB speed, message size, memory, number of instances). The approach is to start conservative, monitor, and adjust.

#### Configuration

Settings are loaded from environment variables with sensible defaults, using two small stdlib helpers (`envOrDefault`, `envOrDefaultInt`). No reflection, no struct tags, no external libraries. For six flat fields this is the right tradeoff — easy to read, easy to test, and zero dependencies. A library like `kelseyhightower/envconfig` would start paying off at 15+ fields or when you need features like `time.Duration` parsing, required-field validation, or auto-generated usage docs.

#### Scanner unchanged

`cmd/scanner/main.go` is untouched, as required.

---

## Discussion points (Q & A Analysis)

**What if the consumer crashes mid-processing?**
The message is never acked, so Pub/Sub redelivers it. The idempotent upsert means processing it a second time produces the same result.

**What about a stale message arriving late?**
The timestamp guard in the upsert handles it — if the record in the DB is already newer, the old message is effectively a no-op.

**What if two consumers write the same key at the same time?**
Both execute the atomic upsert. The database keeps whichever scan has the higher timestamp. No application-level locking is needed.

**Why not batch writes?**
Batching would improve throughput but adds complexity around partial failures and ack coordination. Per-message upserts keep the system simple and correct. In a production setting, batching upserts in a transaction and acking the whole batch on success would be a reasonable next step.

**What's the ack deadline?**
The default is 10 seconds in the Go client. If a write takes longer than that, Pub/Sub redelivers the message and it may be processed twice. The idempotent upsert handles that safely. The deadline can also be extended via `MaxExtension` for slow writes.

**How do you tune `MaxOutstandingMessages`?**
There's no magic number — it's a knob tuned through observation. Start at something conservative like 100, monitor the backlog, processing latency, and memory usage. If the backlog grows while CPU/memory are underutilized, increase it. If the consumer OOMs or latency spikes, decrease it.

---

## Future improvements

**Production database**
- SQLite serializes writes — fine for a single consumer, bottleneck for multiple
- The `Store` interface makes it straightforward to swap in a production database (Postgres, MySQL, CockroachDB, etc.) that supports concurrent writes
- Most relational databases support the same `INSERT ... ON CONFLICT` / `ON DUPLICATE KEY` pattern with minor syntax differences

**Batch writes**
- Each message currently triggers its own `INSERT ... ON CONFLICT`
- Batching upserts in a single transaction would reduce fsync and lock overhead
- Tradeoff: partial batch failures and coordinating which messages to ack

**Structured logging**
- Replace `log.Printf` with a structured logger (`slog`, `zerolog`)
- Emit metrics: messages/sec, upsert latency, Pub/Sub backlog size, error rates
- Needed for capacity planning and knowing when to scale or retune

**Dead letter queue**
- Malformed messages are currently acked to avoid infinite retries, but this silently drops them
- Two possible approaches:
  - **Option A — Subscription-level DLQ** (`--max-delivery-attempts`): simple config, but applies the same retry count to all errors — no way to distinguish malformed vs. transient failures
  - **Option B — Application-level DLQ** (preferred): consumer publishes to a DLQ topic directly, then acks the original — gives per-error-type routing (e.g., malformed → immediate DLQ, DB errors → retry indefinitely)
- Current approach is a pragmatic middle ground; broken messages are only visible in logs

**Schema migrations**
- Schema is created inline with `CREATE TABLE IF NOT EXISTS`
- For evolving schemas, a migration tool (`goose`, `golang-migrate`) would manage versioned changes without manual intervention

**Backpressure visibility**
- No way to observe when the outstanding message limit kicks in
- A metric here would help distinguish "consumer is slow" from "no messages to process"

**New data versions**
- Parser handles V1 and V2
- A registry pattern (`data_version` → handler function) would make new formats a one-line addition
