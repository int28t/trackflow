package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"trackflow/services/order-service/internal/model"
)

const orderKeyPrefix = "order:"

type Client interface {
	Get(ctx context.Context, key string) *goredis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *goredis.StatusCmd
}

type OrderCache struct {
	client Client
}

func NewOrderCache(client Client) *OrderCache {
	return &OrderCache{client: client}
}

func (c *OrderCache) GetOrderByID(ctx context.Context, orderID string) (model.Order, bool, error) {
	if c == nil || c.client == nil || orderID == "" {
		return model.Order{}, false, nil
	}

	payload, err := c.client.Get(ctx, cacheKey(orderID)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return model.Order{}, false, nil
		}

		return model.Order{}, false, err
	}

	var order model.Order
	if err := json.Unmarshal(payload, &order); err != nil {
		return model.Order{}, false, fmt.Errorf("decode order cache value: %w", err)
	}

	if order.ID == "" {
		return model.Order{}, false, nil
	}

	return order, true, nil
}

func (c *OrderCache) SetOrder(ctx context.Context, order model.Order, ttl time.Duration) error {
	if c == nil || c.client == nil || order.ID == "" {
		return nil
	}

	payload, err := json.Marshal(order)
	if err != nil {
		return fmt.Errorf("encode order cache value: %w", err)
	}

	return c.client.Set(ctx, cacheKey(order.ID), payload, ttl).Err()
}

func cacheKey(orderID string) string {
	return orderKeyPrefix + orderID
}
