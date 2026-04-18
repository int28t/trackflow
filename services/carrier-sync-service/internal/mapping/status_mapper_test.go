package mapping

import (
	"errors"
	"testing"
)

func TestMapExternalStatus(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "created passthrough", input: "created", want: InternalStatusCreated},
		{name: "new alias", input: "new", want: InternalStatusCreated},
		{name: "with spaces and case", input: "  AcCePtEd ", want: InternalStatusCreated},
		{name: "courier assigned alias", input: "courier-assigned", want: InternalStatusAssigned},
		{name: "picked up alias", input: "picked up", want: InternalStatusAssigned},
		{name: "out for delivery alias", input: "out-for-delivery", want: InternalStatusInTransit},
		{name: "on route alias", input: "on route", want: InternalStatusInTransit},
		{name: "completed alias", input: "completed", want: InternalStatusDelivered},
		{name: "received alias", input: "received", want: InternalStatusDelivered},
		{name: "canceled alias", input: "canceled", want: InternalStatusCancelled},
		{name: "delivery failed alias", input: "delivery-failed", want: InternalStatusCancelled},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := MapExternalStatus(tc.input)
			if err != nil {
				t.Fatalf("MapExternalStatus returned error: %v", err)
			}

			if got != tc.want {
				t.Fatalf("mapped status mismatch: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMapExternalStatusUnknown(t *testing.T) {
	t.Parallel()

	_, err := MapExternalStatus("mystery_status")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !errors.Is(err, ErrUnknownCarrierStatus) {
		t.Fatalf("expected ErrUnknownCarrierStatus, got %v", err)
	}
}
