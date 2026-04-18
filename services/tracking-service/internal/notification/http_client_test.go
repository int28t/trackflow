package notification

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"trackflow/services/tracking-service/internal/model"
)

func TestNotifyStatusChangedSendsEmailAndTelegram(t *testing.T) {
	t.Parallel()

	type captured struct {
		Channel   string `json:"channel"`
		Recipient string `json:"recipient"`
		OrderID   string `json:"order_id"`
		Status    string `json:"status"`
		Message   string `json:"message"`
	}

	capturedRequests := make([]captured, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		if r.URL.Path != "/internal/notifications/send" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}

		var payload captured
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}

		capturedRequests = append(capturedRequests, payload)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewHTTPClient(log.New(io.Discard, "", 0), server.URL, time.Second, "user@example.com", "@trackflow_user")
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}

	err = client.NotifyStatusChanged(context.Background(), model.StatusHistoryItem{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:    "in_transit",
		CreatedAt: time.Date(2026, 4, 18, 16, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NotifyStatusChanged returned error: %v", err)
	}

	if len(capturedRequests) != 2 {
		t.Fatalf("unexpected requests count: got %d, want %d", len(capturedRequests), 2)
	}

	foundEmail := false
	foundTelegram := false
	for _, req := range capturedRequests {
		if req.OrderID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
			t.Fatalf("unexpected order id: %s", req.OrderID)
		}
		if req.Status != "in_transit" {
			t.Fatalf("unexpected status: %s", req.Status)
		}
		if req.Message == "" {
			t.Fatal("message must not be empty")
		}

		switch req.Channel {
		case "email":
			foundEmail = true
			if req.Recipient != "user@example.com" {
				t.Fatalf("unexpected email recipient: %s", req.Recipient)
			}
		case "telegram":
			foundTelegram = true
			if req.Recipient != "@trackflow_user" {
				t.Fatalf("unexpected telegram recipient: %s", req.Recipient)
			}
		default:
			t.Fatalf("unexpected channel: %s", req.Channel)
		}
	}

	if !foundEmail || !foundTelegram {
		t.Fatalf("expected both email and telegram notifications, got email=%t telegram=%t", foundEmail, foundTelegram)
	}
}

func TestNotifyStatusChangedReturnsErrorOnChannelFailure(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload failed: %v", err)
		}

		channel, _ := payload["channel"].(string)
		if channel == "telegram" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"temporary failure"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewHTTPClient(log.New(io.Discard, "", 0), server.URL, time.Second, "user@example.com", "@trackflow_user")
	if err != nil {
		t.Fatalf("NewHTTPClient returned error: %v", err)
	}

	err = client.NotifyStatusChanged(context.Background(), model.StatusHistoryItem{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:    "delivered",
		CreatedAt: time.Now().UTC(),
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
