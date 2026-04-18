package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"trackflow/services/carrier-sync-service/internal/model"
	"trackflow/services/carrier-sync-service/internal/service"
)

const (
	healthTimeout = 2 * time.Second
	syncTimeout   = 5 * time.Second
)

type Handler struct {
	logger *log.Logger
	svc    *service.SyncService
}

type syncResponse struct {
	Fetched int                  `json:"fetched"`
	Items   []model.StatusUpdate `json:"items"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func New(logger *log.Logger, svc *service.SyncService) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	h := &Handler{
		logger: logger,
		svc:    svc,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.health)
	mux.HandleFunc("/internal/sync/run", h.runSync)

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
		writeJSONError(w, http.StatusServiceUnavailable, "carrier client unavailable")
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (h *Handler) runSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if h.svc == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	batch, err := parseBatch(r.URL.Query().Get("batch"))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), syncTimeout)
	defer cancel()

	updates, err := h.svc.SyncOnce(ctx, batch)
	if err != nil {
		h.logger.Printf("manual sync run failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "sync run failed")
		return
	}

	writeJSON(w, http.StatusOK, syncResponse{Fetched: len(updates), Items: updates})
}

func parseBatch(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}

	batch, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("batch must be an integer")
	}

	if batch <= 0 {
		return 0, errors.New("batch must be greater than zero")
	}

	return batch, nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
