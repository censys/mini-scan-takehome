package main

import (
	"log"
	"os"
	"strconv"
)

// Config holds every tuneable for the consumer, loaded from environment variables.
type Config struct {
	ProjectID           string
	SubscriptionID      string
	DBPath              string
	MaxOutstanding      int
	MaxOutstandingBytes int
	NumGoroutines       int
}

// LoadConfig reads each setting from its environment variable, falling back
// to a sensible default when unset. Plain and explicit — no reflection needed.
func LoadConfig() Config {
	return Config{
		ProjectID:           envOrDefault("PUBSUB_PROJECT_ID", "test-project"),
		SubscriptionID:      envOrDefault("PUBSUB_SUBSCRIPTION_ID", "scan-sub"),
		DBPath:              envOrDefault("DB_PATH", "/data/scans.db"),
		MaxOutstanding:      envOrDefaultInt("PUBSUB_MAX_OUTSTANDING", 100),
		MaxOutstandingBytes: envOrDefaultInt("PUBSUB_MAX_OUTSTANDING_BYTES", 104857600), // 100 MB
		NumGoroutines:       envOrDefaultInt("PUBSUB_NUM_GOROUTINES", 5),
	}
}

// envOrDefault returns the value of the environment variable key, or fallback
// if the variable is unset or empty.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envOrDefaultInt returns the integer value of the environment variable key,
// or fallback if the variable is unset, empty, or not a valid integer.
func envOrDefaultInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		log.Printf("WARN: invalid %s=%q, using default %d", key, raw, fallback)
		return fallback
	}
	return n
}
