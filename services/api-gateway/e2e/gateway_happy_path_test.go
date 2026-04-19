//go:build e2e

package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	defaultGatewayBaseURL = "http://127.0.0.1:8080"
	defaultCourierID      = "c1111111-1111-1111-1111-111111111111"
	defaultCustomerID     = "d1111111-1111-1111-1111-111111111111"
)

type createOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type assignOrderResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
}

type updateStatusResponse struct {
	OrderID string `json:"order_id"`
	Status  string `json:"status"`
	Source  string `json:"source"`
}

type orderResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type timelineResponse struct {
	Items []timelineItem `json:"items"`
}

type timelineItem struct {
	Status string `json:"status"`
	Source string `json:"source"`
}

type gatewayErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	RequestID string `json:"request_id"`
}

func TestGatewayHappyPathSmoke(t *testing.T) {
	baseURL := strings.TrimRight(getEnv("E2E_GATEWAY_BASE_URL", defaultGatewayBaseURL), "/")
	courierID := strings.TrimSpace(getEnv("E2E_COURIER_ID", defaultCourierID))

	client := &http.Client{Timeout: 15 * time.Second}
	requestID := fmt.Sprintf("e2e-smoke-%d", time.Now().UnixNano())
	idempotencyKey := fmt.Sprintf("e2e-idempotency-%d", time.Now().UnixNano())

	healthStatus, healthBody, _ := sendJSONRequest(t, client, http.MethodGet, baseURL+"/health", map[string]string{
		"X-Request-ID": requestID,
	}, nil)
	if healthStatus != http.StatusOK {
		t.Fatalf("health check failed: status=%d body=%s", healthStatus, string(healthBody))
	}

	createPayload := map[string]any{
		"customer_id": defaultCustomerID,
		"pickup_address": map[string]any{
			"city":      "Volgograd",
			"street":    "Ulitsa Gagarina",
			"house":     "14",
			"apartment": "12",
			"lat":       48.712788,
			"lng":       44.515595,
		},
		"dropoff_address": map[string]any{
			"city":      "Volgograd",
			"street":    "Ulitsa Marshala Chuikova",
			"house":     "47",
			"apartment": "4",
			"lat":       48.717594,
			"lng":       44.534392,
		},
		"weight_kg":     1.25,
		"distance_km":   2.2,
		"service_level": "standard",
	}

	createStatus, createBody, createHeaders := sendJSONRequest(t, client, http.MethodPost, baseURL+"/v1/orders", map[string]string{
		"X-Request-ID":    requestID,
		"Idempotency-Key": idempotencyKey,
	}, createPayload)

	if createStatus != http.StatusCreated {
		if createStatus == http.StatusBadRequest || createStatus == http.StatusConflict || createStatus == http.StatusNotFound {
			errResp := decodeGatewayError(t, createBody)
			t.Fatalf("create order failed: status=%d code=%s message=%s body=%s", createStatus, errResp.Error.Code, errResp.Error.Message, string(createBody))
		}
		t.Fatalf("unexpected create order status=%d body=%s", createStatus, string(createBody))
	}

	if got := strings.TrimSpace(createHeaders.Get("X-Request-ID")); got == "" {
		t.Fatalf("response does not contain X-Request-ID header")
	}

	created := decodeJSON[createOrderResponse](t, createBody)
	if strings.TrimSpace(created.OrderID) == "" {
		t.Fatalf("create response has empty order_id: %s", string(createBody))
	}
	if created.Status != "created" {
		t.Fatalf("unexpected initial order status: got=%s want=created", created.Status)
	}

	orderID := created.OrderID

	getStatus, getBody, _ := sendJSONRequest(t, client, http.MethodGet, baseURL+"/v1/orders/"+orderID, map[string]string{
		"X-Request-ID": requestID,
	}, nil)
	if getStatus != http.StatusOK {
		t.Fatalf("get order failed: status=%d body=%s", getStatus, string(getBody))
	}

	orderBeforeAssign := decodeJSON[orderResponse](t, getBody)
	if orderBeforeAssign.ID != orderID {
		t.Fatalf("unexpected order id: got=%s want=%s", orderBeforeAssign.ID, orderID)
	}
	if orderBeforeAssign.Status != "created" {
		t.Fatalf("unexpected order status before assign: got=%s want=created", orderBeforeAssign.Status)
	}

	assignStatus, assignBody, _ := sendJSONRequest(t, client, http.MethodPost, baseURL+"/v1/orders/"+orderID+"/assign", map[string]string{
		"X-Request-ID": requestID,
	}, map[string]any{
		"courier_id":  courierID,
		"assigned_by": "e2e-smoke",
		"comment":     "assigned in gateway happy path smoke",
	})
	if assignStatus != http.StatusOK {
		errResp := decodeGatewayError(t, assignBody)
		t.Fatalf("assign order failed: status=%d code=%s message=%s body=%s", assignStatus, errResp.Error.Code, errResp.Error.Message, string(assignBody))
	}

	assigned := decodeJSON[assignOrderResponse](t, assignBody)
	if assigned.OrderID != orderID {
		t.Fatalf("unexpected assigned order_id: got=%s want=%s", assigned.OrderID, orderID)
	}
	if assigned.Status != "assigned" {
		t.Fatalf("unexpected status after assign: got=%s want=assigned", assigned.Status)
	}

	updateToInTransit := map[string]any{
		"status":  "in_transit",
		"source":  "courier",
		"comment": "picked up",
		"metadata": map[string]any{
			"smoke": true,
		},
	}

	inTransitStatus, inTransitBody, _ := sendJSONRequest(t, client, http.MethodPost, baseURL+"/v1/orders/"+orderID+"/status", map[string]string{
		"X-Request-ID": requestID,
	}, updateToInTransit)
	if inTransitStatus != http.StatusOK {
		errResp := decodeGatewayError(t, inTransitBody)
		t.Fatalf("update status to in_transit failed: status=%d code=%s message=%s body=%s", inTransitStatus, errResp.Error.Code, errResp.Error.Message, string(inTransitBody))
	}

	inTransit := decodeJSON[updateStatusResponse](t, inTransitBody)
	if inTransit.Status != "in_transit" {
		t.Fatalf("unexpected status after in_transit update: got=%s want=in_transit", inTransit.Status)
	}

	updateToDelivered := map[string]any{
		"status":  "delivered",
		"source":  "courier",
		"comment": "delivered to recipient",
		"metadata": map[string]any{
			"smoke": true,
		},
	}

	deliveredStatus, deliveredBody, _ := sendJSONRequest(t, client, http.MethodPost, baseURL+"/v1/orders/"+orderID+"/status", map[string]string{
		"X-Request-ID": requestID,
	}, updateToDelivered)
	if deliveredStatus != http.StatusOK {
		errResp := decodeGatewayError(t, deliveredBody)
		t.Fatalf("update status to delivered failed: status=%d code=%s message=%s body=%s", deliveredStatus, errResp.Error.Code, errResp.Error.Message, string(deliveredBody))
	}

	delivered := decodeJSON[updateStatusResponse](t, deliveredBody)
	if delivered.Status != "delivered" {
		t.Fatalf("unexpected status after delivered update: got=%s want=delivered", delivered.Status)
	}

	timelineStatus, timelineBody, _ := sendJSONRequest(t, client, http.MethodGet, baseURL+"/v1/orders/"+orderID+"/timeline?limit=20", map[string]string{
		"X-Request-ID": requestID,
	}, nil)
	if timelineStatus != http.StatusOK {
		t.Fatalf("get timeline failed: status=%d body=%s", timelineStatus, string(timelineBody))
	}

	timeline := decodeJSON[timelineResponse](t, timelineBody)
	if len(timeline.Items) < 3 {
		t.Fatalf("timeline too short: got=%d expected at least 3", len(timeline.Items))
	}

	assertStatusSequence(t, timeline.Items, []string{"assigned", "in_transit", "delivered"})

	finalGetStatus, finalGetBody, _ := sendJSONRequest(t, client, http.MethodGet, baseURL+"/v1/orders/"+orderID, map[string]string{
		"X-Request-ID": requestID,
	}, nil)
	if finalGetStatus != http.StatusOK {
		t.Fatalf("final get order failed: status=%d body=%s", finalGetStatus, string(finalGetBody))
	}

	finalOrder := decodeJSON[orderResponse](t, finalGetBody)
	if finalOrder.Status != "delivered" {
		t.Fatalf("unexpected final order status: got=%s want=delivered", finalOrder.Status)
	}
}

func sendJSONRequest(t *testing.T, client *http.Client, method, requestURL string, headers map[string]string, payload any) (int, []byte, http.Header) {
	t.Helper()

	var bodyReader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal request payload failed: %v", err)
		}
		bodyReader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequest(method, requestURL, bodyReader)
	if err != nil {
		t.Fatalf("create request failed: %v", err)
	}

	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed method=%s url=%s err=%v", method, requestURL, err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}

	return resp.StatusCode, responseBody, resp.Header
}

func decodeJSON[T any](t *testing.T, body []byte) T {
	t.Helper()

	var payload T
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode response JSON failed: %v, body=%s", err, string(body))
	}

	return payload
}

func decodeGatewayError(t *testing.T, body []byte) gatewayErrorResponse {
	t.Helper()

	var payload gatewayErrorResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return gatewayErrorResponse{}
	}

	return payload
}

func assertStatusSequence(t *testing.T, items []timelineItem, expected []string) {
	t.Helper()

	position := 0
	for _, item := range items {
		if position >= len(expected) {
			break
		}

		if item.Status == expected[position] {
			position++
		}
	}

	if position != len(expected) {
		t.Fatalf("timeline does not contain expected status subsequence %v, got items=%+v", expected, items)
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}
