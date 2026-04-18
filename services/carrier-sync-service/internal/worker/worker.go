package worker

import (
	"context"
	"log"
	"time"

	"trackflow/services/carrier-sync-service/internal/service"
)

const defaultWorkerInterval = 30 * time.Second

type Worker struct {
	logger    *log.Logger
	svc       *service.SyncService
	interval  time.Duration
	batchSize int
}

func New(logger *log.Logger, svc *service.SyncService, interval time.Duration, batchSize int) *Worker {
	if logger == nil {
		logger = log.Default()
	}

	if interval <= 0 {
		interval = defaultWorkerInterval
	}

	if batchSize <= 0 {
		batchSize = 5
	}

	return &Worker{
		logger:    logger,
		svc:       svc,
		interval:  interval,
		batchSize: batchSize,
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

	w.logger.Printf("carrier sync worker started: interval=%s, batch_size=%d", w.interval, w.batchSize)
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
	updates, err := w.svc.SyncOnce(ctx, w.batchSize)
	if err != nil {
		w.logger.Printf("carrier sync cycle failed: %v", err)
		return
	}

	for _, update := range updates {
		w.logger.Printf(
			"carrier update fetched: order_id=%s external_status=%s updated_at=%s",
			update.OrderID,
			update.ExternalStatus,
			update.UpdatedAt.Format(time.RFC3339),
		)
	}

	w.logger.Printf("carrier sync cycle completed: fetched=%d", len(updates))
}
