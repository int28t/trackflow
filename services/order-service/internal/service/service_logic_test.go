package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"trackflow/services/order-service/internal/model"
)

type idempotencyResult struct {
	order model.Order
	err   error
}

type repoBehaviorStub struct {
	pingErr error

	listOrdersCalls int
	listOrdersLimit int
	listOrdersItems []model.Order
	listOrdersErr   error

	getOrderByIDCalls int
	getOrderByIDErr   error
	getOrderByIDOrder model.Order

	assignCalls int
	assignID    string
	assignInput model.AssignOrderInput
	assignOrder model.Order
	assignErr   error

	createCalls int
	createInput model.CreateOrderInput
	createKey   string
	createOrder model.Order
	createErr   error

	getByIdempotencyCalls   int
	getByIdempotencyResults []idempotencyResult
}

func (r *repoBehaviorStub) Ping(context.Context) error {
	return r.pingErr
}

func (r *repoBehaviorStub) ListOrders(_ context.Context, limit int) ([]model.Order, error) {
	r.listOrdersCalls++
	r.listOrdersLimit = limit
	if r.listOrdersErr != nil {
		return nil, r.listOrdersErr
	}

	items := make([]model.Order, len(r.listOrdersItems))
	copy(items, r.listOrdersItems)
	return items, nil
}

func (r *repoBehaviorStub) GetOrderByID(context.Context, string) (model.Order, error) {
	r.getOrderByIDCalls++
	if r.getOrderByIDErr != nil {
		return model.Order{}, r.getOrderByIDErr
	}

	return r.getOrderByIDOrder, nil
}

func (r *repoBehaviorStub) AssignOrder(_ context.Context, orderID string, input model.AssignOrderInput) (model.Order, error) {
	r.assignCalls++
	r.assignID = orderID
	r.assignInput = input
	if r.assignErr != nil {
		return model.Order{}, r.assignErr
	}

	return r.assignOrder, nil
}

func (r *repoBehaviorStub) CreateOrder(_ context.Context, input model.CreateOrderInput, idempotencyKey string) (model.Order, error) {
	r.createCalls++
	r.createInput = input
	r.createKey = idempotencyKey
	if r.createErr != nil {
		return model.Order{}, r.createErr
	}

	return r.createOrder, nil
}

func (r *repoBehaviorStub) GetOrderByIdempotencyKey(context.Context, string) (model.Order, error) {
	r.getByIdempotencyCalls++

	if len(r.getByIdempotencyResults) == 0 {
		return model.Order{}, ErrOrderNotFound
	}

	index := r.getByIdempotencyCalls - 1
	if index >= len(r.getByIdempotencyResults) {
		index = len(r.getByIdempotencyResults) - 1
	}

	result := r.getByIdempotencyResults[index]
	return result.order, result.err
}

func TestListOrdersNormalizesLimit(t *testing.T) {
	t.Parallel()

	t.Run("default", func(t *testing.T) {
		t.Parallel()

		repo := &repoBehaviorStub{}
		svc := New(repo)

		_, err := svc.ListOrders(context.Background(), 0)
		if err != nil {
			t.Fatalf("ListOrders returned error: %v", err)
		}

		if repo.listOrdersLimit != defaultListLimit {
			t.Fatalf("unexpected normalized limit: got %d, want %d", repo.listOrdersLimit, defaultListLimit)
		}
	})

	t.Run("max", func(t *testing.T) {
		t.Parallel()

		repo := &repoBehaviorStub{}
		svc := New(repo)

		_, err := svc.ListOrders(context.Background(), maxListLimit+1)
		if err != nil {
			t.Fatalf("ListOrders returned error: %v", err)
		}

		if repo.listOrdersLimit != maxListLimit {
			t.Fatalf("unexpected normalized limit: got %d, want %d", repo.listOrdersLimit, maxListLimit)
		}
	})
}

func TestGetOrderByIDRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	svc := New(&repoBehaviorStub{})
	_, err := svc.GetOrderByID(context.Background(), "not-a-uuid")
	if err == nil {
		t.Fatal("expected validation error for invalid UUID")
	}

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAssignOrderNormalizesInputAndCaches(t *testing.T) {
	t.Parallel()

	orderID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	courierID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

	repo := &repoBehaviorStub{
		assignOrder: model.Order{
			ID:         orderID,
			CustomerID: "cccccccc-cccc-cccc-cccc-cccccccccccc",
			Status:     "assigned",
		},
	}
	cache := &cacheStub{}

	svc := New(repo).SetCache(cache, time.Minute)

	order, err := svc.AssignOrder(context.Background(), "  "+orderID+"  ", model.AssignOrderInput{
		CourierID:  "  " + courierID + "  ",
		AssignedBy: "  manager  ",
		Comment:    "  first assignment  ",
	})
	if err != nil {
		t.Fatalf("AssignOrder returned error: %v", err)
	}

	if order.ID != orderID {
		t.Fatalf("unexpected assigned order id: got %q, want %q", order.ID, orderID)
	}

	if repo.assignID != orderID {
		t.Fatalf("unexpected normalized order id: got %q, want %q", repo.assignID, orderID)
	}

	if repo.assignInput.CourierID != courierID {
		t.Fatalf("unexpected normalized courier id: got %q, want %q", repo.assignInput.CourierID, courierID)
	}

	if repo.assignInput.AssignedBy != "manager" {
		t.Fatalf("unexpected normalized assigned_by: got %q", repo.assignInput.AssignedBy)
	}

	if repo.assignInput.Comment != "first assignment" {
		t.Fatalf("unexpected normalized comment: got %q", repo.assignInput.Comment)
	}

	if cache.setCalls != 1 {
		t.Fatalf("expected cache set call count 1, got %d", cache.setCalls)
	}
}

