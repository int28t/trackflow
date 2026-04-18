package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"trackflow/services/notification-service/internal/handler"
	"trackflow/services/notification-service/internal/sender"
	"trackflow/services/notification-service/internal/service"
)

const (
	serviceName         = "notification-service"
	portEnvKey          = "NOTIFICATION_SERVICE_PORT"
	providerEnvKey      = "NOTIFICATION_PROVIDER"
	apiKeyEnvKey        = "NOTIFICATION_API_KEY"
	dedupWindowEnvKey   = "NOTIFICATION_DEDUP_WINDOW"
	defaultPort         = "8085"
	defaultProviderMode = "mock"
	defaultDedupWindow  = 24 * time.Hour
)

func main() {
	logger := log.Default()

	provider := strings.ToLower(strings.TrimSpace(getEnv(providerEnvKey, defaultProviderMode)))
	apiKey := strings.TrimSpace(os.Getenv(apiKeyEnvKey))

	messageSender, err := buildSender(logger, provider, apiKey)
	if err != nil {
		log.Fatalf("%s configuration error: %v", serviceName, err)
	}

	notificationService := service.New(messageSender).SetDedupWindow(getDurationEnv(logger, dedupWindowEnvKey, defaultDedupWindow))
	router := handler.New(logger, notificationService)

	port := getEnv(portEnvKey, defaultPort)
	addr := ":" + port

	log.Printf("%s listening on %s", serviceName, addr)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("%s server failed: %v", serviceName, err)
	}
}

func buildSender(logger *log.Logger, provider, apiKey string) (sender.Sender, error) {
	switch provider {
	case "mock":
		return sender.NewMockSender(logger, provider, apiKey), nil
	default:
		return nil, fmt.Errorf("unsupported %s=%q, only mock is supported", providerEnvKey, provider)
	}
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func getDurationEnv(logger *log.Logger, key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		logger.Printf("invalid %s=%q, fallback to %s", key, raw, fallback)
		return fallback
	}

	return value
}
