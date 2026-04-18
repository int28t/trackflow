package gateway

import (
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

	upstreamResp, err := p.client.Do(upstreamReq)
	if err != nil {
		return NewHTTPError(http.StatusBadGateway, "upstream_unavailable", "upstream service is unavailable", err)
	}
	defer upstreamResp.Body.Close()

	copyHeaders(w.Header(), upstreamResp.Header)
	w.WriteHeader(upstreamResp.StatusCode)

	if _, err := io.Copy(w, upstreamResp.Body); err != nil {
		p.logger.Printf("upstream response copy failed method=%s path=%s err=%v", r.Method, r.URL.Path, err)
		return nil
	}

	return nil
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
