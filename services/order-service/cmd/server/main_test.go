package main

import (
	"io"
	"log"
	"testing"
	"time"
)

func TestGetEnvReturnsFallbackWhenUnset(t *testing.T) {
	t.Setenv("ORDER_SERVICE_TEST_ENV", "")
	got := getEnv("ORDER_SERVICE_TEST_ENV", "fallback")
	if got != "fallback" {
		t.Fatalf("unexpected env fallback: got %q, want %q", got, "fallback")
	}
}

func TestGetEnvReturnsConfiguredValue(t *testing.T) {
	t.Setenv("ORDER_SERVICE_TEST_ENV", "configured")
	got := getEnv("ORDER_SERVICE_TEST_ENV", "fallback")
	if got != "configured" {
		t.Fatalf("unexpected env value: got %q, want %q", got, "configured")
	}
}

func TestGetDurationEnvParsesConfiguredDuration(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	t.Setenv("ORDER_SERVICE_TEST_DURATION", "2s")

	got := getDurationEnv(logger, "ORDER_SERVICE_TEST_DURATION", 10*time.Second)
	if got != 2*time.Second {
		t.Fatalf("unexpected duration value: got %s, want %s", got, 2*time.Second)
	}
}

func TestGetDurationEnvFallsBackOnInvalidValue(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	t.Setenv("ORDER_SERVICE_TEST_DURATION", "invalid")

	fallback := 3 * time.Second
	got := getDurationEnv(logger, "ORDER_SERVICE_TEST_DURATION", fallback)
	if got != fallback {
		t.Fatalf("unexpected fallback duration: got %s, want %s", got, fallback)
	}
}
