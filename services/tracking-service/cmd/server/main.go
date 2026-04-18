package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"

	_ "github.com/jackc/pgx/v5/stdlib"

	rediscache "trackflow/services/tracking-service/internal/cache/redis"
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
	redisAddrEnvKey                     = "REDIS_ADDR"
	timelineCacheTTLEnvKey              = "TRACKING_TIMELINE_CACHE_TTL"
	defaultPort                         = "8083"
	defaultNotificationServiceURL       = "http://notification-service:8085"
	defaultNotificationRequestTimeout   = 3 * time.Second
	defaultRedisAddr                    = "redis:6379"
	defaultTimelineCacheTTL             = 15 * time.Second
	dialTimeout                         = 5 * time.Second
)

func main() {
	logger := configureJSONLogger(serviceName)

	dsn := os.Getenv(dsnEnvKey)
	if dsn == "" {
		logger.Fatalf("%s is required", dsnEnvKey)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		logger.Fatalf("%s failed to init db: %v", serviceName, err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		logger.Fatalf("%s failed to connect to db: %v", serviceName, err)
	}

	notificationClient, err := notification.NewHTTPClient(
		logger,
		getEnv(notificationServiceURLEnvKey, defaultNotificationServiceURL),
		getDurationEnv(notificationRequestTimeoutEnvKey, defaultNotificationRequestTimeout),
		getEnv(notificationEmailRecipientEnvKey, ""),
		getEnv(notificationTelegramRecipientEnvKey, ""),
	)
	if err != nil {
		logger.Fatalf("%s notification configuration error: %v", serviceName, err)
	}

	repository := postgres.New(db)
	trackingService := service.New(repository).SetNotifier(notificationClient)

	redisAddr := strings.TrimSpace(getEnv(redisAddrEnvKey, defaultRedisAddr))
	if redisAddr != "" {
		redisClient := goredis.NewClient(&goredis.Options{Addr: redisAddr})
		defer redisClient.Close()

		redisCtx, redisCancel := context.WithTimeout(context.Background(), dialTimeout)
		defer redisCancel()

		if err := redisClient.Ping(redisCtx).Err(); err != nil {
			logger.Printf("%s redis unavailable (%s): %v; continue without timeline cache", serviceName, redisAddr, err)
		} else {
			cacheTTL := getDurationEnv(timelineCacheTTLEnvKey, defaultTimelineCacheTTL)
			trackingService.
				SetTimelineCache(rediscache.NewTimelineCache(redisClient), cacheTTL).
				SetOrderCacheInvalidator(rediscache.NewOrderCacheInvalidator(redisClient))
			logger.Printf("%s timeline cache enabled: addr=%s ttl=%s", serviceName, redisAddr, cacheTTL)
		}
	}

	router := handler.New(logger, trackingService)

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
