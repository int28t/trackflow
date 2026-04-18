package service

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidStatus              = errors.New("invalid status")
	ErrInvalidStatusSource        = errors.New("invalid status source")
	ErrStatusTransitionNotAllowed = errors.New("status transition is not allowed")
)

const (
	StatusCreated   = "created"
	StatusAssigned  = "assigned"
	StatusInTransit = "in_transit"
	StatusDelivered = "delivered"
	StatusCancelled = "cancelled"
	SourceSystem    = "system"
	SourceCourier   = "courier"
	SourceManager   = "manager"
	SourceCarrier   = "carrier_sync"
)

var allowedTransitions = map[string]map[string]struct{}{
	StatusCreated: {
		StatusAssigned:  {},
		StatusCancelled: {},
	},
	StatusAssigned: {
		StatusInTransit: {},
		StatusCancelled: {},
	},
	StatusInTransit: {
		StatusDelivered: {},
	},
	StatusDelivered: {},
	StatusCancelled: {},
}

var allowedSources = map[string]struct{}{
	SourceSystem:  {},
	SourceCourier: {},
	SourceManager: {},
	SourceCarrier: {},
}

func NormalizeStatus(status string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if _, ok := allowedTransitions[normalized]; !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidStatus, status)
	}

	return normalized, nil
}

func ValidateStatusTransition(currentStatus, nextStatus string) error {
	current, err := NormalizeStatus(currentStatus)
	if err != nil {
		return err
	}

	next, err := NormalizeStatus(nextStatus)
	if err != nil {
		return err
	}

	allowedSet := allowedTransitions[current]
	if _, ok := allowedSet[next]; ok {
		return nil
	}

	return fmt.Errorf("%w: %s -> %s", ErrStatusTransitionNotAllowed, current, next)
}

func NormalizeStatusSource(source string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(source))
	if _, ok := allowedSources[normalized]; !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidStatusSource, source)
	}

	return normalized, nil
}
