package handler

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"trackflow/services/order-service/internal/service"
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

func TestExtractValidationMessage(t *testing.T) {
	t.Parallel()

	err := errors.New(service.ErrInvalidInput.Error() + ": customer_id is required")
	got := extractValidationMessage(err)
	if got != "customer_id is required" {
		t.Fatalf("unexpected extracted message: got %q", got)
	}

	if got := extractValidationMessage(nil); got != "invalid request" {
		t.Fatalf("unexpected message for nil error: got %q", got)
	}
}
