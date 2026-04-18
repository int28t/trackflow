package service

import (
	"context"
	"errors"
	"fmt"
	"log"
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

type Notifier interface {
	NotifyStatusChanged(ctx context.Context, item model.StatusHistoryItem) error
}

type noopNotifier struct{}

func (noopNotifier) NotifyStatusChanged(_ context.Context, _ model.StatusHistoryItem) error {
	return nil
}

type TrackingService struct {
	repo     Repository
	notifier Notifier
}

func New(repo Repository) *TrackingService {
	return &TrackingService{
		repo:     repo,
		notifier: noopNotifier{},
	}
}

func (s *TrackingService) SetNotifier(notifier Notifier) *TrackingService {
	if s == nil {
		return nil
	}

	if notifier == nil {
		s.notifier = noopNotifier{}
		return s
	}

	s.notifier = notifier
	return s
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

	id := strings.TrimSpace(orderID)
	if id == "" {
		return nil, validationError("order_id is required")
	}

	if !uuidPattern.MatchString(id) {
		return nil, validationError("order_id must be a valid UUID")
	}

	return s.repo.GetOrderTimeline(ctx, id, normalizeLimit(limit))
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

	historyItem, err := s.repo.UpdateOrderStatus(ctx, id, nextStatus, source, comment, input.Metadata)
	if err != nil {
		return model.StatusHistoryItem{}, err
	}

	if s.notifier != nil {
		if notifyErr := s.notifier.NotifyStatusChanged(ctx, historyItem); notifyErr != nil {
			log.Printf("notification dispatch failed: order_id=%s status=%s err=%v", historyItem.OrderID, historyItem.Status, notifyErr)
		}
	}

	return historyItem, nil
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
