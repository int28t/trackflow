package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

const orderKeyPrefix = "order:"

type keyDeleter interface {
	Del(ctx context.Context, keys ...string) *goredis.IntCmd
}

type OrderCacheInvalidator struct {
	client keyDeleter
}

func NewOrderCacheInvalidator(client keyDeleter) *OrderCacheInvalidator {
	return &OrderCacheInvalidator{client: client}
}

func (i *OrderCacheInvalidator) DeleteOrder(ctx context.Context, orderID string) error {
	if i == nil || i.client == nil || orderID == "" {
		return nil
	}

	return i.client.Del(ctx, orderKeyPrefix+orderID).Err()
}
