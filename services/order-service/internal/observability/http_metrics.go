package observability

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"trackflow/services/order-service/internal/requestid"
)

type metricKey struct {
	Method string
	Route  string
	Status int
}

type metricAggregate struct {
	Count        uint64
	TotalLatency time.Duration
	MaxLatency   time.Duration
}

type routeMetric struct {
	Method       string  `json:"method"`
	Route        string  `json:"route"`
	Status       int     `json:"status"`
	Requests     uint64  `json:"requests"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	MaxLatencyMs float64 `json:"max_latency_ms"`
}

type metricsSnapshot struct {
	UptimeSeconds int64         `json:"uptime_seconds"`
	RequestsTotal uint64        `json:"requests_total"`
	AvgLatencyMs  float64       `json:"avg_latency_ms"`
	Routes        []routeMetric `json:"routes"`
}

type HTTPMetrics struct {
	logger       *log.Logger
	startedAt    time.Time
	mu           sync.RWMutex
	totalCount   uint64
	totalLatency time.Duration
	byKey        map[metricKey]metricAggregate
}

func NewHTTPMetrics(logger *log.Logger) *HTTPMetrics {
	if logger == nil {
		logger = log.Default()
	}

	return &HTTPMetrics{
		logger:    logger,
		startedAt: time.Now(),
		byKey:     make(map[metricKey]metricAggregate),
	}
}

func (m *HTTPMetrics) Middleware(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(recorder, r)

		duration := time.Since(started)
		route := routePattern(r)
		m.observe(r.Method, route, recorder.statusCode, duration)

		m.logger.Printf(
			"http request completed method=%s route=%s status=%d latency_ms=%.3f request_id=%s",
			r.Method,
			route,
			recorder.statusCode,
			latencyMs(duration),
			requestid.FromContext(r.Context()),
		)
	})
}

func (m *HTTPMetrics) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		snapshot := m.snapshot()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(snapshot)
	})
}

func (m *HTTPMetrics) observe(method, route string, status int, duration time.Duration) {
	if m == nil {
		return
	}

	key := metricKey{Method: method, Route: route, Status: status}

	m.mu.Lock()
	defer m.mu.Unlock()

	agg := m.byKey[key]
	agg.Count++
	agg.TotalLatency += duration
	if duration > agg.MaxLatency {
		agg.MaxLatency = duration
	}

	m.byKey[key] = agg
	m.totalCount++
	m.totalLatency += duration
}

func (m *HTTPMetrics) snapshot() metricsSnapshot {
	if m == nil {
		return metricsSnapshot{}
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	routes := make([]routeMetric, 0, len(m.byKey))
	for key, agg := range m.byKey {
		routes = append(routes, routeMetric{
			Method:       key.Method,
			Route:        key.Route,
			Status:       key.Status,
			Requests:     agg.Count,
			AvgLatencyMs: averageLatencyMs(agg.TotalLatency, agg.Count),
			MaxLatencyMs: latencyMs(agg.MaxLatency),
		})
	}

	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Route != routes[j].Route {
			return routes[i].Route < routes[j].Route
		}
		if routes[i].Method != routes[j].Method {
			return routes[i].Method < routes[j].Method
		}
		return routes[i].Status < routes[j].Status
	})

	return metricsSnapshot{
		UptimeSeconds: int64(time.Since(m.startedAt).Seconds()),
		RequestsTotal: m.totalCount,
		AvgLatencyMs:  averageLatencyMs(m.totalLatency, m.totalCount),
		Routes:        routes,
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func routePattern(r *http.Request) string {
	if r == nil {
		return "unknown"
	}

	pattern := strings.TrimSpace(r.Pattern)
	if pattern != "" {
		return pattern
	}

	if r.URL != nil {
		path := strings.TrimSpace(r.URL.Path)
		if path != "" {
			return path
		}
	}

	return "unknown"
}

func latencyMs(duration time.Duration) float64 {
	return float64(duration.Microseconds()) / 1000.0
}

func averageLatencyMs(total time.Duration, count uint64) float64 {
	if count == 0 {
		return 0
	}

	return latencyMs(total) / float64(count)
}
