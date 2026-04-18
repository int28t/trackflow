package worker

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"
	"time"

	"trackflow/services/carrier-sync-service/internal/model"
	"trackflow/services/carrier-sync-service/internal/service"
)

type stubCarrierClient struct {
	failFor int
	calls   int
	updates []model.StatusUpdate
}

func (c *stubCarrierClient) FetchStatusUpdates(_ context.Context, _ int) ([]model.StatusUpdate, error) {
	c.calls++
	if c.calls <= c.failFor {
		return nil, errors.New("temporary carrier error")
	}

	return c.updates, nil
}

type stubTrackingClient struct {
	failFor int
	calls   int
}

func (c *stubTrackingClient) PushStatusUpdate(_ context.Context, _ model.StatusUpdate) error {
	c.calls++
	if c.calls <= c.failFor {
		return errors.New("temporary tracking error")
	}

	return nil
}

type blockingCarrierClient struct {
	calls int
}

func (c *blockingCarrierClient) FetchStatusUpdates(ctx context.Context, _ int) ([]model.StatusUpdate, error) {
	c.calls++
	<-ctx.Done()
	return nil, ctx.Err()
}

type blockingTrackingClient struct {
	calls int
}

func (c *blockingTrackingClient) PushStatusUpdate(ctx context.Context, _ model.StatusUpdate) error {
	c.calls++
	<-ctx.Done()
	return ctx.Err()
}

type carrierResponse struct {
	updates []model.StatusUpdate
	err     error
}

type sequenceCarrierClient struct {
	responses []carrierResponse
	idx       int
	calls     int
}

func (c *sequenceCarrierClient) FetchStatusUpdates(_ context.Context, _ int) ([]model.StatusUpdate, error) {
	c.calls++
	if len(c.responses) == 0 {
		return nil, nil
	}

	if c.idx >= len(c.responses) {
		last := c.responses[len(c.responses)-1]
		return last.updates, last.err
	}

	current := c.responses[c.idx]
	c.idx++

	return current.updates, current.err
}

func TestWorkerRetriesFetchWithExponentialBackoff(t *testing.T) {
	t.Parallel()

	carrierClient := &stubCarrierClient{
		failFor: 2,
		updates: []model.StatusUpdate{{OrderID: "order-1", ExternalStatus: "created"}},
	}
	trackingClient := &stubTrackingClient{}
	syncService := service.New(carrierClient)

	worker := New(log.New(io.Discard, "", 0), syncService, trackingClient, time.Second, 1)
	worker.retryMaxAttempts = 4
	worker.retryBaseBackoff = 100 * time.Millisecond
	worker.retryMaxBackoff = 800 * time.Millisecond
	worker.retryJitterFactor = 0

	sleepCalls := 0
	worker.sleepFn = func(_ context.Context, _ time.Duration) error {
		sleepCalls++
		return nil
	}

	worker.runOnce(context.Background())

	if carrierClient.calls != 3 {
		t.Fatalf("unexpected carrier fetch calls: got %d, want %d", carrierClient.calls, 3)
	}

	if trackingClient.calls != 1 {
		t.Fatalf("unexpected tracking push calls: got %d, want %d", trackingClient.calls, 1)
	}

	if sleepCalls != 2 {
		t.Fatalf("unexpected sleep calls: got %d, want %d", sleepCalls, 2)
	}
}

func TestWorkerRetriesPushWithExponentialBackoff(t *testing.T) {
	t.Parallel()

	carrierClient := &stubCarrierClient{
		updates: []model.StatusUpdate{{OrderID: "order-1", ExternalStatus: "created"}},
	}
	trackingClient := &stubTrackingClient{failFor: 2}
	syncService := service.New(carrierClient)

	worker := New(log.New(io.Discard, "", 0), syncService, trackingClient, time.Second, 1)
	worker.retryMaxAttempts = 4
	worker.retryBaseBackoff = 100 * time.Millisecond
	worker.retryMaxBackoff = 800 * time.Millisecond
	worker.retryJitterFactor = 0

	sleepCalls := 0
	worker.sleepFn = func(_ context.Context, _ time.Duration) error {
		sleepCalls++
		return nil
	}

	worker.runOnce(context.Background())

	if trackingClient.calls != 3 {
		t.Fatalf("unexpected tracking push calls: got %d, want %d", trackingClient.calls, 3)
	}

	if sleepCalls != 2 {
		t.Fatalf("unexpected sleep calls: got %d, want %d", sleepCalls, 2)
	}
}

