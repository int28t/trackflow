package client

import (
	"context"
	"sync"
	"time"

	"trackflow/services/carrier-sync-service/internal/model"
)

var mockOrderIDs = []string{
	"e1111111-1111-1111-1111-111111111111",
	"e2222222-2222-2222-2222-222222222222",
	"e3333333-3333-3333-3333-333333333333",
	"e4444444-4444-4444-4444-444444444444",
	"e5555555-5555-5555-5555-555555555555",
}

var mockStatuses = []string{"created", "assigned", "in_transit", "delivered"}

type MockClient struct {
	baseURL string
	token   string

	mu     sync.Mutex
	cursor int
}

func NewMockClient(baseURL, token string) *MockClient {
	return &MockClient{
		baseURL: baseURL,
		token:   token,
	}
}

func (c *MockClient) FetchStatusUpdates(ctx context.Context, limit int) ([]model.StatusUpdate, error) {
	if limit <= 0 {
		limit = 1
	}

	if limit > 20 {
		limit = 20
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC()
	updates := make([]model.StatusUpdate, 0, limit)

	for i := 0; i < limit; i++ {
		orderID := mockOrderIDs[c.cursor%len(mockOrderIDs)]
		status := mockStatuses[c.cursor%len(mockStatuses)]

		updates = append(updates, model.StatusUpdate{
			OrderID:        orderID,
			ExternalStatus: status,
			UpdatedAt:      now.Add(-time.Duration(c.cursor%120) * time.Second),
		})

		c.cursor++
	}

	return updates, nil
}
