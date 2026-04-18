package service

import (
	"context"
	"errors"
	"sync"

	"trackflow/services/carrier-sync-service/internal/model"
)

const (
	defaultBatchSize = 5
	maxBatchSize     = 50

	StatusSourceCarrier   = "carrier"
	StatusSourceLastKnown = "last_known"
)

type CarrierClient interface {
	FetchStatusUpdates(ctx context.Context, limit int) ([]model.StatusUpdate, error)
}

type SyncResult struct {
	Updates      []model.StatusUpdate
	StatusSource string
	FallbackUsed bool
}

type SyncService struct {
	client CarrierClient

	mu        sync.RWMutex
	lastKnown []model.StatusUpdate
}

func New(client CarrierClient) *SyncService {
	return &SyncService{client: client}
}

func (s *SyncService) Health(_ context.Context) error {
	if s == nil || s.client == nil {
		return errors.New("carrier client is not configured")
	}

	return nil
}

func (s *SyncService) SyncOnce(ctx context.Context, batchSize int) ([]model.StatusUpdate, error) {
	if s == nil || s.client == nil {
		return nil, errors.New("carrier client is not configured")
	}

	updates, err := s.client.FetchStatusUpdates(ctx, normalizeBatchSize(batchSize))
	if err != nil {
		return nil, err
	}

	s.storeLastKnown(updates)

	return cloneStatusUpdates(updates), nil
}

func (s *SyncService) SyncOnceWithFallback(ctx context.Context, batchSize int) (SyncResult, error) {
	updates, err := s.SyncOnce(ctx, batchSize)
	if err == nil {
		return SyncResult{
			Updates:      updates,
			StatusSource: StatusSourceCarrier,
			FallbackUsed: false,
		}, nil
	}

	lastKnown := s.LastKnownStatuses(batchSize)
	if len(lastKnown) == 0 {
		return SyncResult{}, err
	}

	return SyncResult{
		Updates:      lastKnown,
		StatusSource: StatusSourceLastKnown,
		FallbackUsed: true,
	}, nil
}

func (s *SyncService) LastKnownStatuses(batchSize int) []model.StatusUpdate {
	if s == nil {
		return nil
	}

	limit := normalizeBatchSize(batchSize)

	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.lastKnown) == 0 {
		return nil
	}

	if limit > len(s.lastKnown) {
		limit = len(s.lastKnown)
	}

	return cloneStatusUpdates(s.lastKnown[:limit])
}

func (s *SyncService) storeLastKnown(updates []model.StatusUpdate) {
	if s == nil || len(updates) == 0 {
		return
	}

	cloned := cloneStatusUpdates(updates)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastKnown = cloned
}

func normalizeBatchSize(batchSize int) int {
	if batchSize <= 0 {
		return defaultBatchSize
	}

	if batchSize > maxBatchSize {
		return maxBatchSize
	}

	return batchSize
}

func cloneStatusUpdates(updates []model.StatusUpdate) []model.StatusUpdate {
	if len(updates) == 0 {
		return nil
	}

	cloned := make([]model.StatusUpdate, len(updates))
	copy(cloned, updates)

	return cloned
}
