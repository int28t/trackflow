package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"trackflow/services/tracking-service/internal/service"
)

const defaultTestPostgresDSN = "postgres://trackflow:trackflow@localhost:5432/trackflow?sslmode=disable"

func TestTrackingRepositoryIntegrationGetOrderTimeline(t *testing.T) {
	db := prepareIntegrationDB(t, "tracking_repo")
	repo := New(db)
	ctx := context.Background()

	orderID := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	if err := seedOrder(ctx, db, orderID, "created"); err != nil {
		t.Fatalf("seed order failed: %v", err)
	}

	if err := seedStatusHistory(ctx, db, orderID, "created", "system", "seed created", []byte(`{"stage":1}`), time.Now().Add(-2*time.Minute)); err != nil {
		t.Fatalf("seed first status history failed: %v", err)
	}

	if err := seedStatusHistory(ctx, db, orderID, "assigned", "manager", "seed assigned", []byte(`{"stage":2}`), time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("seed second status history failed: %v", err)
	}

	items, err := repo.GetOrderTimeline(ctx, orderID, 10)
	if err != nil {
		t.Fatalf("GetOrderTimeline returned error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("unexpected timeline length: got %d, want %d", len(items), 2)
	}

	if items[0].Status != "created" || items[1].Status != "assigned" {
		t.Fatalf("unexpected timeline order: got [%s, %s]", items[0].Status, items[1].Status)
	}

	if items[0].Comment == nil || *items[0].Comment != "seed created" {
		t.Fatalf("unexpected first comment: %+v", items[0].Comment)
	}

	if !bytes.Equal(items[1].Metadata, []byte(`{"stage":2}`)) {
		t.Fatalf("unexpected metadata in second item: got %s", string(items[1].Metadata))
	}
}

func TestTrackingRepositoryIntegrationGetOrderTimelineEmptyAndNotFound(t *testing.T) {
	db := prepareIntegrationDB(t, "tracking_repo")
	repo := New(db)
	ctx := context.Background()

	existingOrderID := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	if err := seedOrder(ctx, db, existingOrderID, "created"); err != nil {
		t.Fatalf("seed order failed: %v", err)
	}

	items, err := repo.GetOrderTimeline(ctx, existingOrderID, 10)
	if err != nil {
		t.Fatalf("GetOrderTimeline for existing empty order returned error: %v", err)
	}

	if len(items) != 0 {
		t.Fatalf("expected empty timeline for existing order, got %d items", len(items))
	}

	_, err = repo.GetOrderTimeline(ctx, "cccccccc-cccc-cccc-cccc-cccccccccccc", 10)
	if !errors.Is(err, service.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func TestTrackingRepositoryIntegrationUpdateOrderStatus(t *testing.T) {
	db := prepareIntegrationDB(t, "tracking_repo")
	repo := New(db)
	ctx := context.Background()

	orderID := "dddddddd-dddd-dddd-dddd-dddddddddddd"
	if err := seedOrder(ctx, db, orderID, "created"); err != nil {
		t.Fatalf("seed order failed: %v", err)
	}

	metadata := []byte(`{"actor":"integration"}`)
	item, err := repo.UpdateOrderStatus(ctx, orderID, "assigned", "manager", "assigned by integration", metadata)
	if err != nil {
		t.Fatalf("UpdateOrderStatus returned error: %v", err)
	}

	if item.OrderID != orderID {
		t.Fatalf("unexpected order id in history item: got %q, want %q", item.OrderID, orderID)
	}

	if item.Status != "assigned" {
		t.Fatalf("unexpected status in history item: got %q, want %q", item.Status, "assigned")
	}

	var currentStatus string
	if err := db.QueryRowContext(ctx, `SELECT status::text FROM orders WHERE id = $1::uuid`, orderID).Scan(&currentStatus); err != nil {
		t.Fatalf("query updated order status failed: %v", err)
	}

	if currentStatus != "assigned" {
		t.Fatalf("unexpected order status in table: got %q, want %q", currentStatus, "assigned")
	}
}

func TestTrackingRepositoryIntegrationUpdateOrderStatusValidationAndNotFound(t *testing.T) {
	db := prepareIntegrationDB(t, "tracking_repo")
	repo := New(db)
	ctx := context.Background()

	orderID := "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	if err := seedOrder(ctx, db, orderID, "created"); err != nil {
		t.Fatalf("seed order failed: %v", err)
	}

	_, err := repo.UpdateOrderStatus(ctx, orderID, "delivered", "manager", "invalid transition", []byte(`{"reason":"skip"}`))
	if !errors.Is(err, service.ErrStatusTransitionNotAllowed) {
		t.Fatalf("expected ErrStatusTransitionNotAllowed, got %v", err)
	}

	_, err = repo.UpdateOrderStatus(ctx, "ffffffff-ffff-ffff-ffff-ffffffffffff", "assigned", "manager", "missing order", nil)
	if !errors.Is(err, service.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func seedOrder(ctx context.Context, db *sql.DB, orderID, status string) error {
	pickupAddressID := "11111111-1111-1111-1111-111111111111"
	dropoffAddressID := "22222222-2222-2222-2222-222222222222"
	customerID := "33333333-3333-3333-3333-333333333333"
	idempotencyKey := "it-" + orderID

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO addresses (id, city, street, house, lat, lng) VALUES ($1::uuid, 'Moscow', 'Tverskaya', '1', 55.7558, 37.6176)`,
		pickupAddressID,
	); err != nil {
		return err
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO addresses (id, city, street, house, lat, lng) VALUES ($1::uuid, 'Moscow', 'Arbat', '5', 55.7520, 37.5929)`,
		dropoffAddressID,
	); err != nil {
		return err
	}

	_, err := db.ExecContext(
		ctx,
		`INSERT INTO orders (
			id,
			customer_id,
			pickup_address_id,
			dropoff_address_id,
			weight_kg,
			distance_km,
			service_level,
			status,
			idempotency_key
		) VALUES (
			$1::uuid,
			$2::uuid,
			$3::uuid,
			$4::uuid,
			1.5,
			3.2,
			'standard',
			$5::order_status_t,
			$6
		)`,
		orderID,
		customerID,
		pickupAddressID,
		dropoffAddressID,
		status,
		idempotencyKey,
	)

	return err
}

func seedStatusHistory(ctx context.Context, db *sql.DB, orderID, status, source, comment string, metadata []byte, createdAt time.Time) error {
	_, err := db.ExecContext(
		ctx,
		`INSERT INTO order_status_history (order_id, status, source, comment, metadata, created_at)
		 VALUES ($1::uuid, $2::order_status_t, $3::status_source_t, NULLIF($4, ''), $5::jsonb, $6)`,
		orderID,
		status,
		source,
		comment,
		metadata,
		createdAt,
	)

	return err
}

func prepareIntegrationDB(t *testing.T, prefix string) *sql.DB {
	t.Helper()

	baseDSN := strings.TrimSpace(os.Getenv("TEST_POSTGRES_DSN"))
	if baseDSN == "" {
		baseDSN = defaultTestPostgresDSN
	}

	adminDSN, err := withDatabaseName(baseDSN, "postgres")
	if err != nil {
		t.Fatalf("build admin dsn failed: %v", err)
	}

	adminDB, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Skipf("skip integration tests: cannot open postgres admin connection: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := adminDB.PingContext(ctx); err != nil {
		_ = adminDB.Close()
		t.Skipf("skip integration tests: postgres is unavailable (%s): %v", adminDSN, err)
	}

	databaseName := fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE "+quoteIdentifier(databaseName)); err != nil {
		_ = adminDB.Close()
		t.Fatalf("create integration database failed: %v", err)
	}

	testDSN, err := withDatabaseName(baseDSN, databaseName)
	if err != nil {
		_ = adminDB.Close()
		t.Fatalf("build test dsn failed: %v", err)
	}

	testDB, err := sql.Open("pgx", testDSN)
	if err != nil {
		_ = adminDB.Close()
		t.Fatalf("open integration database failed: %v", err)
	}

	setupCtx, setupCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer setupCancel()
	if err := testDB.PingContext(setupCtx); err != nil {
		_ = testDB.Close()
		_ = adminDB.Close()
		t.Fatalf("ping integration database failed: %v", err)
	}

	schemaSQL, err := readSchemaSQL()
	if err != nil {
		_ = testDB.Close()
		_ = adminDB.Close()
		t.Fatalf("read schema SQL failed: %v", err)
	}

	if _, err := testDB.ExecContext(setupCtx, schemaSQL); err != nil {
		_ = testDB.Close()
		_ = adminDB.Close()
		t.Fatalf("apply schema failed: %v", err)
	}

	t.Cleanup(func() {
		_ = testDB.Close()

		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		_, _ = adminDB.ExecContext(
			cleanupCtx,
			`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()`,
			databaseName,
		)
		_, _ = adminDB.ExecContext(cleanupCtx, "DROP DATABASE IF EXISTS "+quoteIdentifier(databaseName))
		_ = adminDB.Close()
	})

	return testDB
}

func readSchemaSQL() (string, error) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("resolve current file path")
	}

	schemaPath := filepath.Clean(filepath.Join(
		filepath.Dir(currentFile),
		"..", "..", "..", "..", "..",
		"migrations", "postgres", "schema-v1.sql",
	))

	content, err := os.ReadFile(schemaPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func withDatabaseName(baseDSN, databaseName string) (string, error) {
	parsed, err := url.Parse(baseDSN)
	if err != nil {
		return "", err
	}

	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
