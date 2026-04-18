package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
)

const (
	HeaderName            = "X-Request-ID"
	CorrelationHeaderName = "X-Correlation-ID"
)

type contextKey string

const requestIDContextKey contextKey = "request_id"

func Middleware(next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := ResolveFromHeaders(r.Header)
		if requestID == "" {
			requestID = Generate()
		}

		SetHeaders(w.Header(), requestID)
		next.ServeHTTP(w, r.WithContext(WithRequestID(r.Context(), requestID)))
	})
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	id := strings.TrimSpace(requestID)
	if id == "" {
		return ctx
	}

	return context.WithValue(ctx, requestIDContextKey, id)
}

func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}

	requestID, _ := ctx.Value(requestIDContextKey).(string)
	return strings.TrimSpace(requestID)
}

func ResolveFromHeaders(header http.Header) string {
	if header == nil {
		return ""
	}

	requestID := strings.TrimSpace(header.Get(HeaderName))
	if requestID != "" {
		return requestID
	}

	return strings.TrimSpace(header.Get(CorrelationHeaderName))
}

func SetHeaders(header http.Header, requestID string) {
	if header == nil {
		return
	}

	id := strings.TrimSpace(requestID)
	if id == "" {
		return
	}

	header.Set(HeaderName, id)
	header.Set(CorrelationHeaderName, id)
}

func ApplyToRequest(req *http.Request) string {
	if req == nil {
		return ""
	}

	requestID := ResolveFromHeaders(req.Header)
	if requestID == "" {
		requestID = FromContext(req.Context())
	}
	if requestID == "" {
		requestID = Generate()
	}

	SetHeaders(req.Header, requestID)
	return requestID
}

func Generate() string {
	var buffer [12]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "generated-request-id"
	}

	return hex.EncodeToString(buffer[:])
}
