package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"trackflow/services/carrier-sync-service/internal/client"
	"trackflow/services/carrier-sync-service/internal/handler"
	"trackflow/services/carrier-sync-service/internal/service"
	"trackflow/services/carrier-sync-service/internal/worker"
)

const (
	serviceName               = "carrier-sync-service"
	portEnvKey                = "CARRIER_SYNC_SERVICE_PORT"
	clientModeEnvKey          = "CARRIER_CLIENT_MODE"
	carrierBaseURLEnvKey      = "CARRIER_API_BASE_URL"
	carrierTokenEnvKey        = "CARRIER_API_TOKEN"
	carrierTimeoutEnvKey      = "CARRIER_REQUEST_TIMEOUT"
	trackingServiceURLEnvKey  = "TRACKING_SERVICE_URL"
	trackingTimeoutEnvKey     = "TRACKING_REQUEST_TIMEOUT"
	syncIntervalEnvKey        = "CARRIER_SYNC_INTERVAL"
	defaultPort               = "8084"
	defaultClientMode         = "mock"
	defaultCarrierBaseURL     = "https://carrier.example/api"
	defaultTrackingServiceURL = "http://tracking-service:8083"
	defaultSyncInterval       = 30 * time.Second
	defaultClientBatchSize    = 5
	defaultCarrierTimeout     = 5 * time.Second
	defaultTrackingTimeout    = 5 * time.Second
)

func main() {
	logger := configureJSONLogger(serviceName)

	carrierClient, err := buildCarrierClientFromEnv()
	if err != nil {
		logger.Fatalf("%s configuration error: %v", serviceName, err)
	}

	carrierTimeout := getDurationEnv(logger, carrierTimeoutEnvKey, defaultCarrierTimeout)
	trackingTimeout := getDurationEnv(logger, trackingTimeoutEnvKey, defaultTrackingTimeout)

	trackingClient, err := client.NewTrackingHTTPClient(
		logger,
		getEnv(trackingServiceURLEnvKey, defaultTrackingServiceURL),
		trackingTimeout,
	)
	if err != nil {
		logger.Fatalf("%s configuration error: %v", serviceName, err)
	}

	syncService := service.New(carrierClient)
	syncWorker := worker.
		New(logger, syncService, trackingClient, getSyncInterval(logger), defaultClientBatchSize).
		SetCallTimeouts(carrierTimeout, trackingTimeout)
	router := handler.New(logger, syncService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go syncWorker.Start(ctx)

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

func buildCarrierClientFromEnv() (client.CarrierClient, error) {
	mode := strings.ToLower(strings.TrimSpace(getEnv(clientModeEnvKey, defaultClientMode)))

	switch mode {
	case "mock":
		baseURL := getEnv(carrierBaseURLEnvKey, defaultCarrierBaseURL)
		token := os.Getenv(carrierTokenEnvKey)
		return client.NewMockClient(baseURL, token), nil
	default:
		return nil, fmt.Errorf("unsupported %s=%q, only mock is supported", clientModeEnvKey, mode)
	}
}

func getSyncInterval(logger *log.Logger) time.Duration {
	return getDurationEnv(logger, syncIntervalEnvKey, defaultSyncInterval)
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

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
