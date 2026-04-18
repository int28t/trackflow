package worker

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"trackflow/services/carrier-sync-service/internal/client"
	"trackflow/services/carrier-sync-service/internal/model"
	"trackflow/services/carrier-sync-service/internal/service"
)

const (
	defaultWorkerInterval    = 30 * time.Second
	defaultRetryMaxAttempts  = 3
	defaultRetryBaseBackoff  = 200 * time.Millisecond
	defaultRetryMaxBackoff   = 3 * time.Second
	defaultRetryJitterFactor = 0.2
)

type Worker struct {
	logger         *log.Logger
	svc            *service.SyncService
	trackingClient client.TrackingStatusClient
	interval       time.Duration
	batchSize      int

	retryMaxAttempts  int
	retryBaseBackoff  time.Duration
	retryMaxBackoff   time.Duration
	retryJitterFactor float64
	sleepFn           func(context.Context, time.Duration) error
	randFloat64       func() float64
}

func New(logger *log.Logger, svc *service.SyncService, trackingClient client.TrackingStatusClient, interval time.Duration, batchSize int) *Worker {
	if logger == nil {
		logger = log.Default()
	}

	if interval <= 0 {
		interval = defaultWorkerInterval
	}

	if batchSize <= 0 {
		batchSize = 5
	}

	randomizer := rand.New(rand.NewSource(time.Now().UnixNano()))

	return &Worker{
		logger:         logger,
		svc:            svc,
		trackingClient: trackingClient,
		interval:       interval,
		batchSize:      batchSize,

		retryMaxAttempts:  defaultRetryMaxAttempts,
		retryBaseBackoff:  defaultRetryBaseBackoff,
		retryMaxBackoff:   defaultRetryMaxBackoff,
		retryJitterFactor: defaultRetryJitterFactor,
		sleepFn:           sleepWithContext,
		randFloat64:       randomizer.Float64,
	}
}

func (w *Worker) Start(ctx context.Context) {
	if w == nil {
		return
	}

	if w.svc == nil {
		w.logger.Print("carrier sync worker disabled: sync service is not configured")
		return
	}

	if w.trackingClient == nil {
		w.logger.Print("carrier sync worker disabled: tracking client is not configured")
		return
	}

	w.logger.Printf(
		"carrier sync worker started: interval=%s batch_size=%d retry_max_attempts=%d",
		w.interval,
		w.batchSize,
		w.retryMaxAttempts,
	)
	w.runOnce(ctx)

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.logger.Printf("carrier sync worker stopped: %v", ctx.Err())
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

func (w *Worker) runOnce(ctx context.Context) {
	updates, err := w.fetchUpdatesWithRetry(ctx)
	if err != nil {
		w.logger.Printf("carrier sync cycle failed: %v", err)
		return
	}

	synced := 0
	failed := 0

	for _, update := range updates {
		err := w.pushUpdateWithRetry(ctx, update)
		if err != nil {
			failed++
			w.logger.Printf(
				"carrier update sync failed: order_id=%s external_status=%s err=%v",
				update.OrderID,
				update.ExternalStatus,
				err,
			)
			continue
		}

		synced++
	}

	w.logger.Printf("carrier sync cycle completed: fetched=%d synced=%d failed=%d", len(updates), synced, failed)
}

func (w *Worker) fetchUpdatesWithRetry(ctx context.Context) ([]model.StatusUpdate, error) {
	updates := make([]model.StatusUpdate, 0)

	err := w.withRetry(ctx, "carrier updates fetch", func() error {
		fetched, fetchErr := w.svc.SyncOnce(ctx, w.batchSize)
		if fetchErr != nil {
			return fetchErr
		}

		updates = fetched
		return nil
	})
	if err != nil {
		return nil, err
	}

	return updates, nil
}

func (w *Worker) pushUpdateWithRetry(ctx context.Context, update model.StatusUpdate) error {
	return w.withRetry(ctx, "carrier update push", func() error {
		return w.trackingClient.PushStatusUpdate(ctx, update)
	})
}

func (w *Worker) withRetry(ctx context.Context, operation string, fn func() error) error {
	if fn == nil {
		return fmt.Errorf("%s: retry function is not configured", operation)
	}

	maxAttempts := w.retryMaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err
		if attempt == maxAttempts {
			break
		}

		delay := w.backoffDelay(attempt)
		w.logger.Printf(
			"%s failed: attempt=%d/%d backoff=%s err=%v",
			operation,
			attempt,
			maxAttempts,
			delay,
			err,
		)

		sleepFn := w.sleepFn
		if sleepFn == nil {
			sleepFn = sleepWithContext
		}

		if sleepErr := sleepFn(ctx, delay); sleepErr != nil {
			return sleepErr
		}
	}

	return lastErr
}

func (w *Worker) backoffDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}

	baseBackoff := w.retryBaseBackoff
	if baseBackoff <= 0 {
		baseBackoff = defaultRetryBaseBackoff
	}

	maxBackoff := w.retryMaxBackoff
	if maxBackoff < baseBackoff {
		maxBackoff = baseBackoff
	}

	delay := baseBackoff
	for i := 1; i < attempt; i++ {
		if delay >= maxBackoff/2 {
			delay = maxBackoff
			break
		}

		delay *= 2
		if delay > maxBackoff {
			delay = maxBackoff
			break
		}
	}

	jitterFactor := w.retryJitterFactor
	if jitterFactor < 0 {
		jitterFactor = 0
	}
	if jitterFactor > 1 {
		jitterFactor = 1
	}

	if jitterFactor == 0 {
		return delay
	}

	jitterRange := time.Duration(float64(delay) * jitterFactor)
	if jitterRange <= 0 {
		return delay
	}

	randValue := 0.5
	if w.randFloat64 != nil {
		randValue = w.randFloat64()
	}
	if randValue < 0 {
		randValue = 0
	}
	if randValue > 1 {
		randValue = 1
	}

	delta := time.Duration((randValue*2 - 1) * float64(jitterRange))
	if delay+delta < 0 {
		return 0
	}

	return delay + delta
}

func sleepWithContext(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}

	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
