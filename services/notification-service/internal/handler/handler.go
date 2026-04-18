package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"trackflow/services/notification-service/internal/model"
	"trackflow/services/notification-service/internal/requestid"
	"trackflow/services/notification-service/internal/service"
)

const (
	healthTimeout = 2 * time.Second
	sendTimeout   = 3 * time.Second
	maxBodyBytes  = 64 * 1024
)

type Handler struct {
	logger *log.Logger
	svc    *service.NotificationService
}

type sendResponse struct {
	Status string `json:"status"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(logger *log.Logger, svc *service.NotificationService) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	h := &Handler{
		logger: logger,
		svc:    svc,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/internal/notifications/send", h.send)

	return requestid.Middleware(mux)
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
		writeJSONError(w, http.StatusServiceUnavailable, "sender unavailable")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	body := http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer body.Close()

	var event model.Event
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&event); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	if err := ensureSingleJSONValue(decoder); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), sendTimeout)
	defer cancel()

	if err := h.svc.Send(ctx, event); err != nil {
		if isValidationError(err) {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}

		h.logger.Printf("send notification failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "failed to send notification")
		return
	}

	writeJSON(w, http.StatusOK, sendResponse{Status: "sent"})
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

func isValidationError(err error) bool {
	if err == nil {
		return false
	}

	message := err.Error()
	return message == "order_id is required" ||
		message == "status is required" ||
		message == "channel is required" ||
		message == "channel must be one of: email, telegram" ||
		message == "recipient is required" ||
		message == "message is required"
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
