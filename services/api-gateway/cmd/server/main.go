package main

import (
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"

	"trackflow/services/api-gateway/internal/gateway"
)

const (
	serviceName = "api-gateway"
	portEnvKey  = "API_GATEWAY_PORT"
	defaultPort = "8081"
)

func main() {
	logger := configureJSONLogger(serviceName)
	router := gateway.NewRouter(logger)

	port := getEnv(portEnvKey, defaultPort)
	addr := ":" + port

	logger.Printf("%s listening on %s", serviceName, addr)

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Fatalf("%s server failed: %v", serviceName, err)
	}
}

func configureJSONLogger(service string) *log.Logger {
	base := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", service)
	structured := slog.NewLogLogger(base.Handler(), slog.LevelInfo)
	structured.SetFlags(0)

	log.SetFlags(0)
	log.SetOutput(structured.Writer())

	return structured
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
