package sender

import (
	"context"
	"log"

	"trackflow/services/notification-service/internal/model"
)

type MockSender struct {
	logger   *log.Logger
	provider string
	apiKey   string
}

func NewMockSender(logger *log.Logger, provider, apiKey string) *MockSender {
	if logger == nil {
		logger = log.Default()
	}

	return &MockSender{
		logger:   logger,
		provider: provider,
		apiKey:   apiKey,
	}
}

func (s *MockSender) Send(ctx context.Context, event model.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.logger.Printf(
		"notification sent: provider=%s order_id=%s status=%s channel=%s recipient=%s message=%s api_key_set=%t",
		s.provider,
		event.OrderID,
		event.Status,
		event.Channel,
		event.Recipient,
		event.Message,
		s.apiKey != "",
	)

	return nil
}
