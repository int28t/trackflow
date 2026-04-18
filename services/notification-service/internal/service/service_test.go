package service

import (
	"context"
	"testing"

	"trackflow/services/notification-service/internal/model"
)

type stubSender struct {
	calls int
	last  model.Event
}

func (s *stubSender) Send(_ context.Context, event model.Event) error {
	s.calls++
	s.last = event
	return nil
}

func TestSendNormalizesEmailChannel(t *testing.T) {
	t.Parallel()

	sender := &stubSender{}
	svc := New(sender)

	err := svc.Send(context.Background(), model.Event{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:    "in_transit",
		Channel:   " Email ",
		Recipient: "user@example.com",
		Message:   "status changed",
	})
	if err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	if sender.calls != 1 {
		t.Fatalf("unexpected sender calls: got %d, want %d", sender.calls, 1)
	}

	if sender.last.Channel != ChannelEmail {
		t.Fatalf("unexpected normalized channel: got %q, want %q", sender.last.Channel, ChannelEmail)
	}
}

func TestSendRejectsUnsupportedChannel(t *testing.T) {
	t.Parallel()

	svc := New(&stubSender{})
	err := svc.Send(context.Background(), model.Event{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:    "in_transit",
		Channel:   "sms",
		Recipient: "user@example.com",
		Message:   "status changed",
	})
	if err == nil {
		t.Fatal("expected error for unsupported channel")
	}
}

func TestSendRejectsMissingStatus(t *testing.T) {
	t.Parallel()

	svc := New(&stubSender{})
	err := svc.Send(context.Background(), model.Event{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Channel:   "email",
		Recipient: "user@example.com",
		Message:   "status changed",
	})
	if err == nil {
		t.Fatal("expected error for missing status")
	}
}
