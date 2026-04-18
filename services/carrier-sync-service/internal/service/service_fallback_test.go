package service

import (
	"context"
	"errors"
	"testing"

	"trackflow/services/carrier-sync-service/internal/model"
)

type carrierResult struct {
	updates []model.StatusUpdate
	err     error
}

type sequenceCarrierClient struct {
	results []carrierResult
	idx     int
}

func (c *sequenceCarrierClient) FetchStatusUpdates(_ context.Context, _ int) ([]model.StatusUpdate, error) {
	if len(c.results) == 0 {
		return nil, nil
	}

	if c.idx >= len(c.results) {
		last := c.results[len(c.results)-1]
		return last.updates, last.err
	}

	current := c.results[c.idx]
	c.idx++
	return current.updates, current.err
}

func TestSyncOnceWithFallbackReturnsLastKnown(t *testing.T) {
	t.Parallel()

	client := &sequenceCarrierClient{
		results: []carrierResult{
			{
				updates: []model.StatusUpdate{{OrderID: "order-1", ExternalStatus: "created"}},
			},
			{
				err: errors.New("carrier unavailable"),
			},
		},
	}

	svc := New(client)

	first, err := svc.SyncOnceWithFallback(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected first sync error: %v", err)
	}

	if first.FallbackUsed {
		t.Fatal("expected first sync without fallback")
	}

	if first.StatusSource != StatusSourceCarrier {
		t.Fatalf("unexpected status source: got %q, want %q", first.StatusSource, StatusSourceCarrier)
	}

	second, err := svc.SyncOnceWithFallback(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected second sync error: %v", err)
	}

	if !second.FallbackUsed {
		t.Fatal("expected fallback on second sync")
	}

	if second.StatusSource != StatusSourceLastKnown {
		t.Fatalf("unexpected fallback status source: got %q, want %q", second.StatusSource, StatusSourceLastKnown)
	}

	if len(second.Updates) != 1 {
		t.Fatalf("unexpected fallback updates count: got %d, want %d", len(second.Updates), 1)
	}

	if second.Updates[0].OrderID != "order-1" {
		t.Fatalf("unexpected fallback order id: got %q, want %q", second.Updates[0].OrderID, "order-1")
	}
}

func TestSyncOnceWithFallbackWithoutLastKnownReturnsError(t *testing.T) {
	t.Parallel()

	client := &sequenceCarrierClient{
		results: []carrierResult{{err: errors.New("carrier unavailable")}},
	}

	svc := New(client)

	_, err := svc.SyncOnceWithFallback(context.Background(), 10)
	if err == nil {
		t.Fatal("expected error when no last known statuses")
	}
}

func TestLastKnownStatusesAreCloned(t *testing.T) {
	t.Parallel()

	client := &sequenceCarrierClient{
		results: []carrierResult{{updates: []model.StatusUpdate{{OrderID: "order-1", ExternalStatus: "created"}}}},
	}

	svc := New(client)

	updates, err := svc.SyncOnce(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected sync error: %v", err)
	}

	updates[0].ExternalStatus = "mutated"

	lastKnown := svc.LastKnownStatuses(10)
	if len(lastKnown) != 1 {
		t.Fatalf("unexpected last known count: got %d, want %d", len(lastKnown), 1)
	}

	if lastKnown[0].ExternalStatus != "created" {
		t.Fatalf("expected cloned last known status, got %q", lastKnown[0].ExternalStatus)
	}
}
