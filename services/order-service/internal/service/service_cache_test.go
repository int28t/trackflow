package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"trackflow/services/order-service/internal/model"
)

type repositoryStub struct {
	getOrderByIDCalls int
	getOrderByIDOrder model.Order
	getOrderByIDErr   error
}

func (r *repositoryStub) Ping(context.Context) error {
	return nil
}

func (r *repositoryStub) ListOrders(context.Context, int) ([]model.Order, error) {
	return nil, nil
}

func (r *repositoryStub) GetOrderByID(_ context.Context, _ string) (model.Order, error) {
	r.getOrderByIDCalls++
	if r.getOrderByIDErr != nil {
		return model.Order{}, r.getOrderByIDErr
	}

	return r.getOrderByIDOrder, nil
}

func (r *repositoryStub) AssignOrder(context.Context, string, model.AssignOrderInput) (model.Order, error) {
	return model.Order{}, nil
}

func (r *repositoryStub) CreateOrder(context.Context, model.CreateOrderInput, string) (model.Order, error) {
	return model.Order{}, nil
}

func (r *repositoryStub) GetOrderByIdempotencyKey(context.Context, string) (model.Order, error) {
	return model.Order{}, ErrOrderNotFound
}

type cacheStub struct {
	getCalls int
	getOrder model.Order
	getFound bool
	getErr   error

	setCalls int
	setOrder model.Order
	setTTL   time.Duration
	setErr   error
}

func (c *cacheStub) GetOrderByID(context.Context, string) (model.Order, bool, error) {
	c.getCalls++
	return c.getOrder, c.getFound, c.getErr
}

func (c *cacheStub) SetOrder(_ context.Context, order model.Order, ttl time.Duration) error {
	c.setCalls++
	c.setOrder = order
	c.setTTL = ttl
	return c.setErr
}

func TestGetOrderByIDReturnsCacheHitWithoutRepositoryCall(t *testing.T) {
	t.Parallel()

	repo := &repositoryStub{}
	cachedOrder := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "created",
	}
	cache := &cacheStub{
		getOrder: cachedOrder,
		getFound: true,
	}

	svc := New(repo).SetCache(cache, time.Minute)

	order, err := svc.GetOrderByID(context.Background(), cachedOrder.ID)
	if err != nil {
		t.Fatalf("GetOrderByID returned error: %v", err)
	}

	if order != cachedOrder {
		t.Fatalf("unexpected order from cache: got %+v, want %+v", order, cachedOrder)
	}

	if repo.getOrderByIDCalls != 0 {
		t.Fatalf("repository should not be called on cache hit, got %d calls", repo.getOrderByIDCalls)
	}
}

func TestGetOrderByIDCachesRepositoryResultOnMiss(t *testing.T) {
	t.Parallel()

	repoOrder := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "assigned",
	}
	repo := &repositoryStub{getOrderByIDOrder: repoOrder}
	cache := &cacheStub{}

	expectedTTL := 2 * time.Minute
	svc := New(repo).SetCache(cache, expectedTTL)

	order, err := svc.GetOrderByID(context.Background(), repoOrder.ID)
	if err != nil {
		t.Fatalf("GetOrderByID returned error: %v", err)
	}

	if order != repoOrder {
		t.Fatalf("unexpected repository order: got %+v, want %+v", order, repoOrder)
	}

	if repo.getOrderByIDCalls != 1 {
		t.Fatalf("expected repository call count 1, got %d", repo.getOrderByIDCalls)
	}

	if cache.setCalls != 1 {
		t.Fatalf("expected cache write count 1, got %d", cache.setCalls)
	}

	if cache.setOrder != repoOrder {
		t.Fatalf("unexpected cache write order: got %+v, want %+v", cache.setOrder, repoOrder)
	}

	if cache.setTTL != expectedTTL {
		t.Fatalf("unexpected cache TTL: got %s, want %s", cache.setTTL, expectedTTL)
	}
}

func TestGetOrderByIDFallsBackToRepositoryWhenCacheFails(t *testing.T) {
	t.Parallel()

	repoOrder := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "in_transit",
	}
	repo := &repositoryStub{getOrderByIDOrder: repoOrder}
	cache := &cacheStub{getErr: errors.New("redis timeout")}

	svc := New(repo).SetCache(cache, time.Minute)

	order, err := svc.GetOrderByID(context.Background(), repoOrder.ID)
	if err != nil {
		t.Fatalf("GetOrderByID returned error: %v", err)
	}

	if order != repoOrder {
		t.Fatalf("unexpected repository fallback order: got %+v, want %+v", order, repoOrder)
	}

	if repo.getOrderByIDCalls != 1 {
		t.Fatalf("expected repository call count 1 after cache error, got %d", repo.getOrderByIDCalls)
	}
}
