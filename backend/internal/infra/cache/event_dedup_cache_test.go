package infracache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type eventDedupFakeRedis struct {
	values map[string]string
}

func newEventDedupFakeRedis() *eventDedupFakeRedis {
	return &eventDedupFakeRedis{values: map[string]string{}}
}

func (r *eventDedupFakeRedis) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	if _, exists := r.values[key]; exists {
		return redis.NewBoolResult(false, nil)
	}
	r.values[key] = "1"
	return redis.NewBoolResult(true, nil)
}

func (r *eventDedupFakeRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	removed := int64(0)
	for _, key := range keys {
		if _, ok := r.values[key]; ok {
			delete(r.values, key)
			removed++
		}
	}
	return redis.NewIntResult(removed, nil)
}

func TestEventDedupCacheClaimOnlyOnce(t *testing.T) {
	ctx := context.Background()
	client := newEventDedupFakeRedis()
	cache := NewEventDedupCache(client)

	claimed, err := cache.Claim(ctx, "rec:consumed:v1:1")
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim must succeed")
	}

	claimed, err = cache.Claim(ctx, "rec:consumed:v1:1")
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if claimed {
		t.Fatal("second claim must be rejected as duplicate")
	}

	// 不同事件互不影响。
	if claimed, err = cache.Claim(ctx, "rec:consumed:v1:2"); err != nil || !claimed {
		t.Fatalf("claim for another key = %v, err = %v", claimed, err)
	}
}

// TestEventDedupCacheReleaseAllowsReprocess 处理失败后必须能重新占用，
// 否则消息重投会被误判为已处理，事件被静默丢弃。
func TestEventDedupCacheReleaseAllowsReprocess(t *testing.T) {
	ctx := context.Background()
	client := newEventDedupFakeRedis()
	cache := NewEventDedupCache(client)

	if claimed, _ := cache.Claim(ctx, "rec:consumed:v1:3"); !claimed {
		t.Fatal("claim must succeed")
	}
	if err := cache.Release(ctx, "rec:consumed:v1:3"); err != nil {
		t.Fatalf("release: %v", err)
	}
	claimed, err := cache.Claim(ctx, "rec:consumed:v1:3")
	if err != nil {
		t.Fatalf("claim after release: %v", err)
	}
	if !claimed {
		t.Fatal("claim after release must succeed")
	}
}

// TestEventDedupCacheDegradesOnEmptyKey 缺少幂等键时视为首次处理，
// 宁可重复执行也不静默丢事件。
func TestEventDedupCacheDegradesOnEmptyKey(t *testing.T) {
	ctx := context.Background()
	cache := NewEventDedupCache(newEventDedupFakeRedis())

	for i := 0; i < 2; i++ {
		claimed, err := cache.Claim(ctx, "")
		if err != nil {
			t.Fatalf("claim %d: %v", i+1, err)
		}
		if !claimed {
			t.Fatalf("empty key claim %d must be treated as first seen", i+1)
		}
	}
	if err := cache.Release(ctx, ""); err != nil {
		t.Fatalf("release empty key: %v", err)
	}
}

func TestEventDedupCacheNilSafety(t *testing.T) {
	ctx := context.Background()

	var nilCache *EventDedupCache
	if claimed, err := nilCache.Claim(ctx, "rec:consumed:v1:4"); !claimed || err != nil {
		t.Fatalf("nil cache claim = %v, err = %v", claimed, err)
	}
	if err := nilCache.Release(ctx, "rec:consumed:v1:4"); err != nil {
		t.Fatalf("nil cache release: %v", err)
	}

	noClient := NewEventDedupCache(nil)
	if claimed, err := noClient.Claim(ctx, "rec:consumed:v1:5"); !claimed || err != nil {
		t.Fatalf("nil client claim = %v, err = %v", claimed, err)
	}
	if err := noClient.Release(ctx, "rec:consumed:v1:5"); err != nil {
		t.Fatalf("nil client release: %v", err)
	}
}
