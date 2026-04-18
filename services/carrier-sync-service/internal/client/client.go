package client

import (
	"context"

	"trackflow/services/carrier-sync-service/internal/model"
)

type CarrierClient interface {
	FetchStatusUpdates(ctx context.Context, limit int) ([]model.StatusUpdate, error)
}
