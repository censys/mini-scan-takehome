package ingestor_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/censys/scan-takehome/pkg/ingestor"
	"github.com/censys/scan-takehome/pkg/scanning"
	"github.com/censys/scan-takehome/pkg/store"
	storemock "github.com/censys/scan-takehome/pkg/store/mock"
)

// --- ack/nack tracker ---

type ackTracker struct {
	acked  bool
	nacked bool
}

func (a *ackTracker) ack()  { a.acked = true }
func (a *ackTracker) nack() { a.nacked = true }

// --- helpers ---

func buildV1Message(ip string, port uint32, svc string, ts int64, body string) []byte {
	msg, _ := json.Marshal(scanning.Scan{
		Ip: ip, Port: port, Service: svc,
		Timestamp: ts, DataVersion: scanning.V1,
		Data: map[string]interface{}{
			"response_bytes_utf8": base64.StdEncoding.EncodeToString([]byte(body)),
		},
	})
	return msg
}

func buildV2Message(ip string, port uint32, svc string, ts int64, body string) []byte {
	msg, _ := json.Marshal(scanning.Scan{
		Ip: ip, Port: port, Service: svc,
		Timestamp: ts, DataVersion: scanning.V2,
		Data: map[string]interface{}{"response_str": body},
	})
	return msg
}

func buildUnknownVersionMessage(version int) []byte {
	msg, _ := json.Marshal(scanning.Scan{
		Ip: "1.2.3.4", Port: 80, Service: "HTTP",
		Timestamp: 1, DataVersion: version,
		Data: map[string]interface{}{},
	})
	return msg
}

// --- tests ---

func TestProcessMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		storeErr    error
		input       []byte
		wantAck     bool
		wantNack    bool
		wantUpserts int
		wantRecord  *store.ScanRecord // nil = skip record assertion
	}{
		{
			name:        "V1 happy path",
			input:       buildV1Message("10.0.0.1", 80, "HTTP", 1000, "hello"),
			wantAck:     true,
			wantUpserts: 1,
			wantRecord:  &store.ScanRecord{IP: "10.0.0.1", Port: 80, Service: "HTTP", Timestamp: 1000, Response: "hello"},
		},
		{
			name:        "V2 happy path",
			input:       buildV2Message("10.0.0.2", 443, "HTTPS", 2000, "world"),
			wantAck:     true,
			wantUpserts: 1,
			wantRecord:  &store.ScanRecord{IP: "10.0.0.2", Port: 443, Service: "HTTPS", Timestamp: 2000, Response: "world"},
		},
		{
			name:        "malformed JSON → ack (permanent failure)",
			input:       []byte(`{not json`),
			wantAck:     true,
			wantUpserts: 0,
		},
		{
			name:        "empty payload → ack (permanent failure)",
			input:       []byte(``),
			wantAck:     true,
			wantUpserts: 0,
		},
		{
			name:        "unknown version 99 → nack (retryable)",
			input:       buildUnknownVersionMessage(99),
			wantNack:    true,
			wantUpserts: 0,
		},
		{
			name:        "version 0 (iota) → nack (retryable)",
			input:       buildUnknownVersionMessage(0),
			wantNack:    true,
			wantUpserts: 0,
		},
		{
			name:        "DB error → nack (transient, safe to retry)",
			storeErr:    errors.New("disk full"),
			input:       buildV2Message("10.0.0.1", 80, "HTTP", 1000, "data"),
			wantNack:    true,
			wantUpserts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ms := &storemock.Store{Err: tt.storeErr}
			ig := ingestor.New(ms)
			at := &ackTracker{}

			ig.ProcessMessage(context.Background(), tt.input, at.ack, at.nack)

			if at.acked != tt.wantAck {
				t.Fatalf("acked = %v, want %v", at.acked, tt.wantAck)
			}
			if at.nacked != tt.wantNack {
				t.Fatalf("nacked = %v, want %v", at.nacked, tt.wantNack)
			}
			if ms.UpsertCount() != tt.wantUpserts {
				t.Fatalf("upserts = %d, want %d", ms.UpsertCount(), tt.wantUpserts)
			}
			if tt.wantRecord != nil {
				got := ms.LastRecord()
				if got != *tt.wantRecord {
					t.Errorf("record = %+v, want %+v", got, *tt.wantRecord)
				}
			}
		})
	}
}
