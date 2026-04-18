package gateway

import (
	"context"
	"io"
	"log"
	"net"
	"testing"
	"time"
)

func TestParseDurationEnv(t *testing.T) {
	t.Parallel()

	logger := log.New(io.Discard, "", 0)

	t.Run("valid", func(t *testing.T) {
		t.Parallel()

		got := parseDurationEnv(logger, "UPSTREAM_REQUEST_TIMEOUT", "1500ms", 10*time.Second)
		if got != 1500*time.Millisecond {
			t.Fatalf("duration mismatch: got %s, want %s", got, 1500*time.Millisecond)
		}
	})

	t.Run("invalid fallback", func(t *testing.T) {
		t.Parallel()

		got := parseDurationEnv(logger, "UPSTREAM_REQUEST_TIMEOUT", "bad-value", 10*time.Second)
		if got != 10*time.Second {
			t.Fatalf("duration mismatch: got %s, want %s", got, 10*time.Second)
		}
	})

	t.Run("empty fallback", func(t *testing.T) {
		t.Parallel()

		got := parseDurationEnv(logger, "UPSTREAM_REQUEST_TIMEOUT", "", 10*time.Second)
		if got != 10*time.Second {
			t.Fatalf("duration mismatch: got %s, want %s", got, 10*time.Second)
		}
	})
}

func TestIsTimeoutError(t *testing.T) {
	t.Parallel()

	if !isTimeoutError(context.DeadlineExceeded) {
		t.Fatal("expected context.DeadlineExceeded to be timeout")
	}

	if !isTimeoutError(&net.DNSError{IsTimeout: true}) {
		t.Fatal("expected net timeout error to be timeout")
	}

	if isTimeoutError(context.Canceled) {
		t.Fatal("did not expect context.Canceled to be timeout")
	}
}
