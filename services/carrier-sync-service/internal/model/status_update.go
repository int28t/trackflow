package model

import "time"

type StatusUpdate struct {
	OrderID        string    `json:"order_id"`
	ExternalStatus string    `json:"external_status"`
	UpdatedAt      time.Time `json:"updated_at"`
}
