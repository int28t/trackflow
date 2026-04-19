package requestid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResolveFromHeadersPrefersRequestID(t *testing.T) {
	t.Parallel()

	headers := make(http.Header)
	headers.Set(CorrelationHeaderName, "corr-1")
	headers.Set(HeaderName, "req-1")

	got := ResolveFromHeaders(headers)
	if got != "req-1" {
		t.Fatalf("unexpected resolved id: got %q, want %q", got, "req-1")
	}
}

func TestMiddlewareSetsHeadersAndContext(t *testing.T) {
	t.Parallel()

	var gotFromContext string
	h := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotFromContext = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	req.Header.Set(CorrelationHeaderName, "corr-2")

	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)

	if gotFromContext != "corr-2" {
		t.Fatalf("unexpected id from context: got %q, want %q", gotFromContext, "corr-2")
	}

	if got := res.Header().Get(HeaderName); got != "corr-2" {
		t.Fatalf("unexpected response %s: got %q, want %q", HeaderName, got, "corr-2")
	}

	if got := res.Header().Get(CorrelationHeaderName); got != "corr-2" {
		t.Fatalf("unexpected response %s: got %q, want %q", CorrelationHeaderName, got, "corr-2")
	}
}

func TestApplyToRequestUsesContextID(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://example.local/notify", nil)
	req = req.WithContext(WithRequestID(req.Context(), "req-ctx-1"))

	applied := ApplyToRequest(req)
	if applied != "req-ctx-1" {
		t.Fatalf("unexpected applied id: got %q, want %q", applied, "req-ctx-1")
	}

	if got := req.Header.Get(HeaderName); got != "req-ctx-1" {
		t.Fatalf("unexpected request %s: got %q, want %q", HeaderName, got, "req-ctx-1")
	}

	if got := req.Header.Get(CorrelationHeaderName); got != "req-ctx-1" {
		t.Fatalf("unexpected request %s: got %q, want %q", CorrelationHeaderName, got, "req-ctx-1")
	}
}

func TestApplyToRequestGeneratesIDWhenMissing(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "http://example.local/notify", nil)
	id := ApplyToRequest(req)
	if id == "" {
		t.Fatal("expected non-empty generated request id")
	}

	if got := req.Header.Get(HeaderName); got == "" {
		t.Fatalf("expected %s to be set", HeaderName)
	}

	if got := req.Header.Get(CorrelationHeaderName); got == "" {
		t.Fatalf("expected %s to be set", CorrelationHeaderName)
	}
}

func TestWithRequestIDWithNilContext(t *testing.T) {
	t.Parallel()

	ctx := WithRequestID(nil, "req-3")
	if got := FromContext(ctx); got != "req-3" {
		t.Fatalf("unexpected id from context: got %q, want %q", got, "req-3")
	}

	if got := FromContext(context.Background()); got != "" {
		t.Fatalf("unexpected id in empty context: got %q", got)
	}
}
