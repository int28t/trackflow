package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"trackflow/services/order-service/internal/model"
	"trackflow/services/order-service/internal/requestid"
	"trackflow/services/order-service/internal/service"
)

const (
	healthTimeout         = 2 * time.Second
	listTimeout           = 3 * time.Second
	getOrderTimeout       = 3 * time.Second
	assignOrderTimeout    = 5 * time.Second
	createOrderTimeout    = 5 * time.Second
	maxRequestPayloadSize = 128 * 1024
)

type Handler struct {
	logger *log.Logger
	svc    *service.OrderService
}

type listOrdersResponse struct {
	Items []model.Order `json:"items"`
}

type createOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type assignOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
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
	mux.HandleFunc("/orders", h.createOrder)
	mux.HandleFunc("/orders/{id}", h.getOrderByID)
	mux.HandleFunc("/orders/{id}/assign", h.assignOrder)
	mux.HandleFunc("/v1/orders", h.orders)
	mux.HandleFunc("/v1/orders/{id}", h.getOrderByID)
	mux.HandleFunc("/v1/orders/{id}/assign", h.assignOrder)

	return requestid.Middleware(mux)
}

func (h *Handler) orders(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listOrders(w, r)
	case http.MethodPost:
		h.createOrder(w, r)
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
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

func (h *Handler) createOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeJSONError(w, http.StatusBadRequest, "Idempotency-Key header is required")
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxRequestPayloadSize)
	defer body.Close()

	var payload model.CreateOrderInput
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), createOrderTimeout)
	defer cancel()

	order, created, err := h.svc.CreateOrder(ctx, payload, idempotencyKey)
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, extractValidationMessage(err))
			return
		}

		h.logger.Printf("create order failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to create order")
		return
	}

	statusCode := http.StatusCreated
	if !created {
		statusCode = http.StatusOK
	}

	writeJSON(w, statusCode, createOrderResponse{
		OrderID: order.ID,
		Status:  order.Status,
	})
}

func (h *Handler) getOrderByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	orderID := strings.TrimSpace(r.PathValue("id"))

	ctx, cancel := context.WithTimeout(r.Context(), getOrderTimeout)
	defer cancel()

	order, err := h.svc.GetOrderByID(ctx, orderID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, extractValidationMessage(err))
			return
		}

		if errors.Is(err, service.ErrOrderNotFound) {
			writeJSONError(w, http.StatusNotFound, "order not found")
			return
		}

		h.logger.Printf("get order failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to get order")
		return
	}

	writeJSON(w, http.StatusOK, order)
}

func (h *Handler) assignOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxRequestPayloadSize)
	defer body.Close()

	var payload model.AssignOrderInput
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	orderID := strings.TrimSpace(r.PathValue("id"))

	ctx, cancel := context.WithTimeout(r.Context(), assignOrderTimeout)
	defer cancel()

	order, err := h.svc.AssignOrder(ctx, orderID, payload)
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, extractValidationMessage(err))
			return
		}

		if errors.Is(err, service.ErrOrderNotFound) {
			writeJSONError(w, http.StatusNotFound, "order not found")
			return
		}

		if errors.Is(err, service.ErrCourierNotFound) {
			writeJSONError(w, http.StatusNotFound, "courier not found")
			return
		}

		if errors.Is(err, service.ErrOrderAlreadyAssigned) {
			writeJSONError(w, http.StatusConflict, "order already assigned")
			return
		}

		if errors.Is(err, service.ErrAssignmentNotAllowed) {
			writeJSONError(w, http.StatusConflict, "assignment is not allowed for current order status")
			return
		}

		h.logger.Printf("assign order failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to assign order")
		return
	}

	writeJSON(w, http.StatusOK, assignOrderResponse{
		OrderID: order.ID,
		Status:  order.Status,
	})
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

func ensureSingleJSONValue(decoder *json.Decoder) error {
	if decoder == nil {
		return errors.New("decoder is nil")
	}

	if err := decoder.Decode(&struct{}{}); err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}

		return err
	}

	return errors.New("multiple json values")
}

func extractValidationMessage(err error) string {
	if err == nil {
		return "invalid request"
	}

	message := err.Error()
	prefix := service.ErrInvalidInput.Error() + ": "
	if strings.HasPrefix(message, prefix) {
		return strings.TrimPrefix(message, prefix)
	}

	return message
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
