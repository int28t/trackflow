package observability

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"trackflow/services/tracking-service/internal/requestid"
)

func TestMiddlewareAndHandlerCollectMetrics(t *testing.T) {
	t.Parallel()

	metrics := NewHTTPMetrics(log.New(io.Discard, "", 0))

	h := metrics.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/orders/1/timeline", nil)
	req = req.WithContext(requestid.WithRequestID(req.Context(), "req-1"))
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if res.Code != http.StatusCreated {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusCreated)
	}

	snapshotReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	snapshotRes := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(snapshotRes, snapshotReq)

	if snapshotRes.Code != http.StatusOK {
		t.Fatalf("unexpected metrics status: got %d, want %d", snapshotRes.Code, http.StatusOK)
	}

	var snapshot metricsSnapshot
	if err := json.NewDecoder(snapshotRes.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode metrics snapshot failed: %v", err)
	}

	if snapshot.RequestsTotal != 1 {
		t.Fatalf("unexpected requests_total: got %d, want %d", snapshot.RequestsTotal, 1)
	}

	if len(snapshot.Routes) != 1 {
		t.Fatalf("unexpected routes count: got %d, want %d", len(snapshot.Routes), 1)
	}

	if snapshot.Routes[0].Status != http.StatusCreated {
		t.Fatalf("unexpected route status: got %d, want %d", snapshot.Routes[0].Status, http.StatusCreated)
	}
}

func TestMetricsHandlerMethodNotAllowed(t *testing.T) {
	t.Parallel()

	metrics := NewHTTPMetrics(log.New(io.Discard, "", 0))

	req := httptest.NewRequest(http.MethodPost, "/metrics", nil)
	res := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(res, req)

	if res.Code != http.StatusMethodNotAllowed {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusMethodNotAllowed)
	}
}

func TestRoutePatternAndHelpers(t *testing.T) {
	t.Parallel()

	if got := routePattern(nil); got != "unknown" {
		t.Fatalf("unexpected route for nil request: got %q", got)
	}

	req := httptest.NewRequest(http.MethodGet, "/plain-path", nil)
	req.Pattern = "/v1/orders/{order_id}/timeline"
	if got := routePattern(req); got != "/v1/orders/{order_id}/timeline" {
		t.Fatalf("unexpected route pattern: got %q", got)
	}

	if got := averageLatencyMs(time.Second, 0); got != 0 {
		t.Fatalf("unexpected average latency for empty count: got %f", got)
	}

	if got := latencyMs(1500 * time.Microsecond); got <= 0 {
		t.Fatalf("expected positive latencyMs, got %f", got)
	}
}
