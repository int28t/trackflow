package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"

	"trackflow/services/order-service/internal/model"
	"trackflow/services/order-service/internal/service"
)

const listOrdersQuery = `
SELECT
	id::text,
	customer_id::text,
	status::text,
	created_at,
	updated_at
FROM orders
ORDER BY created_at DESC
LIMIT $1
`

const getOrderByIDQuery = `
SELECT
	id::text,
	customer_id::text,
	status::text,
	created_at,
	updated_at
FROM orders
WHERE id = $1::uuid
LIMIT 1
`

const getOrderByIdempotencyKeyQuery = `
SELECT
	id::text,
	customer_id::text,
	status::text,
	created_at,
	updated_at
FROM orders
WHERE idempotency_key = $1
LIMIT 1
`

const selectOrderStatusForUpdateQuery = `
SELECT status::text
FROM orders
WHERE id = $1::uuid
FOR UPDATE
`

const isCourierActiveQuery = `
SELECT EXISTS(
	SELECT 1
	FROM couriers
	WHERE id = $1::uuid AND is_active = TRUE
)
`

const insertAssignmentQuery = `
INSERT INTO assignments (
	order_id,
	courier_id,
	assigned_by,
	comment
)
VALUES (
	$1::uuid,
	$2::uuid,
	NULLIF($3, ''),
	NULLIF($4, '')
)
`

const updateOrderStatusToAssignedQuery = `
UPDATE orders
SET status = 'assigned',
	last_status_at = NOW()
WHERE id = $1::uuid
RETURNING
	id::text,
	customer_id::text,
	status::text,
	created_at,
	updated_at
`

const insertAssignedStatusHistoryQuery = `
INSERT INTO order_status_history (
	order_id,
	status,
	source,
	comment,
	metadata
)
VALUES (
	$1::uuid,
	'assigned',
	'manager',
	NULLIF($2, ''),
	jsonb_build_object('courier_id', $3::uuid)
)
`

const insertAddressQuery = `
INSERT INTO addresses (
	city,
	street,
	house,
	apartment,
	lat,
	lng
)
VALUES ($1, $2, $3, NULLIF($4, ''), $5, $6)
RETURNING id::text
`

const insertOrderQuery = `
INSERT INTO orders (
	customer_id,
	pickup_address_id,
	dropoff_address_id,
	weight_kg,
	distance_km,
	service_level,
	idempotency_key
)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6::service_level_t, $7)
RETURNING
	id::text,
	customer_id::text,
	status::text,
	created_at,
	updated_at
`

type OrderRepository struct {
	db *sql.DB
}

var _ service.Repository = (*OrderRepository)(nil)

func New(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Ping(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("database is not configured")
	}

	return r.db.PingContext(ctx)
}

func (r *OrderRepository) ListOrders(ctx context.Context, limit int) ([]model.Order, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("database is not configured")
	}

	rows, err := r.db.QueryContext(ctx, listOrdersQuery, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := make([]model.Order, 0, limit)
	for rows.Next() {
		var order model.Order
		if err := rows.Scan(
			&order.ID,
			&order.CustomerID,
			&order.Status,
			&order.CreatedAt,
			&order.UpdatedAt,
		); err != nil {
			return nil, err
		}

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

func (r *OrderRepository) GetOrderByID(ctx context.Context, orderID string) (model.Order, error) {
	if r == nil || r.db == nil {
		return model.Order{}, errors.New("database is not configured")
	}

	row := r.db.QueryRowContext(ctx, getOrderByIDQuery, orderID)
	order, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Order{}, service.ErrOrderNotFound
		}

		return model.Order{}, err
	}

	return order, nil
}

