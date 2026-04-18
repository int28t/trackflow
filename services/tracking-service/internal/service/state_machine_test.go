package service

import (
	"errors"
	"testing"
)

func TestValidateStatusTransitionAllowed(t *testing.T) {
	tests := []struct {
		name    string
		current string
		next    string
	}{
		{name: "created to assigned", current: StatusCreated, next: StatusAssigned},
		{name: "created to cancelled", current: StatusCreated, next: StatusCancelled},
		{name: "assigned to in transit", current: StatusAssigned, next: StatusInTransit},
		{name: "assigned to cancelled", current: StatusAssigned, next: StatusCancelled},
		{name: "in transit to delivered", current: StatusInTransit, next: StatusDelivered},
		{name: "trim and lower", current: "  CREATED ", next: " Assigned "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateStatusTransition(tt.current, tt.next); err != nil {
				t.Fatalf("expected transition to be allowed, got error: %v", err)
			}
		})
	}
}

func TestValidateStatusTransitionDisallowed(t *testing.T) {
	tests := []struct {
		name    string
		current string
		next    string
	}{
		{name: "created to delivered", current: StatusCreated, next: StatusDelivered},
		{name: "assigned to created", current: StatusAssigned, next: StatusCreated},
		{name: "in transit to cancelled", current: StatusInTransit, next: StatusCancelled},
		{name: "delivered to in transit", current: StatusDelivered, next: StatusInTransit},
		{name: "cancelled to assigned", current: StatusCancelled, next: StatusAssigned},
		{name: "same status", current: StatusAssigned, next: StatusAssigned},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStatusTransition(tt.current, tt.next)
			if err == nil {
				t.Fatalf("expected transition to be rejected")
			}

			if !errors.Is(err, ErrStatusTransitionNotAllowed) {
				t.Fatalf("expected ErrStatusTransitionNotAllowed, got: %v", err)
			}
		})
	}
}

func TestValidateStatusTransitionInvalidStatus(t *testing.T) {
	err := ValidateStatusTransition("unknown", StatusCreated)
	if err == nil {
		t.Fatalf("expected invalid status error")
	}

	if !errors.Is(err, ErrInvalidStatus) {
		t.Fatalf("expected ErrInvalidStatus, got: %v", err)
	}
}
