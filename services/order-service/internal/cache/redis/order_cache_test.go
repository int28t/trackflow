package redis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"trackflow/services/order-service/internal/model"
)

type clientStub struct {
	getKey string
	getCmd *goredis.StringCmd

	setKey   string
	setValue interface{}
	setTTL   time.Duration
	setCmd   *goredis.StatusCmd
	setCalls int
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

func TestGetOrderByIDReturnsMissOnRedisNil(t *testing.T) {
	t.Parallel()

	client := &clientStub{getCmd: goredis.NewStringResult("", goredis.Nil)}
	cache := NewOrderCache(client)

	order, found, err := cache.GetOrderByID(context.Background(), "order-1")
	if err != nil {
		t.Fatalf("GetOrderByID returned error: %v", err)
	}

	if found {
		t.Fatal("expected cache miss")
	}

	if order.ID != "" {
		t.Fatalf("expected empty order on miss, got %+v", order)
	}
}

func TestGetOrderByIDReturnsDecodedOrder(t *testing.T) {
	t.Parallel()

	expected := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "created",
	}

	payload, err := json.Marshal(expected)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	client := &clientStub{getCmd: goredis.NewStringResult(string(payload), nil)}
	cache := NewOrderCache(client)

	order, found, err := cache.GetOrderByID(context.Background(), expected.ID)
	if err != nil {
		t.Fatalf("GetOrderByID returned error: %v", err)
	}

	if !found {
		t.Fatal("expected cache hit")
	}

	if order != expected {
		t.Fatalf("unexpected decoded order: got %+v, want %+v", order, expected)
	}
}

func TestGetOrderByIDReturnsDecodeError(t *testing.T) {
	t.Parallel()

	client := &clientStub{getCmd: goredis.NewStringResult("{", nil)}
	cache := NewOrderCache(client)

	_, found, err := cache.GetOrderByID(context.Background(), "order-2")
	if err == nil {
		t.Fatal("expected decode error")
	}

	if found {
		t.Fatal("did not expect cache hit on decode error")
	}
}

func TestSetOrderStoresPayloadWithTTL(t *testing.T) {
	t.Parallel()

	client := &clientStub{}
	cache := NewOrderCache(client)
	order := model.Order{
		ID:         "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CustomerID: "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
		Status:     "assigned",
	}

	ttl := 2 * time.Minute
	err := cache.SetOrder(context.Background(), order, ttl)
	if err != nil {
		t.Fatalf("SetOrder returned error: %v", err)
	}

	if client.setCalls != 1 {
		t.Fatalf("expected one set call, got %d", client.setCalls)
	}

	if client.setKey != "order:"+order.ID {
		t.Fatalf("unexpected cache key: got %q", client.setKey)
	}

	if client.setTTL != ttl {
		t.Fatalf("unexpected ttl: got %s, want %s", client.setTTL, ttl)
	}

	payload, ok := client.setValue.([]byte)
	if !ok {
		t.Fatalf("expected []byte payload, got %T", client.setValue)
	}

	var decoded model.Order
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal stored payload failed: %v", err)
	}

	if decoded != order {
		t.Fatalf("unexpected stored order: got %+v, want %+v", decoded, order)
	}
}

func TestSetOrderReturnsRedisError(t *testing.T) {
	t.Parallel()

	expectedErr := errors.New("redis write failed")
	client := &clientStub{setCmd: goredis.NewStatusResult("", expectedErr)}
	cache := NewOrderCache(client)

	err := cache.SetOrder(context.Background(), model.Order{ID: "order-3"}, time.Second)
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected redis error %v, got %v", expectedErr, err)
	}
}
