package parser

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/censys/scan-takehome/pkg/scanning"
	"github.com/censys/scan-takehome/pkg/store"
)

// ErrMalformed: permanent failure (bad JSON, missing fields) -> consumer should ACK.
// ErrUnknownVersion: valid data but unsupported version (e.g. V3) -> consumer should NACK
// so Pub/Sub can redeliver or route to a dead-letter topic.
var (
	ErrMalformed      = errors.New("malformed message")
	ErrUnknownVersion = errors.New("unknown data version")
)

// Parse takes a raw Pub/Sub message, resolves V1/V2 format differences,
// and returns a version-agnostic ScanRecord. Pipeline: Pull -> Parse -> Upsert -> Ack
func Parse(data []byte) (store.ScanRecord, error) {
	var scan scanning.Scan
	// unmarshalls outer strcuture (including Data as map[string]interface{}), we'll decode Data into the correct struct later based on DataVersion
	if err := json.Unmarshal(data, &scan); err != nil {
		return store.ScanRecord{}, fmt.Errorf("%w: json unmarshal: %v", ErrMalformed, err)
	}

	response, err := extractResponse(&scan)
	if err != nil {
		return store.ScanRecord{}, err
	}

	return store.ScanRecord{
		IP:        scan.Ip,
		Port:      scan.Port,
		Service:   scan.Service,
		Timestamp: scan.Timestamp,
		Response:  response,
	}, nil
}

// extractResponse extracts the service response string from Scan.Data.
// Because Data is interface{}, JSON decodes it as map[string]interface{} --
// we re-marshal and decode into the correct typed struct per version.
func extractResponse(scan *scanning.Scan) (string, error) {
	rawData, err := json.Marshal(scan.Data)
	if err != nil {
		return "", fmt.Errorf("%w: cannot re-marshal data field: %v", ErrMalformed, err)
	}

	switch scan.DataVersion {
	case scanning.V1:
		// V1: []byte was base64-encoded by JSON. Decode it back to a string.
		var rawMap map[string]interface{}
		if err := json.Unmarshal(rawData, &rawMap); err != nil {
			return "", fmt.Errorf("%w: v1 data unmarshal: %v", ErrMalformed, err)
		}
		encoded, ok := rawMap["response_bytes_utf8"].(string)
		if !ok {
			return "", fmt.Errorf("%w: v1 response_bytes_utf8 missing or wrong type", ErrMalformed)
		}
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return "", fmt.Errorf("%w: v1 base64 decode: %v", ErrMalformed, err)
		}
		return string(decoded), nil

	case scanning.V2:
		var v2 scanning.V2Data
		if err := json.Unmarshal(rawData, &v2); err != nil {
			return "", fmt.Errorf("%w: v2 data unmarshal: %v", ErrMalformed, err)
		}
		return v2.ResponseStr, nil

	default:
		// Valid data, unknown version -- not malformed, just unsupported yet.
		return "", fmt.Errorf("%w: data_version=%d", ErrUnknownVersion, scan.DataVersion)
	}
}
