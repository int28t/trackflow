package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"trackflow/services/tracking-service/internal/handler"
	"trackflow/services/tracking-service/internal/notification"
	"trackflow/services/tracking-service/internal/repository/postgres"
	"trackflow/services/tracking-service/internal/service"
)

const (
	serviceName                         = "tracking-service"
	portEnvKey                          = "TRACKING_SERVICE_PORT"
	dsnEnvKey                           = "POSTGRES_DSN"
	notificationServiceURLEnvKey        = "NOTIFICATION_SERVICE_URL"
	notificationRequestTimeoutEnvKey    = "NOTIFICATION_REQUEST_TIMEOUT"
	notificationEmailRecipientEnvKey    = "NOTIFICATION_EMAIL_RECIPIENT"
	notificationTelegramRecipientEnvKey = "NOTIFICATION_TELEGRAM_RECIPIENT"
	defaultPort                         = "8083"
	defaultNotificationServiceURL       = "http://notification-service:8085"
	defaultNotificationRequestTimeout   = 3 * time.Second
	dialTimeout                         = 5 * time.Second
)

func main() {
	logger := log.Default()

	dsn := os.Getenv(dsnEnvKey)
	if dsn == "" {
		log.Fatalf("%s is required", dsnEnvKey)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("%s failed to init db: %v", serviceName, err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("%s failed to connect to db: %v", serviceName, err)
	}

	notificationClient, err := notification.NewHTTPClient(
		logger,
		getEnv(notificationServiceURLEnvKey, defaultNotificationServiceURL),
		getDurationEnv(notificationRequestTimeoutEnvKey, defaultNotificationRequestTimeout),
		getEnv(notificationEmailRecipientEnvKey, ""),
		getEnv(notificationTelegramRecipientEnvKey, ""),
	)
	if err != nil {
		log.Fatalf("%s notification configuration error: %v", serviceName, err)
	}

	repository := postgres.New(db)
	trackingService := service.New(repository).SetNotifier(notificationClient)
	router := handler.New(logger, trackingService)

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

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		log.Printf("invalid %s=%q, fallback to %s", key, raw, fallback)
		return fallback
	}

	return value
}
