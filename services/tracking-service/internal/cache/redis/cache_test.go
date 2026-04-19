package redis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"trackflow/services/tracking-service/internal/model"
)

type clientStub struct {
	getKey string
	getCmd *goredis.StringCmd

	setKey   string
	setValue interface{}
	setTTL   time.Duration
	setCmd   *goredis.StatusCmd
	setCalls int

	delKeys  []string
	delCmd   *goredis.IntCmd
	delCalls int
}

func (c *clientStub) Get(_ context.Context, key string) *goredis.StringCmd {
	c.getKey = key
	if c.getCmd != nil {
		return c.getCmd
	}

	return goredis.NewStringResult("", goredis.Nil)
}

func (c *clientStub) Set(_ context.Context, key string, value interface{}, expiration time.Duration) *goredis.StatusCmd {
	c.setCalls++
	c.setKey = key
	c.setValue = value
	c.setTTL = expiration

	if c.setCmd != nil {
		return c.setCmd
	}

	return goredis.NewStatusResult("OK", nil)
}

func (c *clientStub) Del(_ context.Context, keys ...string) *goredis.IntCmd {
	c.delCalls++
	c.delKeys = append([]string{}, keys...)
	if c.delCmd != nil {
		return c.delCmd
	}

	return goredis.NewIntResult(1, nil)
}

func TestGetTimelineReturnsMissOnRedisNil(t *testing.T) {
	t.Parallel()

	cache := NewTimelineCache(&clientStub{getCmd: goredis.NewStringResult("", goredis.Nil)})

	items, found, err := cache.GetTimeline(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("GetTimeline returned error: %v", err)
	}

	if found {
		t.Fatal("expected cache miss")
	}

	if len(items) != 0 {
		t.Fatalf("expected empty items on miss, got %d", len(items))
	}
}

func TestGetTimelineReturnsDecodedItems(t *testing.T) {
	t.Parallel()

	expected := []model.StatusHistoryItem{{
		ID:      "hist-1",
		OrderID: "order-1",
		Status:  "created",
		Source:  "system",
	}}

	payload, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	cache := NewTimelineCache(&clientStub{getCmd: goredis.NewStringResult(string(payload), nil)})

	items, found, err := cache.GetTimeline(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("GetTimeline returned error: %v", err)
	}

	if !found {
		t.Fatal("expected cache hit")
	}

	if len(items) != 1 || items[0].ID != expected[0].ID {
		t.Fatalf("unexpected decoded items: got %+v", items)
	}
}

func TestGetTimelineReturnsDecodeError(t *testing.T) {
	t.Parallel()

	cache := NewTimelineCache(&clientStub{getCmd: goredis.NewStringResult("{", nil)})
	_, found, err := cache.GetTimeline(context.Background(), "order-2")
	if err == nil {
		t.Fatal("expected decode error")
	}

	if found {
		t.Fatal("did not expect cache hit on decode error")
	}
}

func TestSetTimelineStoresPayloadWithTTL(t *testing.T) {
	t.Parallel()

	client := &clientStub{}
	cache := NewTimelineCache(client)
	items := []model.StatusHistoryItem{{
		ID:      "hist-2",
		OrderID: "order-3",
		Status:  "assigned",
		Source:  "dispatcher",
	}}

	ttl := 30 * time.Second
	if err := cache.SetTimeline(context.Background(), "order-3", items, ttl); err != nil {
		t.Fatalf("SetTimeline returned error: %v", err)
	}

	if client.setCalls != 1 {
		t.Fatalf("expected one set call, got %d", client.setCalls)
	}

	if client.setKey != "timeline:order-3" {
		t.Fatalf("unexpected cache key: got %q", client.setKey)
	}

	if client.setTTL != ttl {
		t.Fatalf("unexpected ttl: got %s, want %s", client.setTTL, ttl)
	}

	payload, ok := client.setValue.([]byte)
	if !ok {
		t.Fatalf("expected []byte payload, got %T", client.setValue)
	}

	var decoded []model.StatusHistoryItem
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	if len(decoded) != 1 || decoded[0].ID != items[0].ID {
		t.Fatalf("unexpected decoded payload: got %+v", decoded)
	}
}

func TestSetTimelineReturnsRedisError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("redis set failed")
	cache := NewTimelineCache(&clientStub{setCmd: goredis.NewStatusResult("", expectedErr)})

	err := cache.SetTimeline(context.Background(), "order-4", []model.StatusHistoryItem{}, time.Second)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected redis error %v, got %v", expectedErr, err)
	}
}

func TestDeleteTimelineCallsRedisDel(t *testing.T) {
	t.Parallel()

	client := &clientStub{}
	cache := NewTimelineCache(client)
	if err := cache.DeleteTimeline(context.Background(), "order-5"); err != nil {
		t.Fatalf("DeleteTimeline returned error: %v", err)
	}

	if client.delCalls != 1 {
		t.Fatalf("expected one del call, got %d", client.delCalls)
	}

	if len(client.delKeys) != 1 || client.delKeys[0] != "timeline:order-5" {
		t.Fatalf("unexpected del keys: %+v", client.delKeys)
	}
}

func TestDeleteOrderInvalidatorUsesOrderPrefix(t *testing.T) {
	t.Parallel()

	client := &clientStub{}
	invalidator := NewOrderCacheInvalidator(client)
	if err := invalidator.DeleteOrder(context.Background(), "order-6"); err != nil {
		t.Fatalf("DeleteOrder returned error: %v", err)
	}

	if client.delCalls != 1 {
		t.Fatalf("expected one del call, got %d", client.delCalls)
	}

	if len(client.delKeys) != 1 || client.delKeys[0] != "order:order-6" {
		t.Fatalf("unexpected del keys: %+v", client.delKeys)
	}
}
