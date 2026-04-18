package model

import "time"

type Event struct {
	OrderID   string    `json:"order_id"`
	Status    string    `json:"status"`
	Channel   string    `json:"channel"`
	Recipient string    `json:"recipient"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
