package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"trackflow/services/order-service/internal/model"
)

const (
	defaultListLimit    = 20
	maxListLimit        = 100
	defaultServiceLevel = "standard"
)

var (
	ErrOrderNotFound        = errors.New("order not found")
	ErrDuplicateIdempotency = errors.New("duplicate idempotency key")
	ErrInvalidInput         = errors.New("invalid input")
	uuidPattern             = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
)

type Repository interface {
	Ping(ctx context.Context) error
	ListOrders(ctx context.Context, limit int) ([]model.Order, error)
	CreateOrder(ctx context.Context, input model.CreateOrderInput, idempotencyKey string) (model.Order, error)
	GetOrderByIdempotencyKey(ctx context.Context, idempotencyKey string) (model.Order, error)
}

type OrderService struct {
	repo Repository
}

func New(repo Repository) *OrderService {
	return &OrderService{repo: repo}
}

func (s *OrderService) Health(ctx context.Context) error {
	if s == nil || s.repo == nil {
		return errors.New("repository is not configured")
	}

	return s.repo.Ping(ctx)
}

func (s *OrderService) ListOrders(ctx context.Context, limit int) ([]model.Order, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("repository is not configured")
	}

	return s.repo.ListOrders(ctx, normalizeLimit(limit))
}

func (s *OrderService) CreateOrder(ctx context.Context, input model.CreateOrderInput, idempotencyKey string) (model.Order, bool, error) {
	if s == nil || s.repo == nil {
		return model.Order{}, false, errors.New("repository is not configured")
	}

	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return model.Order{}, false, validationError("idempotency key is required")
	}

	normalizedInput, err := normalizeCreateInput(input)
	if err != nil {
		return model.Order{}, false, err
	}

	existing, err := s.repo.GetOrderByIdempotencyKey(ctx, key)
	if err == nil {
		return existing, false, nil
	}

	if !errors.Is(err, ErrOrderNotFound) {
		return model.Order{}, false, err
	}

	createdOrder, err := s.repo.CreateOrder(ctx, normalizedInput, key)
	if err == nil {
		return createdOrder, true, nil
	}

	if !errors.Is(err, ErrDuplicateIdempotency) {
		return model.Order{}, false, err
	}

	existing, lookupErr := s.repo.GetOrderByIdempotencyKey(ctx, key)
	if lookupErr != nil {
		return model.Order{}, false, lookupErr
	}

	return existing, false, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}

	if limit > maxListLimit {
		return maxListLimit
	}

	return limit
}

func normalizeCreateInput(input model.CreateOrderInput) (model.CreateOrderInput, error) {
	normalized := input

	normalized.CustomerID = strings.TrimSpace(normalized.CustomerID)
	if normalized.CustomerID == "" {
		return model.CreateOrderInput{}, validationError("customer_id is required")
	}

	if !uuidPattern.MatchString(normalized.CustomerID) {
		return model.CreateOrderInput{}, validationError("customer_id must be a valid UUID")
	}

	normalized.PickupAddress = normalizeAddressInput(normalized.PickupAddress)
	normalized.DropoffAddress = normalizeAddressInput(normalized.DropoffAddress)

	if err := validateAddress("pickup_address", normalized.PickupAddress); err != nil {
		return model.CreateOrderInput{}, err
	}

	if err := validateAddress("dropoff_address", normalized.DropoffAddress); err != nil {
		return model.CreateOrderInput{}, err
	}

	if normalized.WeightKG <= 0 {
		return model.CreateOrderInput{}, validationError("weight_kg must be greater than zero")
	}

	if normalized.DistanceKM < 0 {
		return model.CreateOrderInput{}, validationError("distance_km must be greater than or equal to zero")
	}

	normalized.ServiceLevel = strings.ToLower(strings.TrimSpace(normalized.ServiceLevel))
	if normalized.ServiceLevel == "" {
		normalized.ServiceLevel = defaultServiceLevel
	}

	if normalized.ServiceLevel != "standard" && normalized.ServiceLevel != "express" {
		return model.CreateOrderInput{}, validationError("service_level must be one of: standard, express")
	}

	return normalized, nil
}

func normalizeAddressInput(input model.AddressInput) model.AddressInput {
	input.City = strings.TrimSpace(input.City)
	input.Street = strings.TrimSpace(input.Street)
	input.House = strings.TrimSpace(input.House)
	input.Apartment = strings.TrimSpace(input.Apartment)
	return input
}

func validateAddress(fieldName string, address model.AddressInput) error {
	if address.City == "" {
		return validationError(fmt.Sprintf("%s.city is required", fieldName))
	}

	if address.Street == "" {
		return validationError(fmt.Sprintf("%s.street is required", fieldName))
	}

	if address.House == "" {
		return validationError(fmt.Sprintf("%s.house is required", fieldName))
	}

	if address.Lat < -90 || address.Lat > 90 {
		return validationError(fmt.Sprintf("%s.lat must be in range [-90, 90]", fieldName))
	}

	if address.Lng < -180 || address.Lng > 180 {
		return validationError(fmt.Sprintf("%s.lng must be in range [-180, 180]", fieldName))
	}

	return nil
}

func validationError(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidInput, message)
}
