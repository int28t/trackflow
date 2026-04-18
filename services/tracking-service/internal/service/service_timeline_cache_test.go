package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"trackflow/services/tracking-service/internal/model"
)

type timelineRepositoryStub struct {
	timelineItems []model.StatusHistoryItem
	timelineErr   error
	timelineCalls int
	timelineLimit int

	updateResult model.StatusHistoryItem
	updateErr    error
	updateCalls  int
}

func (r *timelineRepositoryStub) Ping(context.Context) error {
	return nil
}

func (r *timelineRepositoryStub) GetOrderTimeline(_ context.Context, _ string, limit int) ([]model.StatusHistoryItem, error) {
	r.timelineCalls++
	r.timelineLimit = limit
	if r.timelineErr != nil {
		return nil, r.timelineErr
	}

	items := make([]model.StatusHistoryItem, len(r.timelineItems))
	copy(items, r.timelineItems)
	return items, nil
}

func (r *timelineRepositoryStub) UpdateOrderStatus(context.Context, string, string, string, string, []byte) (model.StatusHistoryItem, error) {
	r.updateCalls++
	if r.updateErr != nil {
		return model.StatusHistoryItem{}, r.updateErr
	}

	return r.updateResult, nil
}

type timelineCacheStub struct {
	getItems []model.StatusHistoryItem
	getFound bool
	getErr   error
	getCalls int

	setCalls   int
	setOrderID string
	setItems   []model.StatusHistoryItem
	setTTL     time.Duration
	setErr     error

	deleteCalls   int
	deleteOrderID string
	deleteErr     error
}

func (c *timelineCacheStub) GetTimeline(context.Context, string) ([]model.StatusHistoryItem, bool, error) {
	c.getCalls++
	items := make([]model.StatusHistoryItem, len(c.getItems))
	copy(items, c.getItems)
	return items, c.getFound, c.getErr
}

func (c *timelineCacheStub) SetTimeline(_ context.Context, orderID string, items []model.StatusHistoryItem, ttl time.Duration) error {
	c.setCalls++
	c.setOrderID = orderID
	c.setTTL = ttl
	c.setItems = make([]model.StatusHistoryItem, len(items))
	copy(c.setItems, items)
	return c.setErr
}

func (c *timelineCacheStub) DeleteTimeline(_ context.Context, orderID string) error {
	c.deleteCalls++
	c.deleteOrderID = orderID
	return c.deleteErr
}

type orderCacheInvalidatorStub struct {
	deleteCalls   int
	deleteOrderID string
	deleteErr     error
}

func (c *orderCacheInvalidatorStub) DeleteOrder(_ context.Context, orderID string) error {
	c.deleteCalls++
	c.deleteOrderID = orderID
	return c.deleteErr
}

func TestGetOrderTimelineReturnsCacheHit(t *testing.T) {
	t.Parallel()

	cached := []model.StatusHistoryItem{
		{ID: "1", OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Status: "created", Source: "system"},
		{ID: "2", OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Status: "assigned", Source: "manager"},
		{ID: "3", OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Status: "in_transit", Source: "courier"},
	}
	repo := &timelineRepositoryStub{}
	cache := &timelineCacheStub{getItems: cached, getFound: true}

	svc := New(repo).SetTimelineCache(cache, 15*time.Second)

	items, err := svc.GetOrderTimeline(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", 2)
	if err != nil {
		t.Fatalf("GetOrderTimeline returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("unexpected item count: got %d, want %d", len(items), 2)
	}

	if repo.timelineCalls != 0 {
		t.Fatalf("repository should not be called on cache hit, got %d calls", repo.timelineCalls)
	}
}

func TestGetOrderTimelineCachesRepositoryResultOnMiss(t *testing.T) {
	t.Parallel()

	repoItems := []model.StatusHistoryItem{
		{ID: "1", OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Status: "created", Source: "system"},
		{ID: "2", OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Status: "assigned", Source: "manager"},
		{ID: "3", OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Status: "in_transit", Source: "courier"},
	}
	repo := &timelineRepositoryStub{timelineItems: repoItems}
	cache := &timelineCacheStub{}

	ttl := 10 * time.Second
	svc := New(repo).SetTimelineCache(cache, ttl)

	items, err := svc.GetOrderTimeline(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", 2)
	if err != nil {
		t.Fatalf("GetOrderTimeline returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("unexpected item count: got %d, want %d", len(items), 2)
	}

	if repo.timelineCalls != 1 {
		t.Fatalf("expected repository calls 1, got %d", repo.timelineCalls)
	}

	if repo.timelineLimit != maxTimelineLimit {
		t.Fatalf("expected repository limit %d, got %d", maxTimelineLimit, repo.timelineLimit)
	}

	if cache.setCalls != 1 {
		t.Fatalf("expected cache write calls 1, got %d", cache.setCalls)
	}

	if cache.setTTL != ttl {
		t.Fatalf("unexpected cache TTL: got %s, want %s", cache.setTTL, ttl)
	}

	if len(cache.setItems) != len(repoItems) {
		t.Fatalf("unexpected cached item count: got %d, want %d", len(cache.setItems), len(repoItems))
	}
}

func TestGetOrderTimelineFallsBackWhenCacheReadFails(t *testing.T) {
	t.Parallel()

	repoItems := []model.StatusHistoryItem{
		{ID: "1", OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", Status: "created", Source: "system"},
	}
	repo := &timelineRepositoryStub{timelineItems: repoItems}
	cache := &timelineCacheStub{getErr: errors.New("redis timeout")}

	svc := New(repo).SetTimelineCache(cache, 15*time.Second)

	items, err := svc.GetOrderTimeline(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", 50)
	if err != nil {
		t.Fatalf("GetOrderTimeline returned error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("unexpected item count: got %d, want %d", len(items), 1)
	}

	if repo.timelineCalls != 1 {
		t.Fatalf("expected repository calls 1, got %d", repo.timelineCalls)
	}
}

func TestUpdateOrderStatusInvalidatesTimelineAndOrderCaches(t *testing.T) {
	t.Parallel()

	repo := &timelineRepositoryStub{
		updateResult: model.StatusHistoryItem{
			ID:      "history-1",
			OrderID: "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Status:  "assigned",
			Source:  "manager",
		},
	}
	timelineCache := &timelineCacheStub{}
	orderCache := &orderCacheInvalidatorStub{}

	svc := New(repo).
		SetTimelineCache(timelineCache, 15*time.Second).
		SetOrderCacheInvalidator(orderCache)

	_, err := svc.UpdateOrderStatus(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", model.UpdateStatusInput{
		Status: "assigned",
		Source: "manager",
	})
	if err != nil {
		t.Fatalf("UpdateOrderStatus returned error: %v", err)
	}

	if timelineCache.deleteCalls != 1 {
		t.Fatalf("expected timeline cache delete calls 1, got %d", timelineCache.deleteCalls)
	}

	if timelineCache.deleteOrderID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("unexpected timeline cache delete order_id: got %q", timelineCache.deleteOrderID)
	}

	if orderCache.deleteCalls != 1 {
		t.Fatalf("expected order cache delete calls 1, got %d", orderCache.deleteCalls)
	}

	if orderCache.deleteOrderID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
		t.Fatalf("unexpected order cache delete order_id: got %q", orderCache.deleteOrderID)
	}
}
