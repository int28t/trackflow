package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"trackflow/services/tracking-service/internal/service"
)

func TestParseLimitValidation(t *testing.T) {
	t.Parallel()

	if _, err := parseLimit("bad"); err == nil {
		t.Fatal("expected parse error for non-integer limit")
	}

	if _, err := parseLimit("0"); err == nil {
		t.Fatal("expected parse error for zero limit")
	}

	limit, err := parseLimit("25")
	if err != nil {
		t.Fatalf("parseLimit returned error: %v", err)
	}

	if limit != 25 {
		t.Fatalf("unexpected parsed limit: got %d, want %d", limit, 25)
	}
}

func TestEnsureSingleJSONValue(t *testing.T) {
	t.Parallel()

	decoder := json.NewDecoder(strings.NewReader("{\"a\":1} {\"b\":2}"))
	var payload map[string]any
	if err := decoder.Decode(&payload); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	if err := ensureSingleJSONValue(decoder); err == nil {
		t.Fatal("expected error for multiple json values")
	}

	if err := ensureSingleJSONValue(nil); err == nil {
		t.Fatal("expected error for nil decoder")
	}
}

func TestExtractOrderID(t *testing.T) {
	t.Parallel()

	reqByOrderID := httptest.NewRequest("GET", "/v1/orders/abc/timeline", nil)
	reqByOrderID.SetPathValue("order_id", "  order-1  ")
	if got := extractOrderID(reqByOrderID); got != "order-1" {
		t.Fatalf("unexpected order_id extraction: got %q", got)
	}

	reqByID := httptest.NewRequest("GET", "/orders/abc/timeline", nil)
	reqByID.SetPathValue("id", " id-2 ")
	if got := extractOrderID(reqByID); got != "id-2" {
		t.Fatalf("unexpected id extraction: got %q", got)
	}

	if got := extractOrderID(nil); got != "" {
		t.Fatalf("expected empty id for nil request, got %q", got)
	}
}

func TestExtractValidationMessageAndUUIDError(t *testing.T) {
	t.Parallel()

	err := errors.New(service.ErrInvalidInput.Error() + ": status is required")
	if got := extractValidationMessage(err); got != "status is required" {
		t.Fatalf("unexpected extracted message: got %q", got)
	}

	if got := extractValidationMessage(nil); got != "invalid request" {
		t.Fatalf("unexpected message for nil error: got %q", got)
	}

	if !isInvalidUUIDError(errors.New("invalid input syntax for type uuid: bad")) {
		t.Fatal("expected true for invalid uuid error")
	}

	if isInvalidUUIDError(nil) {
		t.Fatal("expected false for nil error")
	}
}