func TestCreateOrderIdempotencyHitReturnsExisting(t *testing.T) {
	t.Parallel()

	existing := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "created",
	}

	repo := &repoBehaviorStub{
		getByIdempotencyResults: []idempotencyResult{{order: existing, err: nil}},
	}
	cache := &cacheStub{}
	svc := New(repo).SetCache(cache, time.Minute)

	order, created, err := svc.CreateOrder(context.Background(), validCreateInput(), "  idem-1  ")
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if created {
		t.Fatal("expected created=false on idempotency hit")
	}

	if order != existing {
		t.Fatalf("unexpected order: got %+v, want %+v", order, existing)
	}

	if repo.createCalls != 0 {
		t.Fatalf("expected create repository calls 0, got %d", repo.createCalls)
	}

	if cache.setCalls != 1 {
		t.Fatalf("expected cache write on idempotency hit, got %d", cache.setCalls)
	}
}

func TestCreateOrderCreatesWhenIdempotencyNotFound(t *testing.T) {
	t.Parallel()

	createdOrder := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "created",
	}

	repo := &repoBehaviorStub{
		getByIdempotencyResults: []idempotencyResult{{err: ErrOrderNotFound}},
		createOrder:             createdOrder,
	}
	cache := &cacheStub{}
	svc := New(repo).SetCache(cache, time.Minute)

	order, created, err := svc.CreateOrder(context.Background(), validCreateInputWithServiceLevel("  EXPRESS  "), "  idem-2  ")
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if !created {
		t.Fatal("expected created=true")
	}

	if order != createdOrder {
		t.Fatalf("unexpected created order: got %+v, want %+v", order, createdOrder)
	}

	if repo.createKey != "idem-2" {
		t.Fatalf("unexpected normalized idempotency key: got %q", repo.createKey)
	}

	if repo.createInput.ServiceLevel != "express" {
		t.Fatalf("unexpected normalized service level: got %q, want %q", repo.createInput.ServiceLevel, "express")
	}

	if cache.setCalls != 1 {
		t.Fatalf("expected cache set call count 1, got %d", cache.setCalls)
	}
}

func TestCreateOrderReturnsExistingOnDuplicateConflict(t *testing.T) {
	t.Parallel()

	existing := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "created",
	}

	repo := &repoBehaviorStub{
		getByIdempotencyResults: []idempotencyResult{
			{err: ErrOrderNotFound},
			{order: existing, err: nil},
		},
		createErr: ErrDuplicateIdempotency,
	}
	cache := &cacheStub{}
	svc := New(repo).SetCache(cache, time.Minute)

	order, created, err := svc.CreateOrder(context.Background(), validCreateInput(), "idem-3")
	if err != nil {
		t.Fatalf("CreateOrder returned error: %v", err)
	}

	if created {
		t.Fatal("expected created=false when duplicate conflict occurs")
	}

	if order != existing {
		t.Fatalf("unexpected order returned after duplicate conflict: got %+v, want %+v", order, existing)
	}

	if repo.getByIdempotencyCalls != 2 {
		t.Fatalf("expected 2 idempotency lookups, got %d", repo.getByIdempotencyCalls)
	}

	if cache.setCalls != 1 {
		t.Fatalf("expected cache set call count 1, got %d", cache.setCalls)
	}
}

func TestCreateOrderRejectsInvalidCustomerID(t *testing.T) {
	t.Parallel()

	svc := New(&repoBehaviorStub{})
	invalid := validCreateInput()
	invalid.CustomerID = ""

	_, _, err := svc.CreateOrder(context.Background(), invalid, "idem-4")
	if err == nil {
		t.Fatal("expected validation error")
	}

	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestHealthReturnsErrorForNilReceiver(t *testing.T) {
	t.Parallel()

	var svc *OrderService
	err := svc.Health(context.Background())
	if err == nil {
		t.Fatal("expected error for nil service")
	}
}

func validCreateInput() model.CreateOrderInput {
	return validCreateInputWithServiceLevel("")
}

func validCreateInputWithServiceLevel(level string) model.CreateOrderInput {
	return model.CreateOrderInput{
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		PickupAddress: model.AddressInput{
			City:      "Moscow",
			Street:    "Tverskaya",
			House:     "1",
			Apartment: "10",
			Lat:       55.7558,
			Lng:       37.6176,
		},
		DropoffAddress: model.AddressInput{
			City:      "Moscow",
			Street:    "Arbat",
			House:     "5",
			Apartment: "12",
			Lat:       55.7520,
			Lng:       37.5929,
		},
		WeightKG:     1.5,
		DistanceKM:   3.2,
		ServiceLevel: level,
	}
}
