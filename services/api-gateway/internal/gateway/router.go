package gateway

import (
	"log"
	"net/http"
)

func NewRouter(logger *log.Logger) http.Handler {
	errorHandler := NewErrorHandler(logger)
	proxy := NewGatewayProxy(logger)
	mux := http.NewServeMux()

	mux.Handle("/health", Adapt(healthHandler, errorHandler))
	mux.Handle("/v1/orders", Adapt(proxy.ordersCollection, errorHandler))
	mux.Handle("/v1/orders/{id}", Adapt(proxy.orderByID, errorHandler))
	mux.Handle("/v1/orders/{id}/assign", Adapt(proxy.assignOrder, errorHandler))
	mux.Handle("/v1/orders/{id}/status", Adapt(proxy.updateOrderStatus, errorHandler))
	mux.Handle("/v1/orders/{id}/timeline", Adapt(proxy.getOrderTimeline, errorHandler))

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
