package gateway

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	orderServiceURLEnvKey     = "ORDER_SERVICE_URL"
	trackingServiceURLEnvKey  = "TRACKING_SERVICE_URL"
	defaultOrderServiceURL    = "http://order-service:8082"
	defaultTrackingServiceURL = "http://tracking-service:8083"
	upstreamRequestTimeout    = 10 * time.Second
	maxUpstreamErrorBodySize  = 64 * 1024
)

type GatewayProxy struct {
	logger       *log.Logger
	client       *http.Client
	orderBase    *url.URL
	trackingBase *url.URL
}

func NewGatewayProxy(logger *log.Logger) *GatewayProxy {
	if logger == nil {
		logger = log.Default()
	}

	return &GatewayProxy{
		logger: logger,
		client: &http.Client{Timeout: upstreamRequestTimeout},
		orderBase: parseServiceURL(
			logger,
			orderServiceURLEnvKey,
			os.Getenv(orderServiceURLEnvKey),
			defaultOrderServiceURL,
		),
		trackingBase: parseServiceURL(
			logger,
			trackingServiceURLEnvKey,
			os.Getenv(trackingServiceURLEnvKey),
			defaultTrackingServiceURL,
		),
	}
}

func (p *GatewayProxy) ordersCollection(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		return MethodNotAllowed(r.Method)
	}

	return p.forward(w, r, p.orderBase)
}

func (p *GatewayProxy) orderByID(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return MethodNotAllowed(r.Method)
	}

	return p.forward(w, r, p.orderBase)
}

func (p *GatewayProxy) assignOrder(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return MethodNotAllowed(r.Method)
	}

	return p.forward(w, r, p.orderBase)
}

func (p *GatewayProxy) updateOrderStatus(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodPost {
		return MethodNotAllowed(r.Method)
	}

	return p.forward(w, r, p.trackingBase)
}

func (p *GatewayProxy) getOrderTimeline(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return MethodNotAllowed(r.Method)
	}

	return p.forward(w, r, p.trackingBase)
}

func (p *GatewayProxy) forward(w http.ResponseWriter, r *http.Request, base *url.URL) error {
	if p == nil || p.client == nil || base == nil {
		return NewHTTPError(http.StatusBadGateway, "upstream_unavailable", "upstream service is not configured", nil)
	}

	targetURL := buildTargetURL(base, r.URL.Path, r.URL.RawQuery)

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), r.Body)
	if err != nil {
		return NewHTTPError(http.StatusBadGateway, "upstream_request_error", "failed to prepare upstream request", err)
	}

	upstreamReq.Header = cloneHeader(r.Header)
	upstreamReq.ContentLength = r.ContentLength

	requestID := getRequestID(r.Context())
	if requestID != "" {
		upstreamReq.Header.Set(requestIDHeader, requestID)
	}

	upstreamResp, err := p.client.Do(upstreamReq)
	if err != nil {
		return NewHTTPError(http.StatusBadGateway, "upstream_unavailable", "upstream service is unavailable", err)
	}
	defer upstreamResp.Body.Close()

	if upstreamResp.StatusCode >= http.StatusBadRequest {
		code, message, rawBody, parseErr := parseUpstreamError(upstreamResp)
		if parseErr != nil {
			p.logger.Printf("failed to parse upstream error method=%s path=%s status=%d err=%v", r.Method, r.URL.Path, upstreamResp.StatusCode, parseErr)
		}

		if code == "" {
			code = statusCodeToErrorCode(upstreamResp.StatusCode)
		}

		if message == "" {
			message = strings.ToLower(http.StatusText(upstreamResp.StatusCode))
			if message == "" {
				message = "upstream service error"
			}
		}

		wrappedErr := error(nil)
		if rawBody != "" {
			wrappedErr = fmt.Errorf("upstream response: %s", rawBody)
		}

		return NewHTTPError(upstreamResp.StatusCode, code, message, wrappedErr)
	}

	copyHeaders(w.Header(), upstreamResp.Header)
	w.WriteHeader(upstreamResp.StatusCode)

	if _, err := io.Copy(w, upstreamResp.Body); err != nil {
		p.logger.Printf("upstream response copy failed method=%s path=%s err=%v", r.Method, r.URL.Path, err)
		return nil
	}

	return nil
}

func parseUpstreamError(resp *http.Response) (string, string, string, error) {
	if resp == nil || resp.Body == nil {
		return "", "", "", nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxUpstreamErrorBodySize))
	if err != nil {
		return "", "", "", err
	}

	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "", "", "", nil
	}

	type detailedError struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}

	type envelope struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Error   json.RawMessage `json:"error"`
	}

	var payload envelope
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", raw, raw, nil
	}

	code := strings.TrimSpace(payload.Code)
	message := strings.TrimSpace(payload.Message)

	if len(payload.Error) > 0 {
		var textValue string
		if err := json.Unmarshal(payload.Error, &textValue); err == nil {
			if message == "" {
				message = strings.TrimSpace(textValue)
			}
		} else {
			var objectValue detailedError
			if err := json.Unmarshal(payload.Error, &objectValue); err == nil {
				if code == "" {
					code = strings.TrimSpace(objectValue.Code)
				}

				if message == "" {
					message = strings.TrimSpace(objectValue.Message)
				}
			}
		}
	}

	return code, message, raw, nil
}

func statusCodeToErrorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "too_many_requests"
	case http.StatusInternalServerError:
		return "internal_error"
	case http.StatusBadGateway:
		return "bad_gateway"
	case http.StatusServiceUnavailable:
		return "service_unavailable"
	case http.StatusGatewayTimeout:
		return "gateway_timeout"
	default:
		return "upstream_error"
	}
}

func buildTargetURL(base *url.URL, requestPath, rawQuery string) *url.URL {
	target := *base
	target.Path = joinURLPaths(base.Path, requestPath)
	target.RawQuery = rawQuery
	return &target
}

func joinURLPaths(basePath, requestPath string) string {
	baseTrimmed := strings.TrimSuffix(basePath, "/")
	requestTrimmed := strings.TrimPrefix(requestPath, "/")

	if baseTrimmed == "" {
		return "/" + requestTrimmed
	}

	if requestTrimmed == "" {
		return baseTrimmed
	}

	return baseTrimmed + "/" + requestTrimmed
}

func parseServiceURL(logger *log.Logger, envKey, value, fallback string) *url.URL {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		candidate = fallback
	}

	parsed, err := url.Parse(candidate)
	if err == nil && parsed.Scheme != "" && parsed.Host != "" {
		return parsed
	}

	logger.Printf("invalid %s=%q, fallback to %q", envKey, value, fallback)
	parsedFallback, _ := url.Parse(fallback)
	return parsedFallback
}

func cloneHeader(source http.Header) http.Header {
	cloned := make(http.Header, len(source))
	for key, values := range source {
		clonedValues := make([]string, len(values))
		copy(clonedValues, values)
		cloned[key] = clonedValues
	}

	return cloned
}

func copyHeaders(destination, source http.Header) {
	for key, values := range source {
		for _, value := range values {
			destination.Add(key, value)
		}
	}
}
