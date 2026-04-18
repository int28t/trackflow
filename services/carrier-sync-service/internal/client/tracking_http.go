package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"trackflow/services/carrier-sync-service/internal/mapping"
	"trackflow/services/carrier-sync-service/internal/model"
)

const (
	trackingStatusSource = "carrier_sync"
	defaultHTTPTimeout   = 5 * time.Second
	maxErrorBodySize     = 4096
)

type TrackingHTTPClient struct {
	logger  *log.Logger
	baseURL string
	client  *http.Client
}

type trackingStatusUpdateRequest struct {
	Status   string         `json:"status"`
	Source   string         `json:"source"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

type trackingServiceError struct {
	Error string `json:"error"`
}

func NewTrackingHTTPClient(logger *log.Logger, baseURL string, timeout time.Duration) (*TrackingHTTPClient, error) {
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		return nil, errors.New("tracking service base url is required")
	}

	if _, err := url.ParseRequestURI(normalizedBaseURL); err != nil {
		return nil, fmt.Errorf("invalid tracking service base url: %w", err)
	}

	if logger == nil {
		logger = log.Default()
	}

	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}

	return &TrackingHTTPClient{
		logger:  logger,
		baseURL: normalizedBaseURL,
		client: &http.Client{
			Timeout: timeout,
		},
	}, nil
}

func (c *TrackingHTTPClient) PushStatusUpdate(ctx context.Context, update model.StatusUpdate) error {
	if c == nil || c.client == nil {
		return errors.New("tracking client is not configured")
	}

	orderID := strings.TrimSpace(update.OrderID)
	if orderID == "" {
		return errors.New("order_id is required")
	}

	status := strings.TrimSpace(update.ExternalStatus)
	if status == "" {
		return errors.New("external_status is required")
	}

	internalStatus, err := mapping.MapExternalStatus(status)
	if err != nil {
		return fmt.Errorf("map external status: %w", err)
	}

	payload := trackingStatusUpdateRequest{
		Status: internalStatus,
		Source: trackingStatusSource,
		Metadata: map[string]any{
			"carrier_external_status": status,
		},
	}

	if !update.UpdatedAt.IsZero() {
		payload.Metadata["carrier_updated_at"] = update.UpdatedAt.UTC().Format(time.RFC3339)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}

	endpoint := c.baseURL + "/orders/" + url.PathEscape(orderID) + "/status"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusOK && res.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	rawBody, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBodySize))
	message := extractTrackingError(rawBody)
	if message == "" {
		message = strings.TrimSpace(string(rawBody))
	}
	if message == "" {
		message = http.StatusText(res.StatusCode)
	}

	return fmt.Errorf("tracking service returned status %d: %s", res.StatusCode, message)
}

func extractTrackingError(rawBody []byte) string {
	if len(rawBody) == 0 {
		return ""
	}

	var payload trackingServiceError
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return ""
	}

	return strings.TrimSpace(payload.Error)
}
