package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"trackflow/services/notification-service/internal/model"
)

type Sender interface {
	Send(ctx context.Context, event model.Event) error
}

type NotificationService struct {
	sender Sender
}

const (
	ChannelEmail    = "email"
	ChannelTelegram = "telegram"
)

func New(sender Sender) *NotificationService {
	return &NotificationService{sender: sender}
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
		event.CreatedAt = time.Now().UTC()
	}

	return s.sender.Send(ctx, event)
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
