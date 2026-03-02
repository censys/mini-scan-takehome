package parser_test

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/censys/scan-takehome/pkg/parser"
	"github.com/censys/scan-takehome/pkg/scanning"
	"github.com/censys/scan-takehome/pkg/store"
)

// buildMessage marshals a Scan into the JSON byte slice the parser expects.
func buildMessage(s scanning.Scan) []byte {
	b, err := json.Marshal(s)
	if err != nil {
		panic("test helper: " + err.Error())
	}
	return b
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      []byte
		wantRecord *store.ScanRecord // nil when error expected
		wantErr    error             // nil when success expected
	}{
		// --- success cases ---
		{
			name: "V1 valid",
			input: buildMessage(scanning.Scan{
				Ip: "192.168.1.1", Port: 80, Service: "HTTP",
				Timestamp: 1700000000, DataVersion: scanning.V1,
				Data: map[string]interface{}{
					"response_bytes_utf8": base64.StdEncoding.EncodeToString([]byte("HTTP/1.1 200 OK")),
				},
			}),
			wantRecord: &store.ScanRecord{
				IP: "192.168.1.1", Port: 80, Service: "HTTP",
				Timestamp: 1700000000, Response: "HTTP/1.1 200 OK",
			},
		},
		{
			name: "V2 valid",
			input: buildMessage(scanning.Scan{
				Ip: "10.0.0.1", Port: 443, Service: "HTTPS",
				Timestamp: 1700000001, DataVersion: scanning.V2,
				Data: map[string]interface{}{"response_str": "HTTP/1.1 200 OK"},
			}),
			wantRecord: &store.ScanRecord{
				IP: "10.0.0.1", Port: 443, Service: "HTTPS",
				Timestamp: 1700000001, Response: "HTTP/1.1 200 OK",
			},
		},
		{
			name: "V1 empty response",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: scanning.V1,
				Data: map[string]interface{}{
					"response_bytes_utf8": base64.StdEncoding.EncodeToString([]byte("")),
				},
			}),
			wantRecord: &store.ScanRecord{
				IP: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, Response: "",
			},
		},
		{
			name: "V2 empty response",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: scanning.V2,
				Data: map[string]interface{}{"response_str": ""},
			}),
			wantRecord: &store.ScanRecord{
				IP: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, Response: "",
			},
		},
		{
			name: "preserves all fields (max values)",
			input: buildMessage(scanning.Scan{
				Ip: "255.255.255.255", Port: 65535, Service: "FTP",
				Timestamp: 9999999999, DataVersion: scanning.V2,
				Data: map[string]interface{}{"response_str": "220 Welcome"},
			}),
			wantRecord: &store.ScanRecord{
				IP: "255.255.255.255", Port: 65535, Service: "FTP",
				Timestamp: 9999999999, Response: "220 Welcome",
			},
		},
		// --- error cases ---
		{
			name:    "invalid JSON",
			input:   []byte(`{not json`),
			wantErr: parser.ErrMalformed,
		},
		{
			name:    "empty payload",
			input:   []byte(``),
			wantErr: parser.ErrMalformed,
		},
		{
			name: "unknown version (99)",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: 99,
				Data: map[string]interface{}{},
			}),
			wantErr: parser.ErrUnknownVersion,
		},
		{
			name: "phantom version 0 (Version iota)",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: 0,
				Data: map[string]interface{}{},
			}),
			wantErr: parser.ErrUnknownVersion,
		},
		{
			name: "V1 missing response_bytes_utf8",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: scanning.V1,
				Data: map[string]interface{}{"other_field": "x"},
			}),
			wantErr: parser.ErrMalformed,
		},
		{
			name: "V1 invalid base64",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: scanning.V1,
				Data: map[string]interface{}{
					"response_bytes_utf8": "!!!not-base64!!!",
				},
			}),
			wantErr: parser.ErrMalformed,
		},
		{
			name: "V1 response_bytes_utf8 wrong type (number)",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: scanning.V1,
				Data: map[string]interface{}{
					"response_bytes_utf8": 42,
				},
			}),
			wantErr: parser.ErrMalformed,
		},
		{
			name: "negative version",
			input: buildMessage(scanning.Scan{
				Ip: "1.2.3.4", Port: 80, Service: "HTTP",
				Timestamp: 1, DataVersion: -1,
				Data: map[string]interface{}{},
			}),
			wantErr: parser.ErrUnknownVersion,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec, err := parser.Parse(tt.input)

			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantRecord != nil && rec != *tt.wantRecord {
				t.Errorf("got %+v, want %+v", rec, *tt.wantRecord)
			}
		})
	}
}
