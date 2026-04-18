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
