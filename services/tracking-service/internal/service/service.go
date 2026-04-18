package service

import (
	"context"
	"errors"

	"trackflow/services/tracking-service/internal/model"
)

const (
	defaultTimelineLimit = 50
	maxTimelineLimit     = 200
)

type Repository interface {
	Ping(ctx context.Context) error
	GetOrderTimeline(ctx context.Context, orderID string, limit int) ([]model.StatusHistoryItem, error)
}

type TrackingService struct {
	repo Repository
}

func New(repo Repository) *TrackingService {
	return &TrackingService{repo: repo}
}

func (s *TrackingService) Health(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return errors.New("repository is not configured")
	}

	return s.repo.Ping(ctx)
}

func (s *TrackingService) GetOrderTimeline(ctx context.Context, orderID string, limit int) ([]model.StatusHistoryItem, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("repository is not configured")
	}

	return s.repo.GetOrderTimeline(ctx, orderID, normalizeLimit(limit))
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultTimelineLimit
	}

	if limit > maxTimelineLimit {
		return maxTimelineLimit
	}

	return limit
}
