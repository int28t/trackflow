package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"trackflow/services/order-service/internal/model"
	"trackflow/services/order-service/internal/service"
)

const (
	healthTimeout = 2 * time.Second
	listTimeout   = 3 * time.Second
)

type Handler struct {
	logger *log.Logger
	svc    *service.OrderService
}

type listOrdersResponse struct {
	Items []model.Order `json:"items"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(logger *log.Logger, svc *service.OrderService) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	h := &Handler{
		logger: logger,
		svc:    svc,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/v1/orders", h.listOrders)

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

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), listTimeout)
	defer cancel()

	orders, err := h.svc.ListOrders(ctx, limit)
	if err != nil {
		h.logger.Printf("list orders failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to list orders")
		return
	}

	writeJSON(w, http.StatusOK, listOrdersResponse{Items: orders})
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
