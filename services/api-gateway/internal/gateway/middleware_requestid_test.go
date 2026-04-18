package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddlewareUsesCorrelationIDHeader(t *testing.T) {
	t.Parallel()

	const expectedRequestID = "corr-123"

	var requestIDFromContext string
	handler := RequestID()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestIDFromContext = getRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/orders", nil)
	req.Header.Set(correlationIDHeader, expectedRequestID)

	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if requestIDFromContext != expectedRequestID {
		t.Fatalf("unexpected request id in context: got %q, want %q", requestIDFromContext, expectedRequestID)
	}

	if got := res.Header().Get(requestIDHeader); got != expectedRequestID {
		t.Fatalf("unexpected %s response header: got %q, want %q", requestIDHeader, got, expectedRequestID)
	}

	if got := res.Header().Get(correlationIDHeader); got != expectedRequestID {
		t.Fatalf("unexpected %s response header: got %q, want %q", correlationIDHeader, got, expectedRequestID)
	}
}

func TestSetRequestIDHeadersSetsBothHeaders(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	setRequestIDHeaders(headers, "req-1")

	if got := headers.Get(requestIDHeader); got != "req-1" {
		t.Fatalf("unexpected %s header: got %q, want %q", requestIDHeader, got, "req-1")
	}

	if got := headers.Get(correlationIDHeader); got != "req-1" {
		t.Fatalf("unexpected %s header: got %q, want %q", correlationIDHeader, got, "req-1")
	}
}
