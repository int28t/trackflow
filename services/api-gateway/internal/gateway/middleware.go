package gateway

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"strings"
	"time"
)

type Middleware func(http.Handler) http.Handler

func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		wrapped := next

		for i := len(middlewares) - 1; i >= 0; i-- {
			wrapped = middlewares[i](wrapped)
		}

		return wrapped
	}
}

func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := requestIDFromHeaders(r.Header)
			if requestID == "" {
				requestID = generateRequestID()
			}

			setRequestIDHeaders(w.Header(), requestID)
			ctx := context.WithValue(r.Context(), requestIDContextKey, requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func Logging(logger *log.Logger) Middleware {
	if logger == nil {
		logger = log.Default()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(recorder, r)

			logger.Printf(
				"request method=%s path=%s status=%d duration=%s bytes=%d request_id=%s",
				r.Method,
				r.URL.Path,
				recorder.statusCode,
				time.Since(startedAt).String(),
				recorder.bytesWritten,
				getRequestID(r.Context()),
			)
		})
	}
}

func Recover(errorHandler AppErrorHandler) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					errorHandler.Handle(
						w,
						r,
						NewHTTPError(http.StatusInternalServerError, "panic_recovered", "internal server error", nil),
					)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (r *statusRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
	r.ResponseWriter.WriteHeader(statusCode)
}

func (r *statusRecorder) Write(data []byte) (int, error) {
	written, err := r.ResponseWriter.Write(data)
	r.bytesWritten += written

	return written, err
}

func getRequestID(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDContextKey).(string)
	if !ok {
		return ""
	}

	return requestID
}

func generateRequestID() string {
	var buffer [12]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "generated-request-id"
	}

	return hex.EncodeToString(buffer[:])
}

func requestIDFromHeaders(header http.Header) string {
	if header == nil {
		return ""
	}

	requestID := strings.TrimSpace(header.Get(requestIDHeader))
	if requestID != "" {
		return requestID
	}

	return strings.TrimSpace(header.Get(correlationIDHeader))
}

func setRequestIDHeaders(header http.Header, requestID string) {
	if header == nil {
		return
	}

	id := strings.TrimSpace(requestID)
	if id == "" {
		return
	}

	header.Set(requestIDHeader, id)
	header.Set(correlationIDHeader, id)
}
