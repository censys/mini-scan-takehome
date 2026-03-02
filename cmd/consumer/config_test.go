package main

import (
	"os"
	"testing"
)

func TestEnvOrDefault(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envValue string // "" means unset
		fallback string
		want     string
	}{
		{"env set", "TEST_EOR_SET", "custom", "default", "custom"},
		{"env empty", "TEST_EOR_EMPTY", "", "default", "default"},
		{"env unset", "TEST_EOR_UNSET", "", "fallback", "fallback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			} else {
				os.Unsetenv(tt.key)
			}

			got := envOrDefault(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("envOrDefault(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestEnvOrDefaultInt(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		envValue string // "" means unset
		fallback int
		want     int
	}{
		{"valid int", "TEST_EORI_VALID", "42", 0, 42},
		{"unset", "TEST_EORI_UNSET", "", 99, 99},
		{"invalid string", "TEST_EORI_BAD", "abc", 77, 77},
		{"float value", "TEST_EORI_FLOAT", "3.14", 10, 10},
		{"negative int", "TEST_EORI_NEG", "-5", 0, -5},
		{"zero", "TEST_EORI_ZERO", "0", 50, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				t.Setenv(tt.key, tt.envValue)
			} else {
				os.Unsetenv(tt.key)
			}

			got := envOrDefaultInt(tt.key, tt.fallback)
			if got != tt.want {
				t.Errorf("envOrDefaultInt(%q, %d) = %d, want %d", tt.key, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Config
	}{
		{
			name: "defaults (no env set)",
			env:  nil,
			want: Config{
				ProjectID:           "test-project",
				SubscriptionID:      "scan-sub",
				DBPath:              "/data/scans.db",
				MaxOutstanding:      100,
				MaxOutstandingBytes: 104857600,
				NumGoroutines:       5,
			},
		},
		{
			name: "custom values",
			env: map[string]string{
				"PUBSUB_PROJECT_ID":            "my-project",
				"PUBSUB_SUBSCRIPTION_ID":       "my-sub",
				"DB_PATH":                      "/tmp/test.db",
				"PUBSUB_MAX_OUTSTANDING":       "50",
				"PUBSUB_MAX_OUTSTANDING_BYTES": "1048576",
				"PUBSUB_NUM_GOROUTINES":        "10",
			},
			want: Config{
				ProjectID:           "my-project",
				SubscriptionID:      "my-sub",
				DBPath:              "/tmp/test.db",
				MaxOutstanding:      50,
				MaxOutstandingBytes: 1048576,
				NumGoroutines:       10,
			},
		},
	}

	allKeys := []string{
		"PUBSUB_PROJECT_ID", "PUBSUB_SUBSCRIPTION_ID", "DB_PATH",
		"PUBSUB_MAX_OUTSTANDING", "PUBSUB_MAX_OUTSTANDING_BYTES", "PUBSUB_NUM_GOROUTINES",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear all keys first.
			for _, k := range allKeys {
				os.Unsetenv(k)
			}
			// Set the ones specified by this test case.
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got := LoadConfig()
			if got != tt.want {
				t.Errorf("LoadConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
