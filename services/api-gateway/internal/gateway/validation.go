package gateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	maxGatewayRequestBodySize = 128 * 1024
	maxOrdersListLimit        = 100
	maxTimelineListLimit      = 200
	maxIdempotencyKeyLength   = 128
)

var (
	uuidPattern   = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")
	serviceLevels = map[string]struct{}{
		"standard": {},
		"express":  {},
	}
	statuses = map[string]struct{}{
		"created":    {},
		"assigned":   {},
		"in_transit": {},
		"delivered":  {},
		"cancelled":  {},
	}
	statusSources = map[string]struct{}{
		"system":       {},
		"courier":      {},
		"manager":      {},
		"carrier_sync": {},
	}
)

type createOrderAddressDTO struct {
	City      string  `json:"city"`
	Street    string  `json:"street"`
	House     string  `json:"house"`
	Apartment string  `json:"apartment"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
}

type createOrderDTO struct {
	CustomerID     string                `json:"customer_id"`
	PickupAddress  createOrderAddressDTO `json:"pickup_address"`
	DropoffAddress createOrderAddressDTO `json:"dropoff_address"`
	WeightKG       float64               `json:"weight_kg"`
	DistanceKM     float64               `json:"distance_km"`
	ServiceLevel   string                `json:"service_level"`
}

type assignOrderDTO struct {
	CourierID  string `json:"courier_id"`
	AssignedBy string `json:"assigned_by"`
	Comment    string `json:"comment"`
}

type updateStatusDTO struct {
	Status   string          `json:"status"`
	Source   string          `json:"source"`
	Comment  string          `json:"comment"`
	Metadata json.RawMessage `json:"metadata"`
}

func validationError(message string, err error) *HTTPError {
	return NewHTTPError(http.StatusBadRequest, "validation_error", message, err)
}

func validateOrderID(orderID string) error {
	id := strings.TrimSpace(orderID)
	if id == "" {
		return validationError("order_id is required", nil)
	}

	if !uuidPattern.MatchString(id) {
		return validationError("order_id must be a valid UUID", nil)
	}

	return nil
}

func validateLimit(raw string, max int) error {
	if raw == "" {
		return nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil {
		return validationError("limit must be an integer", err)
	}

	if limit <= 0 {
		return validationError("limit must be greater than zero", nil)
	}

	if limit > max {
		return validationError("limit is too large", nil)
	}

	return nil
}

func validateCreateOrderRequest(r *http.Request) error {
	if r == nil {
		return validationError("request is nil", nil)
	}

	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		return validationError("Idempotency-Key header is required", nil)
	}

	if len(idempotencyKey) > maxIdempotencyKeyLength {
		return validationError("Idempotency-Key is too long", nil)
	}

	var payload createOrderDTO
	if err := decodeAndRestoreJSONBody(r, maxGatewayRequestBodySize, &payload); err != nil {
		return err
	}

	if !uuidPattern.MatchString(strings.TrimSpace(payload.CustomerID)) {
		return validationError("customer_id must be a valid UUID", nil)
	}

	if err := validateCreateAddress("pickup_address", payload.PickupAddress); err != nil {
		return err
	}

	if err := validateCreateAddress("dropoff_address", payload.DropoffAddress); err != nil {
		return err
	}

	if payload.WeightKG <= 0 {
		return validationError("weight_kg must be greater than zero", nil)
	}

	if payload.DistanceKM < 0 {
		return validationError("distance_km must be greater than or equal to zero", nil)
	}

	serviceLevel := strings.ToLower(strings.TrimSpace(payload.ServiceLevel))
	if serviceLevel != "" {
		if _, ok := serviceLevels[serviceLevel]; !ok {
			return validationError("service_level must be one of: standard, express", nil)
		}
	}

	return nil
}

func validateCreateAddress(field string, value createOrderAddressDTO) error {
	if strings.TrimSpace(value.City) == "" {
		return validationError(field+".city is required", nil)
	}

	if strings.TrimSpace(value.Street) == "" {
		return validationError(field+".street is required", nil)
	}

	if strings.TrimSpace(value.House) == "" {
		return validationError(field+".house is required", nil)
	}

	if value.Lat < -90 || value.Lat > 90 {
		return validationError(field+".lat must be in range [-90, 90]", nil)
	}

	if value.Lng < -180 || value.Lng > 180 {
		return validationError(field+".lng must be in range [-180, 180]", nil)
	}

	return nil
}

func validateAssignOrderRequest(r *http.Request) error {
	if r == nil {
		return validationError("request is nil", nil)
	}

	var payload assignOrderDTO
	if err := decodeAndRestoreJSONBody(r, maxGatewayRequestBodySize, &payload); err != nil {
		return err
	}

	if !uuidPattern.MatchString(strings.TrimSpace(payload.CourierID)) {
		return validationError("courier_id must be a valid UUID", nil)
	}

	if payload.AssignedBy != "" && strings.TrimSpace(payload.AssignedBy) == "" {
		return validationError("assigned_by must not be blank", nil)
	}

	if payload.Comment != "" && strings.TrimSpace(payload.Comment) == "" {
		return validationError("comment must not be blank", nil)
	}

	return nil
}

func validateUpdateStatusRequest(r *http.Request) error {
	if r == nil {
		return validationError("request is nil", nil)
	}

	var payload updateStatusDTO
	if err := decodeAndRestoreJSONBody(r, maxGatewayRequestBodySize, &payload); err != nil {
		return err
	}

	status := strings.ToLower(strings.TrimSpace(payload.Status))
	if status == "" {
		return validationError("status is required", nil)
	}

	if _, ok := statuses[status]; !ok {
		return validationError("status is invalid", nil)
	}

	if source := strings.ToLower(strings.TrimSpace(payload.Source)); source != "" {
		if _, ok := statusSources[source]; !ok {
			return validationError("source is invalid", nil)
		}
	}

	if payload.Comment != "" && strings.TrimSpace(payload.Comment) == "" {
		return validationError("comment must not be blank", nil)
	}

	if len(payload.Metadata) > 0 {
		var parsed any
		if err := json.Unmarshal(payload.Metadata, &parsed); err != nil {
			return validationError("metadata must be valid JSON", err)
		}

		if _, ok := parsed.(map[string]any); !ok {
			return validationError("metadata must be a JSON object", nil)
		}
	}

	return nil
}

func decodeAndRestoreJSONBody(r *http.Request, maxSize int64, dst any) error {
	if r == nil {
		return validationError("request is nil", nil)
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxSize+1))
	if err != nil {
		return validationError("failed to read request body", err)
	}

	if r.Body != nil {
		_ = r.Body.Close()
	}

	if int64(len(body)) > maxSize {
		return validationError("request body is too large", nil)
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return validationError("request body is required", nil)
	}

	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return validationError("invalid json payload", err)
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return validationError("invalid json payload", err)
	}

	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	return nil
}
