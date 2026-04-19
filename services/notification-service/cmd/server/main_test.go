package main

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	"trackflow/services/notification-service/internal/model"
)

func TestGetEnvReturnsFallbackWhenUnset(t *testing.T) {
	t.Setenv("NOTIFICATION_SERVICE_TEST_ENV", "")

	got := getEnv("NOTIFICATION_SERVICE_TEST_ENV", "fallback")
	if got != "fallback" {
		t.Fatalf("unexpected env fallback: got %q, want %q", got, "fallback")
	}
}

func TestGetEnvReturnsConfiguredValue(t *testing.T) {
	t.Setenv("NOTIFICATION_SERVICE_TEST_ENV", "configured")

	got := getEnv("NOTIFICATION_SERVICE_TEST_ENV", "fallback")
	if got != "configured" {
		t.Fatalf("unexpected env value: got %q, want %q", got, "configured")
	}
}

func TestGetDurationEnvParsesConfiguredDuration(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	t.Setenv("NOTIFICATION_SERVICE_TEST_DURATION", "2s")

	got := getDurationEnv(logger, "NOTIFICATION_SERVICE_TEST_DURATION", 10*time.Second)
	if got != 2*time.Second {
		t.Fatalf("unexpected duration value: got %s, want %s", got, 2*time.Second)
	}
}

func TestGetDurationEnvFallsBackOnInvalidValue(t *testing.T) {
	logger := log.New(io.Discard, "", 0)
	t.Setenv("NOTIFICATION_SERVICE_TEST_DURATION", "invalid")

	fallback := 3 * time.Second
	got := getDurationEnv(logger, "NOTIFICATION_SERVICE_TEST_DURATION", fallback)
	if got != fallback {
		t.Fatalf("unexpected fallback duration: got %s, want %s", got, fallback)
	}
}

func TestBuildSenderMockProvider(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	messageSender, err := buildSender(logger, "mock", "token")
	if err != nil {
		t.Fatalf("buildSender returned error: %v", err)
	}

	err = messageSender.Send(context.Background(), model.Event{
		OrderID:   "order-1",
		Status:    "created",
		Channel:   "email",
		Recipient: "user@example.com",
		Message:   "created",
	})
	if err != nil {
		t.Fatalf("sender.Send returned error: %v", err)
	}
}

func TestBuildSenderUnsupportedProvider(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	_, err := buildSender(logger, "smtp", "token")
	if err == nil {
		t.Fatal("expected error for unsupported provider")
	}
}
