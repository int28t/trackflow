package sender

import (
	"context"
	"errors"
	"io"
	"log"
	"testing"

	"trackflow/services/notification-service/internal/model"
)

func TestNewMockSenderCreatesSenderWithNilLogger(t *testing.T) {
	t.Parallel()

	s := NewMockSender(nil, "mock", "token")
	if s == nil {
		t.Fatal("expected non-nil sender")
	}
}

func TestMockSenderSendSuccess(t *testing.T) {
	t.Parallel()

	s := NewMockSender(log.New(io.Discard, "", 0), "mock", "token")
	err := s.Send(context.Background(), model.Event{
		OrderID:   "order-1",
		Status:    "created",
		Channel:   "email",
		Recipient: "user@example.com",
		Message:   "created",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
}

func TestMockSenderSendReturnsContextError(t *testing.T) {
	t.Parallel()

	s := NewMockSender(log.New(io.Discard, "", 0), "mock", "token")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := s.Send(ctx, model.Event{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
