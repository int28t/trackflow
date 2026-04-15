package postgres

import (
	"context"
	"database/sql"
	"errors"

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
