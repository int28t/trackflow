package main

import (
	"errors"
	"log"
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
	router := gateway.NewRouter(log.Default())

	port := getEnv(portEnvKey, defaultPort)
	addr := ":" + port

	log.Printf("%s listening on %s", serviceName, addr)

	server := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("%s server failed: %v", serviceName, err)
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
