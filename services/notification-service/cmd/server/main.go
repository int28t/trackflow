package main

import (
	"log"
	"net/http"
	"os"
)

const (
	serviceName = "notification-service"
	portEnvKey  = "NOTIFICATION_SERVICE_PORT"
	defaultPort = "8085"
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	port := getEnv(portEnvKey, defaultPort)
	addr := ":" + port

	log.Printf("%s listening on %s", serviceName, addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
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
