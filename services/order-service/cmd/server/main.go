package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"trackflow/services/order-service/internal/handler"
	"trackflow/services/order-service/internal/repository/postgres"
	"trackflow/services/order-service/internal/service"
)

const (
	serviceName = "order-service"
	portEnvKey  = "ORDER_SERVICE_PORT"
	dsnEnvKey   = "POSTGRES_DSN"
	defaultPort = "8082"
	dialTimeout = 5 * time.Second
)

func main() {
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
	router := handler.New(log.Default(), orderService)

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
