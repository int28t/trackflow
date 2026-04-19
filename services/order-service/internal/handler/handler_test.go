package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"trackflow/services/order-service/internal/model"
	"trackflow/services/order-service/internal/service"
)

type idempotencyLookupResult struct {
	order model.Order
	err   error
}

type handlerRepoStub struct {
	pingErr error

	listOrdersItems []model.Order
	listOrdersErr   error

	getOrder model.Order
	getErr   error

	assignOrder model.Order
	assignErr   error

	createOrder model.Order
	createErr   error

	idempotencyResults []idempotencyLookupResult
	idempotencyCalls   int
}

func (r *handlerRepoStub) Ping(context.Context) error {
	return r.pingErr
}

func (r *handlerRepoStub) ListOrders(context.Context, int) ([]model.Order, error) {
	if r.listOrdersErr != nil {
		return nil, r.listOrdersErr
	}

	items := make([]model.Order, len(r.listOrdersItems))
	copy(items, r.listOrdersItems)
	return items, nil
}

func (r *handlerRepoStub) GetOrderByID(context.Context, string) (model.Order, error) {
	if r.getErr != nil {
		return model.Order{}, r.getErr
	}

	return r.getOrder, nil
}

func (r *handlerRepoStub) AssignOrder(context.Context, string, model.AssignOrderInput) (model.Order, error) {
	if r.assignErr != nil {
		return model.Order{}, r.assignErr
	}

	return r.assignOrder, nil
}

func (r *handlerRepoStub) CreateOrder(context.Context, model.CreateOrderInput, string) (model.Order, error) {
	if r.createErr != nil {
		return model.Order{}, r.createErr
	}

	return r.createOrder, nil
}

func (r *handlerRepoStub) GetOrderByIdempotencyKey(context.Context, string) (model.Order, error) {
	r.idempotencyCalls++
	if len(r.idempotencyResults) == 0 {
		return model.Order{}, service.ErrOrderNotFound
	}

	index := r.idempotencyCalls - 1
	if index >= len(r.idempotencyResults) {
		index = len(r.idempotencyResults) - 1
	}

	res := r.idempotencyResults[index]
	return res.order, res.err
}

func TestHealthReturnsOK(t *testing.T) {
	t.Parallel()

	h := newHTTPHandler(&handlerRepoStub{})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusOK)
	}
}

func TestListOrdersReturnsItems(t *testing.T) {
	t.Parallel()

	h := newHTTPHandler(&handlerRepoStub{
		listOrdersItems: []model.Order{{
			ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			Status:     "created",
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/orders?limit=1", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusOK)
	}

	var payload listOrdersResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if len(payload.Items) != 1 {
		t.Fatalf("unexpected items count: got %d, want %d", len(payload.Items), 1)
	}
}

func TestListOrdersRejectsInvalidLimit(t *testing.T) {
	t.Parallel()

	h := newHTTPHandler(&handlerRepoStub{})

	req := httptest.NewRequest(http.MethodGet, "/v1/orders?limit=bad", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestCreateOrderRequiresIdempotencyHeader(t *testing.T) {
	t.Parallel()

	h := newHTTPHandler(&handlerRepoStub{})

	req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewBufferString(validCreateOrderJSON()))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusBadRequest)
	}
}

func TestCreateOrderReturnsCreated(t *testing.T) {
	t.Parallel()

	repo := &handlerRepoStub{
		idempotencyResults: []idempotencyLookupResult{{err: service.ErrOrderNotFound}},
		createOrder: model.Order{
			ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			Status:     "created",
		},
	}
	h := newHTTPHandler(repo)

	req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewBufferString(validCreateOrderJSON()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "idem-1")

	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusCreated)
	}

	var payload createOrderResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if payload.OrderID == "" {
		t.Fatal("expected non-empty order_id")
	}
}

func TestGetOrderByIDReturnsNotFound(t *testing.T) {
	t.Parallel()

	h := newHTTPHandler(&handlerRepoStub{getErr: service.ErrOrderNotFound})

	req := httptest.NewRequest(http.MethodGet, "/v1/orders/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusNotFound)
	}
}

func TestAssignOrderMapsConflictError(t *testing.T) {
	t.Parallel()

	h := newHTTPHandler(&handlerRepoStub{assignErr: service.ErrAssignmentNotAllowed})

	req := httptest.NewRequest(http.MethodPost, "/v1/orders/aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa/assign", bytes.NewBufferString(validAssignOrderJSON()))
	req.Header.Set("Content-Type", "application/json")

	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusConflict {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusConflict)
	}
}

func TestMetricsEndpointReturnsSnapshot(t *testing.T) {
	t.Parallel()

	h := newHTTPHandler(&handlerRepoStub{})

	// Warm up one request so snapshot is non-empty.
	warmupReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	warmupRes := httptest.NewRecorder()
	h.ServeHTTP(warmupRes, warmupReq)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusOK)
	}

	if contentType := res.Header().Get("Content-Type"); contentType == "" {
		t.Fatal("expected content type for metrics response")
	}

	var payload map[string]any
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode metrics response failed: %v", err)
	}

	if _, ok := payload["requests_total"]; !ok {
		t.Fatal("expected requests_total in metrics payload")
	}
}

func newHTTPHandler(repo *handlerRepoStub) http.Handler {
	if repo == nil {
		repo = &handlerRepoStub{}
	}

	logger := log.New(io.Discard, "", 0)
	svc := service.New(repo)
	return New(logger, svc)
}

func validCreateOrderJSON() string {
	return `{
		"customer_id":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		"pickup_address":{
			"city":"Moscow",
			"street":"Tverskaya",
			"house":"1",
			"apartment":"10",
			"lat":55.7558,
			"lng":37.6176
		},
		"dropoff_address":{
			"city":"Moscow",
			"street":"Arbat",
			"house":"5",
			"apartment":"12",
			"lat":55.7520,
			"lng":37.5929
		},
		"weight_kg":1.5,
		"distance_km":3.2,
		"service_level":"standard"
	}`
}

func validAssignOrderJSON() string {
	return `{
		"courier_id":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		"assigned_by":"manager",
		"comment":"manual assignment"
	}`
}
