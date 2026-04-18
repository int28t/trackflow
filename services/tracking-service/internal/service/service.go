package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"trackflow/services/tracking-service/internal/model"
)

const (
	defaultTimelineLimit = 50
	maxTimelineLimit     = 200
)

var (
	ErrOrderNotFound = errors.New("order not found")
	ErrInvalidInput  = errors.New("invalid input")
	uuidPattern      = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
)

type Repository interface {
	Ping(ctx context.Context) error
	GetOrderTimeline(ctx context.Context, orderID string, limit int) ([]model.StatusHistoryItem, error)
	UpdateOrderStatus(ctx context.Context, orderID, nextStatus, source, comment string, metadata []byte) (model.StatusHistoryItem, error)
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

func (s *TrackingService) UpdateOrderStatus(ctx context.Context, orderID string, input model.UpdateStatusInput) (model.StatusHistoryItem, error) {
	if s == nil || s.repo == nil {
		return model.StatusHistoryItem{}, errors.New("repository is not configured")
	}

	id := strings.TrimSpace(orderID)
	if id == "" {
		return model.StatusHistoryItem{}, validationError("order_id is required")
	}

	if !uuidPattern.MatchString(id) {
		return model.StatusHistoryItem{}, validationError("order_id must be a valid UUID")
	}

	nextStatus, err := NormalizeStatus(input.Status)
	if err != nil {
		return model.StatusHistoryItem{}, validationError(err.Error())
	}

	sourceValue := strings.TrimSpace(input.Source)
	if sourceValue == "" {
		sourceValue = SourceSystem
	}

	source, err := NormalizeStatusSource(sourceValue)
	if err != nil {
		return model.StatusHistoryItem{}, validationError(err.Error())
	}

	comment := strings.TrimSpace(input.Comment)

	return s.repo.UpdateOrderStatus(ctx, id, nextStatus, source, comment, input.Metadata)
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

func validationError(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, message)
}
