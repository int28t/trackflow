package service

import (
	"context"
	"errors"

	"trackflow/services/order-service/internal/model"
)

const (
	defaultListLimit = 20
	maxListLimit     = 100
)

type Repository interface {
	Ping(ctx context.Context) error
	ListOrders(ctx context.Context, limit int) ([]model.Order, error)
}

type OrderService struct {
	repo Repository
}

func New(repo Repository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) Health(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return errors.New("repository is not configured")
	}

	return s.repo.Ping(ctx)
}

func (s *OrderService) ListOrders(ctx context.Context, limit int) ([]model.Order, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("repository is not configured")
	}

	return s.repo.ListOrders(ctx, normalizeLimit(limit))
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}

	if limit > maxListLimit {
		return maxListLimit
	}

	return limit
}
