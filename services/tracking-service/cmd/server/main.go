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

	"trackflow/services/tracking-service/internal/handler"
	"trackflow/services/tracking-service/internal/repository/postgres"
	"trackflow/services/tracking-service/internal/service"
)

const (
	serviceName = "tracking-service"
	portEnvKey  = "TRACKING_SERVICE_PORT"
	dsnEnvKey   = "POSTGRES_DSN"
	defaultPort = "8083"
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
	trackingService := service.New(repository)
	router := handler.New(log.Default(), trackingService)

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
