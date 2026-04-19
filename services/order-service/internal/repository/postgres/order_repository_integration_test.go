package postgres

import (
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

	"trackflow/services/order-service/internal/model"
	"trackflow/services/order-service/internal/service"
)

const defaultTestPostgresDSN = "postgres://trackflow:trackflow@localhost:5432/trackflow?sslmode=disable"

func TestOrderRepositoryIntegrationCreateReadAndList(t *testing.T) {
	db := prepareIntegrationDB(t, "order_repo")
	repo := New(db)
	ctx := context.Background()

	createdOne, err := repo.CreateOrder(ctx, validCreateOrderInput(), uniqueIdempotencyKey("create-read-1"))
	if err != nil {
		t.Fatalf("CreateOrder first returned error: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	createdTwo, err := repo.CreateOrder(ctx, validCreateOrderInput(), uniqueIdempotencyKey("create-read-2"))
	if err != nil {
		t.Fatalf("CreateOrder second returned error: %v", err)
	}

	fromID, err := repo.GetOrderByID(ctx, createdOne.ID)
	if err != nil {
		t.Fatalf("GetOrderByID returned error: %v", err)
	}

	if fromID.ID != createdOne.ID {
		t.Fatalf("unexpected order from id lookup: got %q, want %q", fromID.ID, createdOne.ID)
	}

	fromKey, err := repo.GetOrderByIdempotencyKey(ctx, uniqueIdempotencyKey("create-read-1"))
	if err != nil {
		t.Fatalf("GetOrderByIdempotencyKey returned error: %v", err)
	}

	if fromKey.ID != createdOne.ID {
		t.Fatalf("unexpected order from idempotency lookup: got %q, want %q", fromKey.ID, createdOne.ID)
	}

	listed, err := repo.ListOrders(ctx, 1)
	if err != nil {
		t.Fatalf("ListOrders returned error: %v", err)
	}

	if len(listed) != 1 {
		t.Fatalf("unexpected listed length: got %d, want %d", len(listed), 1)
	}

	if listed[0].ID != createdTwo.ID {
		t.Fatalf("unexpected latest order from list: got %q, want %q", listed[0].ID, createdTwo.ID)
	}
}

func TestOrderRepositoryIntegrationDuplicateIdempotency(t *testing.T) {
	db := prepareIntegrationDB(t, "order_repo")
	repo := New(db)
	ctx := context.Background()

	idempotencyKey := uniqueIdempotencyKey("duplicate")
	if _, err := repo.CreateOrder(ctx, validCreateOrderInput(), idempotencyKey); err != nil {
		t.Fatalf("first CreateOrder returned error: %v", err)
	}

	_, err := repo.CreateOrder(ctx, validCreateOrderInput(), idempotencyKey)
	if !errors.Is(err, service.ErrDuplicateIdempotency) {
		t.Fatalf("expected ErrDuplicateIdempotency, got %v", err)
	}
}

func TestOrderRepositoryIntegrationGetOrderByIDNotFound(t *testing.T) {
	db := prepareIntegrationDB(t, "order_repo")
	repo := New(db)

	_, err := repo.GetOrderByID(context.Background(), "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	if !errors.Is(err, service.ErrOrderNotFound) {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func TestOrderRepositoryIntegrationAssignOrderFlow(t *testing.T) {
	db := prepareIntegrationDB(t, "order_repo")
	repo := New(db)
	ctx := context.Background()

	activeCourierID := "cccccccc-cccc-cccc-cccc-cccccccccccc"
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO couriers (id, name, vehicle_type, is_active) VALUES ($1::uuid, 'Integration Courier', 'car', TRUE)`,
		activeCourierID,
	); err != nil {
		t.Fatalf("insert active courier failed: %v", err)
	}

	createdOrder, err := repo.CreateOrder(ctx, validCreateOrderInput(), uniqueIdempotencyKey("assign-created"))
	if err != nil {
		t.Fatalf("CreateOrder for assign returned error: %v", err)
	}

	assigned, err := repo.AssignOrder(ctx, createdOrder.ID, model.AssignOrderInput{
		CourierID:  activeCourierID,
		AssignedBy: "integration",
		Comment:    "assigned by integration test",
	})
	if err != nil {
		t.Fatalf("AssignOrder returned error: %v", err)
	}

	if assigned.Status != "assigned" {
		t.Fatalf("unexpected assigned status: got %q, want %q", assigned.Status, "assigned")
	}

	_, err = repo.AssignOrder(ctx, createdOrder.ID, model.AssignOrderInput{CourierID: activeCourierID})
	if !errors.Is(err, service.ErrOrderAlreadyAssigned) {
		t.Fatalf("expected ErrOrderAlreadyAssigned, got %v", err)
	}

	secondOrder, err := repo.CreateOrder(ctx, validCreateOrderInput(), uniqueIdempotencyKey("assign-missing-courier"))
	if err != nil {
		t.Fatalf("CreateOrder second returned error: %v", err)
	}

	_, err = repo.AssignOrder(ctx, secondOrder.ID, model.AssignOrderInput{CourierID: "dddddddd-dddd-dddd-dddd-dddddddddddd"})
	if !errors.Is(err, service.ErrCourierNotFound) {
		t.Fatalf("expected ErrCourierNotFound, got %v", err)
	}

	thirdOrder, err := repo.CreateOrder(ctx, validCreateOrderInput(), uniqueIdempotencyKey("assign-not-allowed"))
	if err != nil {
		t.Fatalf("CreateOrder third returned error: %v", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`UPDATE orders SET status = 'delivered' WHERE id = $1::uuid`,
		thirdOrder.ID,
	); err != nil {
		t.Fatalf("update order to delivered failed: %v", err)
	}

	_, err = repo.AssignOrder(ctx, thirdOrder.ID, model.AssignOrderInput{CourierID: activeCourierID})
	if !errors.Is(err, service.ErrAssignmentNotAllowed) {
		t.Fatalf("expected ErrAssignmentNotAllowed, got %v", err)
	}
}

func validCreateOrderInput() model.CreateOrderInput {
	return model.CreateOrderInput{
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		PickupAddress: model.AddressInput{
			City:      "Moscow",
			Street:    "Tverskaya",
			House:     "1",
			Apartment: "10",
			Lat:       55.7558,
			Lng:       37.6176,
		},
		DropoffAddress: model.AddressInput{
			City:      "Moscow",
			Street:    "Arbat",
			House:     "5",
			Apartment: "12",
			Lat:       55.7520,
			Lng:       37.5929,
		},
		WeightKG:     1.5,
		DistanceKM:   3.2,
		ServiceLevel: "standard",
	}
}

func uniqueIdempotencyKey(prefix string) string {
	return fmt.Sprintf("it-%s", prefix)
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
