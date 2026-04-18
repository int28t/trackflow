package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"trackflow/services/tracking-service/internal/model"
	"trackflow/services/tracking-service/internal/service"
)

const (
	healthTimeout = 2 * time.Second
	readTimeout   = 3 * time.Second
)

type Handler struct {
	logger *log.Logger
	svc    *service.TrackingService
}

type timelineResponse struct {
	Items []model.StatusHistoryItem `json:"items"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(logger *log.Logger, svc *service.TrackingService) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	h := &Handler{
		logger: logger,
		svc:    svc,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/v1/orders/{order_id}/timeline", h.getOrderTimeline)

	return mux
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), healthTimeout)
	defer cancel()

	if err := h.svc.Health(ctx); err != nil {
		h.logger.Printf("health check failed: %v", err)
		writeJSONError(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) getOrderTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	orderID := strings.TrimSpace(r.PathValue("order_id"))
	if orderID == "" {
		writeJSONError(w, http.StatusBadRequest, "order_id is required")
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), readTimeout)
	defer cancel()

	items, err := h.svc.GetOrderTimeline(ctx, orderID, limit)
	if err != nil {
		if isInvalidUUIDError(err) {
			writeJSONError(w, http.StatusBadRequest, "order_id must be a valid UUID")
			return
		}

		h.logger.Printf("get timeline failed for order %s: %v", orderID, err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get order timeline")
		return
	}

	writeJSON(w, http.StatusOK, timelineResponse{Items: items})
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}

	if limit <= 0 {
		return 0, errors.New("limit must be greater than zero")
	}

	return limit, nil
}

func isInvalidUUIDError(err error) bool {
	if err == nil {
		return false
	}

	return strings.Contains(err.Error(), "invalid input syntax for type uuid")
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
