package main

import (
	"context"
	"io"
	"log"
	"testing"
	"time"
)

func TestGetEnvReturnsFallbackWhenUnset(t *testing.T) {
	t.Setenv("CARRIER_SYNC_SERVICE_TEST_ENV", "")

	got := getEnv("CARRIER_SYNC_SERVICE_TEST_ENV", "fallback")
	if got != "fallback" {
		t.Fatalf("unexpected env fallback: got %q, want %q", got, "fallback")
	}
}

func TestGetEnvReturnsConfiguredValue(t *testing.T) {
	t.Setenv("CARRIER_SYNC_SERVICE_TEST_ENV", "configured")

	got := getEnv("CARRIER_SYNC_SERVICE_TEST_ENV", "fallback")
	if got != "configured" {
		t.Fatalf("unexpected env value: got %q, want %q", got, "configured")
	}
}

func TestGetDurationEnvParsesConfiguredDuration(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	t.Setenv("CARRIER_SYNC_TEST_DURATION", "2s")

	got := getDurationEnv(logger, "CARRIER_SYNC_TEST_DURATION", 10*time.Second)
	if got != 2*time.Second {
		t.Fatalf("unexpected duration value: got %s, want %s", got, 2*time.Second)
	}
}

func TestGetDurationEnvFallsBackOnInvalidValue(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	t.Setenv("CARRIER_SYNC_TEST_DURATION", "invalid")

	fallback := 3 * time.Second
	got := getDurationEnv(logger, "CARRIER_SYNC_TEST_DURATION", fallback)
	if got != fallback {
		t.Fatalf("unexpected fallback duration: got %s, want %s", got, fallback)
	}
}

func TestBuildCarrierClientFromEnvMock(t *testing.T) {
	t.Setenv(clientModeEnvKey, "mock")
	t.Setenv(carrierBaseURLEnvKey, "https://carrier.local/api")
	t.Setenv(carrierTokenEnvKey, "token")

	carrierClient, err := buildCarrierClientFromEnv()
	if err != nil {
		t.Fatalf("buildCarrierClientFromEnv returned error: %v", err)
	}

	updates, err := carrierClient.FetchStatusUpdates(context.Background(), 1)
	if err != nil {
		t.Fatalf("carrier client fetch failed: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("unexpected updates length: got %d, want %d", len(updates), 1)
	}
}

func TestBuildCarrierClientFromEnvUnsupportedMode(t *testing.T) {
	t.Setenv(clientModeEnvKey, "unsupported")

	_, err := buildCarrierClientFromEnv()
	if err == nil {
		t.Fatal("expected error for unsupported carrier mode")
	}
}

func TestGetSyncInterval(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	t.Setenv(syncIntervalEnvKey, "45s")

	if got := getSyncInterval(logger); got != 45*time.Second {
		t.Fatalf("unexpected sync interval: got %s, want %s", got, 45*time.Second)
	}
}
