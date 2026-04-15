package gateway

import (
	"log"
	"net/http"
)

func NewRouter(logger *log.Logger) http.Handler {
	errorHandler := NewErrorHandler(logger)
	mux := http.NewServeMux()

	mux.Handle("/health", Adapt(healthHandler, errorHandler))
	mux.Handle("/v1/orders", Adapt(listOrdersHandler, errorHandler))

	return Chain(
		Recover(errorHandler),
		RequestID(),
		Logging(logger),
	)(mux)
}

func healthHandler(w http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return MethodNotAllowed(r.Method)
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))

	return nil
}

func listOrdersHandler(_ http.ResponseWriter, r *http.Request) error {
	if r.Method != http.MethodGet {
		return MethodNotAllowed(r.Method)
	}

	return NotImplemented("GET /v1/orders")
}
