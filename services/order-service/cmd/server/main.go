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

	goredis "github.com/redis/go-redis/v9"

	_ "github.com/jackc/pgx/v5/stdlib"

	rediscache "trackflow/services/order-service/internal/cache/redis"
	"trackflow/services/order-service/internal/handler"
	"trackflow/services/order-service/internal/repository/postgres"
	"trackflow/services/order-service/internal/service"
)

const (
	serviceName          = "order-service"
	portEnvKey           = "ORDER_SERVICE_PORT"
	dsnEnvKey            = "POSTGRES_DSN"
	redisAddrEnvKey      = "REDIS_ADDR"
	orderCacheTTLEnvKey  = "ORDER_CACHE_TTL"
	defaultPort          = "8082"
	defaultRedisAddr     = "redis:6379"
	defaultOrderCacheTTL = time.Minute
	dialTimeout          = 5 * time.Second
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

	repository := postgres.New(db)
	orderService := service.New(repository)

	redisAddr := strings.TrimSpace(getEnv(redisAddrEnvKey, defaultRedisAddr))
	if redisAddr != "" {
		redisClient := goredis.NewClient(&goredis.Options{Addr: redisAddr})
		defer redisClient.Close()

		redisCtx, redisCancel := context.WithTimeout(context.Background(), dialTimeout)
		defer redisCancel()

		if err := redisClient.Ping(redisCtx).Err(); err != nil {
			logger.Printf("%s redis unavailable (%s): %v; continue without cache", serviceName, redisAddr, err)
		} else {
			cacheTTL := getDurationEnv(logger, orderCacheTTLEnvKey, defaultOrderCacheTTL)
			orderService.SetCache(rediscache.NewOrderCache(redisClient), cacheTTL)
			logger.Printf("%s redis cache enabled: addr=%s ttl=%s", serviceName, redisAddr, cacheTTL)
		}
	}

	router := handler.New(logger, orderService)

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