func TestBackoffDelayIsExponentialAndCapped(t *testing.T) {
	t.Parallel()

	worker := &Worker{
		retryBaseBackoff:  100 * time.Millisecond,
		retryMaxBackoff:   500 * time.Millisecond,
		retryJitterFactor: 0,
	}

	testCases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 100 * time.Millisecond},
		{attempt: 2, want: 200 * time.Millisecond},
		{attempt: 3, want: 400 * time.Millisecond},
		{attempt: 4, want: 500 * time.Millisecond},
		{attempt: 5, want: 500 * time.Millisecond},
	}

	for _, tc := range testCases {
		got := worker.backoffDelay(tc.attempt)
		if got != tc.want {
			t.Fatalf("backoff mismatch for attempt %d: got %s, want %s", tc.attempt, got, tc.want)
		}
	}
}

func TestBackoffDelayAppliesJitter(t *testing.T) {
	t.Parallel()

	worker := &Worker{
		retryBaseBackoff:  100 * time.Millisecond,
		retryMaxBackoff:   1 * time.Second,
		retryJitterFactor: 0.5,
		randFloat64:       func() float64 { return 1 },
	}

	got := worker.backoffDelay(2)
	want := 300 * time.Millisecond
	if got != want {
		t.Fatalf("jittered backoff mismatch: got %s, want %s", got, want)
	}
}

func TestWorkerUsesCarrierTimeout(t *testing.T) {
	t.Parallel()

	carrierClient := &blockingCarrierClient{}
	trackingClient := &stubTrackingClient{}
	syncService := service.New(carrierClient)

	worker := New(log.New(io.Discard, "", 0), syncService, trackingClient, time.Second, 1)
	worker.retryMaxAttempts = 1
	worker.carrierTimeout = 20 * time.Millisecond

	start := time.Now()
	worker.runOnce(context.Background())
	elapsed := time.Since(start)

	if carrierClient.calls != 1 {
		t.Fatalf("unexpected carrier fetch calls: got %d, want %d", carrierClient.calls, 1)
	}

	if elapsed > 300*time.Millisecond {
		t.Fatalf("carrier timeout was not applied, elapsed=%s", elapsed)
	}
}

func TestWorkerUsesTrackingTimeout(t *testing.T) {
	t.Parallel()

	carrierClient := &stubCarrierClient{
		updates: []model.StatusUpdate{{OrderID: "order-1", ExternalStatus: "created"}},
	}
	trackingClient := &blockingTrackingClient{}
	syncService := service.New(carrierClient)

	worker := New(log.New(io.Discard, "", 0), syncService, trackingClient, time.Second, 1)
	worker.retryMaxAttempts = 1
	worker.trackingTimeout = 20 * time.Millisecond

	start := time.Now()
	worker.runOnce(context.Background())
	elapsed := time.Since(start)

	if trackingClient.calls != 1 {
		t.Fatalf("unexpected tracking push calls: got %d, want %d", trackingClient.calls, 1)
	}

	if elapsed > 300*time.Millisecond {
		t.Fatalf("tracking timeout was not applied, elapsed=%s", elapsed)
	}
}

func TestWorkerUsesLastKnownFallbackWithoutPush(t *testing.T) {
	t.Parallel()

	carrierClient := &sequenceCarrierClient{
		responses: []carrierResponse{
			{
				updates: []model.StatusUpdate{{OrderID: "order-1", ExternalStatus: "created"}},
			},
			{
				err: errors.New("carrier unavailable"),
			},
		},
	}

	trackingClient := &stubTrackingClient{}
	syncService := service.New(carrierClient)

	worker := New(log.New(io.Discard, "", 0), syncService, trackingClient, time.Second, 1)
	worker.retryMaxAttempts = 1

	worker.runOnce(context.Background())
	worker.runOnce(context.Background())

	if carrierClient.calls != 2 {
		t.Fatalf("unexpected carrier fetch calls: got %d, want %d", carrierClient.calls, 2)
	}

	if trackingClient.calls != 1 {
		t.Fatalf("unexpected tracking push calls: got %d, want %d", trackingClient.calls, 1)
	}
}
