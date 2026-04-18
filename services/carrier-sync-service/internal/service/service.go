package service

import (
	"context"
	"errors"

	"trackflow/services/carrier-sync-service/internal/model"
)

const (
	defaultBatchSize = 5
	maxBatchSize     = 50
)

type CarrierClient interface {
	FetchStatusUpdates(ctx context.Context, limit int) ([]model.StatusUpdate, error)
}

type SyncService struct {
	client CarrierClient
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

	return s.client.FetchStatusUpdates(ctx, normalizeBatchSize(batchSize))
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
