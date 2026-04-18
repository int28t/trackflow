package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"trackflow/services/tracking-service/internal/model"
)

type stubRepository struct {
	updateResult model.StatusHistoryItem
	updateErr    error
	updateCalls  int
}

func (r *stubRepository) Ping(context.Context) error {
	return nil
}

func (r *stubRepository) GetOrderTimeline(context.Context, string, int) ([]model.StatusHistoryItem, error) {
	return nil, nil
}

func (r *stubRepository) UpdateOrderStatus(context.Context, string, string, string, string, []byte) (model.StatusHistoryItem, error) {
	r.updateCalls++
	if r.updateErr != nil {
		return model.StatusHistoryItem{}, r.updateErr
	}

	return r.updateResult, nil
}

type stubNotifier struct {
	calls int
	err   error
}

func (n *stubNotifier) NotifyStatusChanged(context.Context, model.StatusHistoryItem) error {
	n.calls++
	return n.err
}

func TestUpdateOrderStatusDispatchesNotification(t *testing.T) {
	t.Parallel()

	repo := &stubRepository{
		updateResult: model.StatusHistoryItem{
			OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Status:    "assigned",
			Source:    "manager",
			CreatedAt: time.Now().UTC(),
		},
	}
	notifier := &stubNotifier{}

	svc := New(repo).SetNotifier(notifier)
	item, err := svc.UpdateOrderStatus(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", model.UpdateStatusInput{
		Status: "assigned",
		Source: "manager",
	})
	if err != nil {
		t.Fatalf("UpdateOrderStatus returned error: %v", err)
	}

	if item.Status != "assigned" {
		t.Fatalf("unexpected status: %s", item.Status)
	}

	if repo.updateCalls != 1 {
		t.Fatalf("unexpected repo update calls: got %d, want %d", repo.updateCalls, 1)
	}

	if notifier.calls != 1 {
		t.Fatalf("unexpected notifier calls: got %d, want %d", notifier.calls, 1)
	}
}

func TestUpdateOrderStatusIgnoresNotifierFailure(t *testing.T) {
	t.Parallel()

	repo := &stubRepository{
		updateResult: model.StatusHistoryItem{
			OrderID:   "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Status:    "delivered",
			Source:    "carrier_sync",
			CreatedAt: time.Now().UTC(),
		},
	}
	notifier := &stubNotifier{err: errors.New("notification unavailable")}

	svc := New(repo).SetNotifier(notifier)
	_, err := svc.UpdateOrderStatus(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", model.UpdateStatusInput{
		Status: "delivered",
		Source: "carrier_sync",
	})
	if err != nil {
		t.Fatalf("notification failure must not fail status update, got error: %v", err)
	}

	if notifier.calls != 1 {
		t.Fatalf("unexpected notifier calls: got %d, want %d", notifier.calls, 1)
	}
}
