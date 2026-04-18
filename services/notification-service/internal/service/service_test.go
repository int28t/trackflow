package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"trackflow/services/notification-service/internal/model"
)

type stubSender struct {
	calls  int
	last   model.Event
	failAt map[int]error
}

func (s *stubSender) Send(_ context.Context, event model.Event) error {
	s.calls++
	s.last = event

	if s.failAt != nil {
		if err, ok := s.failAt[s.calls]; ok {
			return err
		}
	}

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

func TestSendDeduplicatesNotifications(t *testing.T) {
	t.Parallel()

	sender := &stubSender{}
	svc := New(sender)

	event := model.Event{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:    "in_transit",
		Channel:   "email",
		Recipient: "user@example.com",
		Message:   "status changed",
	}

	if err := svc.Send(context.Background(), event); err != nil {
		t.Fatalf("first Send returned error: %v", err)
	}

	if err := svc.Send(context.Background(), event); err != nil {
		t.Fatalf("second Send returned error: %v", err)
	}

	if sender.calls != 1 {
		t.Fatalf("expected deduplicated send count to be 1, got %d", sender.calls)
	}
}

func TestSendDedupWindowExpiryAllowsResend(t *testing.T) {
	t.Parallel()

	sender := &stubSender{}
	svc := New(sender).SetDedupWindow(time.Second)

	current := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	svc.nowFn = func() time.Time { return current }

	event := model.Event{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:    "assigned",
		Channel:   "telegram",
		Recipient: "@trackflow_user",
		Message:   "status changed",
	}

	if err := svc.Send(context.Background(), event); err != nil {
		t.Fatalf("first Send returned error: %v", err)
	}

	current = current.Add(500 * time.Millisecond)
	if err := svc.Send(context.Background(), event); err != nil {
		t.Fatalf("second Send returned error: %v", err)
	}

	current = current.Add(2 * time.Second)
	if err := svc.Send(context.Background(), event); err != nil {
		t.Fatalf("third Send returned error: %v", err)
	}

	if sender.calls != 2 {
		t.Fatalf("expected send count 2 after dedup expiry, got %d", sender.calls)
	}
}

func TestSendDedupRollbackOnSenderFailure(t *testing.T) {
	t.Parallel()

	sender := &stubSender{
		failAt: map[int]error{1: errors.New("temporary sender failure")},
	}
	svc := New(sender)

	event := model.Event{
		OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		Status:    "delivered",
		Channel:   "email",
		Recipient: "user@example.com",
		Message:   "status changed",
	}

	err := svc.Send(context.Background(), event)
	if err == nil {
		t.Fatal("expected first Send to fail")
	}

	err = svc.Send(context.Background(), event)
	if err != nil {
		t.Fatalf("expected second Send to succeed, got: %v", err)
	}

	if sender.calls != 2 {
		t.Fatalf("expected send calls to be 2 after retry, got %d", sender.calls)
	}
}
