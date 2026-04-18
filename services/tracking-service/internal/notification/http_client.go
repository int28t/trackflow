package notification

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

	"trackflow/services/tracking-service/internal/model"
	"trackflow/services/tracking-service/internal/requestid"
)

const (
	defaultRequestTimeout = 3 * time.Second
	maxErrorBodyBytes     = 4096

	channelEmail    = "email"
	channelTelegram = "telegram"
)

type HTTPClient struct {
	logger            *log.Logger
	baseURL           string
	httpClient        *http.Client
	emailRecipient    string
	telegramRecipient string
}

type sendNotificationRequest struct {
	OrderID   string `json:"order_id"`
	Status    string `json:"status"`
	Channel   string `json:"channel"`
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
}

type sendNotificationResponse struct {
	Error string `json:"error"`
}

func NewHTTPClient(logger *log.Logger, baseURL string, timeout time.Duration, emailRecipient, telegramRecipient string) (*HTTPClient, error) {
	normalizedBaseURL := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if normalizedBaseURL == "" {
		return nil, errors.New("notification service base url is required")
	}

	if _, err := url.ParseRequestURI(normalizedBaseURL); err != nil {
		return nil, fmt.Errorf("invalid notification service base url: %w", err)
	}

	if logger == nil {
		logger = log.Default()
	}

	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

	normalizedEmailRecipient := strings.TrimSpace(emailRecipient)
	if normalizedEmailRecipient == "" {
		normalizedEmailRecipient = "user@example.com"
	}

	normalizedTelegramRecipient := strings.TrimSpace(telegramRecipient)
	if normalizedTelegramRecipient == "" {
		normalizedTelegramRecipient = "@trackflow_user"
	}

	return &HTTPClient{
		logger:            logger,
		baseURL:           normalizedBaseURL,
		httpClient:        &http.Client{Timeout: timeout},
		emailRecipient:    normalizedEmailRecipient,
		telegramRecipient: normalizedTelegramRecipient,
	}, nil
}

func (c *HTTPClient) NotifyStatusChanged(ctx context.Context, item model.StatusHistoryItem) error {
	if c == nil || c.httpClient == nil {
		return errors.New("notification client is not configured")
	}

	orderID := strings.TrimSpace(item.OrderID)
	status := strings.TrimSpace(item.Status)
	if orderID == "" || status == "" {
		return errors.New("order_id and status are required")
	}

	requests := []sendNotificationRequest{
		{
			OrderID:   orderID,
			Status:    status,
			Channel:   channelEmail,
			Recipient: c.emailRecipient,
			Message:   buildNotificationMessage(item),
		},
		{
			OrderID:   orderID,
			Status:    status,
			Channel:   channelTelegram,
			Recipient: c.telegramRecipient,
			Message:   buildNotificationMessage(item),
		},
	}

	errorsByChannel := make([]string, 0)
	for _, reqPayload := range requests {
		if err := c.send(ctx, reqPayload); err != nil {
			errorsByChannel = append(errorsByChannel, fmt.Sprintf("%s: %v", reqPayload.Channel, err))
		}
	}

	if len(errorsByChannel) > 0 {
		return fmt.Errorf("notification dispatch failed: %s", strings.Join(errorsByChannel, "; "))
	}

	return nil
}

func (c *HTTPClient) send(ctx context.Context, payload sendNotificationRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal notification request: %w", err)
	}

	endpoint := c.baseURL + "/internal/notifications/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	requestid.ApplyToRequest(req)

	res, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send notification request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode >= http.StatusOK && res.StatusCode < http.StatusMultipleChoices {
		return nil
	}

	rawBody, _ := io.ReadAll(io.LimitReader(res.Body, maxErrorBodyBytes))
	message := extractNotificationError(rawBody)
	if message == "" {
		message = strings.TrimSpace(string(rawBody))
	}
	if message == "" {
		message = http.StatusText(res.StatusCode)
	}

	return fmt.Errorf("notification service returned status %d: %s", res.StatusCode, message)
}

func extractNotificationError(rawBody []byte) string {
	if len(rawBody) == 0 {
		return ""
	}

	var payload sendNotificationResponse
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		return ""
	}

	return strings.TrimSpace(payload.Error)
}

func buildNotificationMessage(item model.StatusHistoryItem) string {
	createdAt := item.CreatedAt.UTC().Format(time.RFC3339)
	return fmt.Sprintf("Order %s status changed to %s at %s", item.OrderID, item.Status, createdAt)
}