func (r *OrderRepository) AssignOrder(ctx context.Context, orderID string, input model.AssignOrderInput) (model.Order, error) {
	if r == nil || r.db == nil {
		return model.Order{}, errors.New("database is not configured")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Order{}, err
	}

	defer func() {
		_ = tx.Rollback()
	}()

	var currentStatus string
	statusRow := tx.QueryRowContext(ctx, selectOrderStatusForUpdateQuery, orderID)
	if err := statusRow.Scan(&currentStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Order{}, service.ErrOrderNotFound
		}

		return model.Order{}, err
	}

	if currentStatus == "assigned" {
		return model.Order{}, service.ErrOrderAlreadyAssigned
	}

	if currentStatus != "created" {
		return model.Order{}, service.ErrAssignmentNotAllowed
	}

	var courierActive bool
	courierRow := tx.QueryRowContext(ctx, isCourierActiveQuery, input.CourierID)
	if err := courierRow.Scan(&courierActive); err != nil {
		return model.Order{}, err
	}

	if !courierActive {
		return model.Order{}, service.ErrCourierNotFound
	}

	if _, err := tx.ExecContext(
		ctx,
		insertAssignmentQuery,
		orderID,
		input.CourierID,
		input.AssignedBy,
		input.Comment,
	); err != nil {
		if isAssignmentConflict(err) {
			return model.Order{}, service.ErrOrderAlreadyAssigned
		}

		return model.Order{}, err
	}

	orderRow := tx.QueryRowContext(ctx, updateOrderStatusToAssignedQuery, orderID)
	order, err := scanOrder(orderRow)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Order{}, service.ErrOrderNotFound
		}

		return model.Order{}, err
	}

	if _, err := tx.ExecContext(
		ctx,
		insertAssignedStatusHistoryQuery,
		orderID,
		input.Comment,
		input.CourierID,
	); err != nil {
		return model.Order{}, err
	}

	if err := tx.Commit(); err != nil {
		if isAssignmentConflict(err) {
			return model.Order{}, service.ErrOrderAlreadyAssigned
		}

		return model.Order{}, err
	}

	return order, nil
}

func (r *OrderRepository) GetOrderByIdempotencyKey(ctx context.Context, idempotencyKey string) (model.Order, error) {
	if r == nil || r.db == nil {
		return model.Order{}, errors.New("database is not configured")
	}

	row := r.db.QueryRowContext(ctx, getOrderByIdempotencyKeyQuery, idempotencyKey)
	order, err := scanOrder(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Order{}, service.ErrOrderNotFound
		}

		return model.Order{}, err
	}

	return order, nil
}

func (r *OrderRepository) CreateOrder(ctx context.Context, input model.CreateOrderInput, idempotencyKey string) (model.Order, error) {
	if r == nil || r.db == nil {
		return model.Order{}, errors.New("database is not configured")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.Order{}, err
	}

	defer func() {
		_ = tx.Rollback()
	}()

	pickupAddressID, err := insertAddress(ctx, tx, input.PickupAddress)
	if err != nil {
		return model.Order{}, err
	}

	dropoffAddressID, err := insertAddress(ctx, tx, input.DropoffAddress)
	if err != nil {
		return model.Order{}, err
	}

	orderRow := tx.QueryRowContext(
		ctx,
		insertOrderQuery,
		input.CustomerID,
		pickupAddressID,
		dropoffAddressID,
		input.WeightKG,
		input.DistanceKM,
		input.ServiceLevel,
		idempotencyKey,
	)

	order, err := scanOrder(orderRow)
	if err != nil {
		if isIdempotencyConflict(err) {
			return model.Order{}, service.ErrDuplicateIdempotency
		}

		return model.Order{}, err
	}

	if err := tx.Commit(); err != nil {
		if isIdempotencyConflict(err) {
			return model.Order{}, service.ErrDuplicateIdempotency
		}

		return model.Order{}, err
	}

	return order, nil
}

func insertAddress(ctx context.Context, tx *sql.Tx, input model.AddressInput) (string, error) {
	row := tx.QueryRowContext(
		ctx,
		insertAddressQuery,
		input.City,
		input.Street,
		input.House,
		input.Apartment,
		input.Lat,
		input.Lng,
	)

	var addressID string
	if err := row.Scan(&addressID); err != nil {
		return "", err
	}

	return addressID, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanOrder(row rowScanner) (model.Order, error) {
	var order model.Order
	err := row.Scan(
		&order.ID,
		&order.CustomerID,
		&order.Status,
		&order.CreatedAt,
		&order.UpdatedAt,
	)
	if err != nil {
		return model.Order{}, err
	}

	return order, nil
}

func isIdempotencyConflict(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	if pgErr.Code != "23505" {
		return false
	}

	return strings.EqualFold(pgErr.ConstraintName, "uq_orders_idempotency_key")
}

func isAssignmentConflict(err error) bool {
	if err == nil {
		return false
	}

	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	if pgErr.Code != "23505" {
		return false
	}

	return strings.EqualFold(pgErr.ConstraintName, "uq_assignments_order_id")
}
