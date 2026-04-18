package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"trackflow/services/tracking-service/internal/model"
)

const timelineKeyPrefix = "timeline:"

type Client interface {
	Get(ctx context.Context, key string) *goredis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *goredis.StatusCmd
	Del(ctx context.Context, keys ...string) *goredis.IntCmd
}

type TimelineCache struct {
	client Client
}

func NewTimelineCache(client Client) *TimelineCache {
	return &TimelineCache{client: client}
}

func (c *TimelineCache) GetTimeline(ctx context.Context, orderID string) ([]model.StatusHistoryItem, bool, error) {
	if c == nil || c.client == nil || orderID == "" {
		return nil, false, nil
	}

	payload, err := c.client.Get(ctx, cacheKey(orderID)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, false, nil
		}

		return nil, false, err
	}

	var items []model.StatusHistoryItem
	if err := json.Unmarshal(payload, &items); err != nil {
		return nil, false, fmt.Errorf("decode timeline cache value: %w", err)
	}

	if items == nil {
		items = []model.StatusHistoryItem{}
	}

	return items, true, nil
}

func (c *TimelineCache) SetTimeline(ctx context.Context, orderID string, items []model.StatusHistoryItem, ttl time.Duration) error {
	if c == nil || c.client == nil || orderID == "" {
		return nil
	}

	if items == nil {
		items = []model.StatusHistoryItem{}
	}

	payload, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("encode timeline cache value: %w", err)
	}

	return c.client.Set(ctx, cacheKey(orderID), payload, ttl).Err()
}

func (c *TimelineCache) DeleteTimeline(ctx context.Context, orderID string) error {
	if c == nil || c.client == nil || orderID == "" {
		return nil
	}

	return c.client.Del(ctx, cacheKey(orderID)).Err()
}

func cacheKey(orderID string) string {
	return timelineKeyPrefix + orderID
}
