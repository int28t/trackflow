package client

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"trackflow/services/carrier-sync-service/internal/model"
	"trackflow/services/carrier-sync-service/internal/requestid"
)

func TestPushStatusUpdateMapsStatusAndSendsMetadata(t *testing.T) {
	t.Parallel()

	var gotPath string
	var gotRequest trackingStatusUpdateRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}

		if err := json.NewDecoder(r.Body).Decode(&gotRequest); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpClient, err := NewTrackingHTTPClient(log.New(io.Discard, "", 0), server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewTrackingHTTPClient returned error: %v", err)
	}

	updatedAt := time.Date(2026, 4, 18, 14, 30, 0, 0, time.UTC)
	err = httpClient.PushStatusUpdate(context.Background(), model.StatusUpdate{
		OrderID:        "order-1",
		ExternalStatus: "out-for-delivery",
		UpdatedAt:      updatedAt,
	})
	if err != nil {
		t.Fatalf("PushStatusUpdate returned error: %v", err)
	}

	if gotPath != "/orders/order-1/status" {
		t.Fatalf("unexpected request path: %s", gotPath)
	}

	if gotRequest.Status != "in_transit" {
		t.Fatalf("unexpected mapped status: %s", gotRequest.Status)
	}

	if gotRequest.Source != trackingStatusSource {
		t.Fatalf("unexpected source: %s", gotRequest.Source)
	}

	if gotRequest.Metadata == nil {
		t.Fatal("expected metadata, got nil")
	}

	externalStatus, ok := gotRequest.Metadata["carrier_external_status"].(string)
	if !ok {
		t.Fatalf("carrier_external_status metadata has unexpected type: %T", gotRequest.Metadata["carrier_external_status"])
	}

	if externalStatus != "out-for-delivery" {
		t.Fatalf("unexpected carrier_external_status metadata: %s", externalStatus)
	}

	carrierUpdatedAt, ok := gotRequest.Metadata["carrier_updated_at"].(string)
	if !ok {
		t.Fatalf("carrier_updated_at metadata has unexpected type: %T", gotRequest.Metadata["carrier_updated_at"])
	}

	if carrierUpdatedAt != updatedAt.Format(time.RFC3339) {
		t.Fatalf("unexpected carrier_updated_at metadata: %s", carrierUpdatedAt)
	}
}

func TestPushStatusUpdateUnknownExternalStatus(t *testing.T) {
	t.Parallel()

	httpClient, err := NewTrackingHTTPClient(log.New(io.Discard, "", 0), "http://tracking-service:8083", time.Second)
	if err != nil {
		t.Fatalf("NewTrackingHTTPClient returned error: %v", err)
	}

	err = httpClient.PushStatusUpdate(context.Background(), model.StatusUpdate{
		OrderID:        "order-1",
		ExternalStatus: "not-supported-status",
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "map external status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPushStatusUpdatePropagatesRequestIDHeaders(t *testing.T) {
	t.Parallel()

	const expectedRequestID = "req-carrier-123"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(requestid.HeaderName); got != expectedRequestID {
			t.Fatalf("unexpected %s header: got %q, want %q", requestid.HeaderName, got, expectedRequestID)
		}

		if got := r.Header.Get(requestid.CorrelationHeaderName); got != expectedRequestID {
			t.Fatalf("unexpected %s header: got %q, want %q", requestid.CorrelationHeaderName, got, expectedRequestID)
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	httpClient, err := NewTrackingHTTPClient(log.New(io.Discard, "", 0), server.URL, time.Second)
	if err != nil {
		t.Fatalf("NewTrackingHTTPClient returned error: %v", err)
	}

	ctx := requestid.WithRequestID(context.Background(), expectedRequestID)
	err = httpClient.PushStatusUpdate(ctx, model.StatusUpdate{
		OrderID:        "order-1",
		ExternalStatus: "out-for-delivery",
		UpdatedAt:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("PushStatusUpdate returned error: %v", err)
	}
}
