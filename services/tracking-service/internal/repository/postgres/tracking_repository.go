package postgres

import (
	"context"
	"database/sql"
	"errors"

	"trackflow/services/tracking-service/internal/model"
	"trackflow/services/tracking-service/internal/service"
)

const getOrderTimelineQuery = `
SELECT
	id::text,
	order_id::text,
	status::text,
	source::text,
	comment,
	metadata,
	created_at
FROM order_status_history
WHERE order_id = $1::uuid
ORDER BY created_at ASC
LIMIT $2
`

const selectOrderStatusForUpdateQuery = `
SELECT status::text
FROM orders
WHERE id = $1::uuid
FOR UPDATE
`

const updateOrderStatusQuery = `
UPDATE orders
SET status = $2::order_status_t,
	last_status_at = NOW()
WHERE id = $1::uuid
`

const insertStatusHistoryQuery = `
INSERT INTO order_status_history (
	order_id,
	status,
	source,
	comment,
	metadata
)
VALUES (
	$1::uuid,
	$2::order_status_t,
	$3::status_source_t,
	NULLIF($4, ''),
	$5::jsonb
)
RETURNING
	id::text,
	order_id::text,
	status::text,
	source::text,
	comment,
	metadata,
	created_at
`

type TrackingRepository struct {
	db *sql.DB
}

var _ service.Repository = (*TrackingRepository)(nil)

func New(db *sql.DB) *TrackingRepository {
	return &TrackingRepository{db: db}
}

func (r *TrackingRepository) Ping(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("database is not configured")
	}

	return r.db.PingContext(ctx)
}

func (r *TrackingRepository) GetOrderTimeline(ctx context.Context, orderID string, limit int) ([]model.StatusHistoryItem, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("database is not configured")
	}

	rows, err := r.db.QueryContext(ctx, getOrderTimelineQuery, orderID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.StatusHistoryItem, 0, limit)
	for rows.Next() {
		var item model.StatusHistoryItem
		var comment sql.NullString
		var metadata []byte

		if err := rows.Scan(
			&item.ID,
			&item.OrderID,
			&item.Status,
			&item.Source,
			&comment,
			&metadata,
			&item.CreatedAt,
		); err != nil {
			return nil, err
		}

		if comment.Valid {
			itemComment := comment.String
			item.Comment = &itemComment
		}

		if len(metadata) > 0 {
			item.Metadata = append(item.Metadata[:0], metadata...)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *TrackingRepository) UpdateOrderStatus(ctx context.Context, orderID, nextStatus, source, comment string, metadata []byte) (model.StatusHistoryItem, error) {
	if r == nil || r.db == nil {
		return model.StatusHistoryItem{}, errors.New("database is not configured")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return model.StatusHistoryItem{}, err
	}

	defer func() {
		_ = tx.Rollback()
	}()

	var currentStatus string
	statusRow := tx.QueryRowContext(ctx, selectOrderStatusForUpdateQuery, orderID)
	if err := statusRow.Scan(&currentStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.StatusHistoryItem{}, service.ErrOrderNotFound
		}

		return model.StatusHistoryItem{}, err
	}

	if err := service.ValidateStatusTransition(currentStatus, nextStatus); err != nil {
		return model.StatusHistoryItem{}, err
	}

	if _, err := tx.ExecContext(ctx, updateOrderStatusQuery, orderID, nextStatus); err != nil {
		return model.StatusHistoryItem{}, err
	}

	historyRow := tx.QueryRowContext(ctx, insertStatusHistoryQuery, orderID, nextStatus, source, comment, metadata)
	historyItem, err := scanStatusHistoryItem(historyRow)
	if err != nil {
		return model.StatusHistoryItem{}, err
	}

	if err := tx.Commit(); err != nil {
		return model.StatusHistoryItem{}, err
	}

	return historyItem, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanStatusHistoryItem(row rowScanner) (model.StatusHistoryItem, error) {
	var item model.StatusHistoryItem
	var comment sql.NullString
	var metadata []byte

	if err := row.Scan(
		&item.ID,
		&item.OrderID,
		&item.Status,
		&item.Source,
		&comment,
		&metadata,
		&item.CreatedAt,
	); err != nil {
		return model.StatusHistoryItem{}, err
	}

	if comment.Valid {
		commentText := comment.String
		item.Comment = &commentText
	}

	if len(metadata) > 0 {
		item.Metadata = append(item.Metadata[:0], metadata...)
	}

	return item, nil
}
