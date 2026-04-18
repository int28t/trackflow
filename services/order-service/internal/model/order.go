package model

import "time"

type Order struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type AddressInput struct {
	City      string  `json:"city"`
	Street    string  `json:"street"`
	House     string  `json:"house"`
	Apartment string  `json:"apartment"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
}

type CreateOrderInput struct {
	CustomerID     string       `json:"customer_id"`
	PickupAddress  AddressInput `json:"pickup_address"`
	DropoffAddress AddressInput `json:"dropoff_address"`
	WeightKG       float64      `json:"weight_kg"`
	DistanceKM     float64      `json:"distance_km"`
	ServiceLevel   string       `json:"service_level"`
}

type AssignOrderInput struct {
	CourierID  string `json:"courier_id"`
	AssignedBy string `json:"assigned_by"`
	Comment    string `json:"comment"`
}
