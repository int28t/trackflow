package sender

import (
	"context"

	"trackflow/services/notification-service/internal/model"
)

type Sender interface {
	Send(ctx context.Context, event model.Event) error
}
