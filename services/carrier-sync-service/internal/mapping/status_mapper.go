package mapping

import (
	"errors"
	"fmt"
	"strings"
)

const (
	InternalStatusCreated   = "created"
	InternalStatusAssigned  = "assigned"
	InternalStatusInTransit = "in_transit"
	InternalStatusDelivered = "delivered"
	InternalStatusCancelled = "cancelled"
)

var ErrUnknownCarrierStatus = errors.New("unknown carrier status")

var externalToInternalStatusMap = map[string]string{
	"created":            InternalStatusCreated,
	"new":                InternalStatusCreated,
	"registered":         InternalStatusCreated,
	"accepted":           InternalStatusCreated,
	"confirmed":          InternalStatusCreated,
	"processing":         InternalStatusCreated,
	"order_received":     InternalStatusCreated,
	"shipment_created":   InternalStatusCreated,
	"info_received":      InternalStatusCreated,
	"pending":            InternalStatusCreated,
	"assigned":           InternalStatusAssigned,
	"courier_assigned":   InternalStatusAssigned,
	"picked_up":          InternalStatusAssigned,
	"pickup_completed":   InternalStatusAssigned,
	"collected":          InternalStatusAssigned,
	"ready_for_dispatch": InternalStatusAssigned,
	"in_transit":         InternalStatusInTransit,
	"out_for_delivery":   InternalStatusInTransit,
	"on_the_way":         InternalStatusInTransit,
	"on_route":           InternalStatusInTransit,
	"departed_hub":       InternalStatusInTransit,
	"arrived_hub":        InternalStatusInTransit,
	"at_sorting_center":  InternalStatusInTransit,
	"sorting":            InternalStatusInTransit,
	"delivered":          InternalStatusDelivered,
	"completed":          InternalStatusDelivered,
	"received":           InternalStatusDelivered,
	"done":               InternalStatusDelivered,
	"cancelled":          InternalStatusCancelled,
	"canceled":           InternalStatusCancelled,
	"delivery_failed":    InternalStatusCancelled,
	"undeliverable":      InternalStatusCancelled,
	"rejected":           InternalStatusCancelled,
	"returned":           InternalStatusCancelled,
	"lost":               InternalStatusCancelled,
}

func MapExternalStatus(externalStatus string) (string, error) {
	normalized := normalizeStatus(externalStatus)
	if normalized == "" {
		return "", fmt.Errorf("%w: empty status", ErrUnknownCarrierStatus)
	}

	internalStatus, ok := externalToInternalStatusMap[normalized]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrUnknownCarrierStatus, externalStatus)
	}

	return internalStatus, nil
}

func normalizeStatus(status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	replacer := strings.NewReplacer("-", "_", " ", "_")
	normalized = replacer.Replace(normalized)

	for strings.Contains(normalized, "__") {
		normalized = strings.ReplaceAll(normalized, "__", "_")
	}

	return strings.Trim(normalized, "_")
}
