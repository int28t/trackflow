package main

import (
	"context"
	"errors"
	"fmt"
	"log"
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
	trackingServiceURLEnvKey  = "TRACKING_SERVICE_URL"
	syncIntervalEnvKey        = "CARRIER_SYNC_INTERVAL"
	defaultPort               = "8084"
	defaultClientMode         = "mock"
	defaultCarrierBaseURL     = "https://carrier.example/api"
	defaultTrackingServiceURL = "http://tracking-service:8083"
	defaultSyncInterval       = 30 * time.Second
	defaultClientBatchSize    = 5
	trackingRequestTimeout    = 5 * time.Second
)

func main() {
	logger := log.Default()

	carrierClient, err := buildCarrierClientFromEnv()
	if err != nil {
		log.Fatalf("%s configuration error: %v", serviceName, err)
	}

	trackingClient, err := client.NewTrackingHTTPClient(
		logger,
		getEnv(trackingServiceURLEnvKey, defaultTrackingServiceURL),
		trackingRequestTimeout,
	)
	if err != nil {
		log.Fatalf("%s configuration error: %v", serviceName, err)
	}

	syncService := service.New(carrierClient)
	syncWorker := worker.New(logger, syncService, trackingClient, getSyncInterval(logger), defaultClientBatchSize)
	router := handler.New(logger, syncService)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go syncWorker.Start(ctx)

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
	raw := strings.TrimSpace(os.Getenv(syncIntervalEnvKey))
	if raw == "" {
		return defaultSyncInterval
	}

	interval, err := time.ParseDuration(raw)
	if err != nil || interval <= 0 {
		logger.Printf("invalid %s=%q, fallback to %s", syncIntervalEnvKey, raw, defaultSyncInterval)
		return defaultSyncInterval
	}

	return interval
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}
