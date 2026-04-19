package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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

func TestIsValidationError(t *testing.T) {
	t.Parallel()

	if !isValidationError(errors.New("order_id is required")) {
		t.Fatal("expected true for known validation error")
	}

	if isValidationError(errors.New("unexpected")) {
		t.Fatal("expected false for unknown error")
	}

	if isValidationError(nil) {
		t.Fatal("expected false for nil error")
	}
}

func TestWriteJSONAndError(t *testing.T) {
	t.Parallel()

	res := httptest.NewRecorder()
	writeJSON(res, http.StatusCreated, map[string]string{"status": "ok"})

	if res.Code != http.StatusCreated {
		t.Fatalf("unexpected status code: got %d, want %d", res.Code, http.StatusCreated)
	}

	if got := res.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: got %q", got)
	}

	var payload map[string]string
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}

	if payload["status"] != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	errRes := httptest.NewRecorder()
	writeJSONError(errRes, http.StatusBadRequest, "bad request")

	if errRes.Code != http.StatusBadRequest {
		t.Fatalf("unexpected error status code: got %d, want %d", errRes.Code, http.StatusBadRequest)
	}

	var errPayload errorResponse
	if err := json.NewDecoder(errRes.Body).Decode(&errPayload); err != nil {
		t.Fatalf("decode error payload failed: %v", err)
	}

	if errPayload.Error != "bad request" {
		t.Fatalf("unexpected error message: got %q", errPayload.Error)
	}
}
