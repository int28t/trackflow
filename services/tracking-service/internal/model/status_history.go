package model

import (
	"encoding/json"
	"time"
)

type StatusHistoryItem struct {
	ID        string          `json:"id"`
	OrderID   string          `json:"order_id"`
	Status    string          `json:"status"`
	Source    string          `json:"source"`
	Comment   *string         `json:"comment,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
}
