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

	"trackflow/services/tracking-service/internal/model"
	"trackflow/services/tracking-service/internal/observability"
	"trackflow/services/tracking-service/internal/requestid"
	"trackflow/services/tracking-service/internal/service"
)

const (
	healthTimeout         = 2 * time.Second
	readTimeout           = 3 * time.Second
	updateStatusTimeout   = 5 * time.Second
	maxRequestPayloadSize = 128 * 1024
)

type Handler struct {
	logger *log.Logger
	svc    *service.TrackingService
}

type timelineResponse struct {
	Items []model.StatusHistoryItem `json:"items"`
}

type updateStatusResponse struct {
	OrderID   string    `json:"order_id"`
	Status    string    `json:"status"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
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
	metrics := observability.NewHTTPMetrics(logger)

	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.Handler())
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/orders/{id}/timeline", h.getOrderTimeline)
	mux.HandleFunc("/v1/orders/{order_id}/timeline", h.getOrderTimeline)
	mux.HandleFunc("/orders/{id}/status", h.updateOrderStatus)
	mux.HandleFunc("/v1/orders/{order_id}/status", h.updateOrderStatus)

	return requestid.Middleware(metrics.Middleware(mux))
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

	orderID := extractOrderID(r)
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
		if errors.Is(err, service.ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, extractValidationMessage(err))
			return
		}

		if errors.Is(err, service.ErrOrderNotFound) {
			writeJSONError(w, http.StatusNotFound, "order not found")
			return
		}

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

func (h *Handler) updateOrderStatus(w http.ResponseWriter, r *http.Request) {
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

	var payload model.UpdateStatusInput
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

	orderID := extractOrderID(r)
	if orderID == "" {
		writeJSONError(w, http.StatusBadRequest, "order_id is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), updateStatusTimeout)
	defer cancel()

	historyItem, err := h.svc.UpdateOrderStatus(ctx, orderID, payload)
	if err != nil {
		if errors.Is(err, service.ErrInvalidInput) {
			writeJSONError(w, http.StatusBadRequest, extractValidationMessage(err))
			return
		}

		if errors.Is(err, service.ErrOrderNotFound) {
			writeJSONError(w, http.StatusNotFound, "order not found")
			return
		}

		if errors.Is(err, service.ErrStatusTransitionNotAllowed) {
			writeJSONError(w, http.StatusConflict, "status transition is not allowed")
			return
		}

		h.logger.Printf("update status failed for order %s: %v", orderID, err)
		writeJSONError(w, http.StatusInternalServerError, "failed to update order status")
		return
	}

	writeJSON(w, http.StatusOK, updateStatusResponse{
		OrderID:   historyItem.OrderID,
		Status:    historyItem.Status,
		Source:    historyItem.Source,
		CreatedAt: historyItem.CreatedAt,
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

func extractOrderID(r *http.Request) string {
	if r == nil {
		return ""
	}

	orderID := strings.TrimSpace(r.PathValue("order_id"))
	if orderID != "" {
		return orderID
	}

	return strings.TrimSpace(r.PathValue("id"))
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
