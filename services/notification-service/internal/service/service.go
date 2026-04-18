package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"trackflow/services/notification-service/internal/model"
)

type Sender interface {
	Send(ctx context.Context, event model.Event) error
}

type NotificationService struct {
	sender Sender

	dedupMu     sync.Mutex
	dedupWindow time.Duration
	dedupSentAt map[string]time.Time
	nowFn       func() time.Time
}

const (
	ChannelEmail    = "email"
	ChannelTelegram = "telegram"

	defaultDedupWindow = 24 * time.Hour
)

func New(sender Sender) *NotificationService {
	return &NotificationService{
		sender:      sender,
		dedupWindow: defaultDedupWindow,
		dedupSentAt: make(map[string]time.Time),
		nowFn:       time.Now,
	}
}

func (s *NotificationService) SetDedupWindow(window time.Duration) *NotificationService {
	if s == nil {
		return nil
	}

	if window <= 0 {
		window = defaultDedupWindow
	}

	s.dedupMu.Lock()
	s.dedupWindow = window
	s.dedupMu.Unlock()

	return s
}

func (s *NotificationService) Health(_ context.Context) error {
	if s == nil || s.sender == nil {
		return errors.New("sender is not configured")
	}

	return nil
}

func (s *NotificationService) Send(ctx context.Context, event model.Event) error {
	if s == nil || s.sender == nil {
		return errors.New("sender is not configured")
	}

	if strings.TrimSpace(event.OrderID) == "" {
		return errors.New("order_id is required")
	}

	if strings.TrimSpace(event.Status) == "" {
		return errors.New("status is required")
	}

	if strings.TrimSpace(event.Channel) == "" {
		return errors.New("channel is required")
	}

	normalizedChannel, err := normalizeChannel(event.Channel)
	if err != nil {
		return err
	}
	event.Channel = normalizedChannel

	if strings.TrimSpace(event.Recipient) == "" {
		return errors.New("recipient is required")
	}

	if strings.TrimSpace(event.Message) == "" {
		return errors.New("message is required")
	}

	if event.CreatedAt.IsZero() {
		event.CreatedAt = s.now().UTC()
	}

	dedupKey := buildDedupKey(event)
	now := s.now().UTC()
	if !s.reserveDedupKey(dedupKey, now) {
		return nil
	}

	err = s.sender.Send(ctx, event)
	if err != nil {
		s.releaseDedupKey(dedupKey)
		return err
	}

	return nil
}

func normalizeChannel(channel string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(channel))
	switch normalized {
	case ChannelEmail, ChannelTelegram:
		return normalized, nil
	default:
		return "", fmt.Errorf("channel must be one of: %s, %s", ChannelEmail, ChannelTelegram)
	}
}

func (s *NotificationService) now() time.Time {
	if s != nil && s.nowFn != nil {
		return s.nowFn()
	}

	return time.Now()
}

func (s *NotificationService) reserveDedupKey(key string, now time.Time) bool {
	if s == nil {
		return true
	}

	window := s.dedupWindow
	if window <= 0 {
		window = defaultDedupWindow
	}

	s.dedupMu.Lock()
	defer s.dedupMu.Unlock()

	s.cleanupExpiredDedup(now, window)

	if sentAt, ok := s.dedupSentAt[key]; ok {
		if now.Sub(sentAt) < window {
			return false
		}
	}

	s.dedupSentAt[key] = now
	return true
}

func (s *NotificationService) releaseDedupKey(key string) {
	if s == nil {
		return
	}

	s.dedupMu.Lock()
	delete(s.dedupSentAt, key)
	s.dedupMu.Unlock()
}

func (s *NotificationService) cleanupExpiredDedup(now time.Time, window time.Duration) {
	if s == nil || len(s.dedupSentAt) == 0 {
		return
	}

	for key, sentAt := range s.dedupSentAt {
		if now.Sub(sentAt) >= window {
			delete(s.dedupSentAt, key)
		}
	}
}

func buildDedupKey(event model.Event) string {
	orderID := strings.ToLower(strings.TrimSpace(event.OrderID))
	status := strings.ToLower(strings.TrimSpace(event.Status))
	channel := strings.ToLower(strings.TrimSpace(event.Channel))
	recipient := strings.ToLower(strings.TrimSpace(event.Recipient))

	return orderID + "|" + status + "|" + channel + "|" + recipient
}
