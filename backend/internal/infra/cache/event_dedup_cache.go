package infracache

import (
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// eventDedupTTL 覆盖消息被重投的时间窗口。重投受 max_retries 限制且立即执行，
// 24 小时远大于实际窗口，同时避免幂等键无限增长。
const eventDedupTTL = 24 * time.Hour

type redisEventDedupClient interface {
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// EventDedupCache 用 Redis 做事件级幂等，防止消息重投导致副作用重复执行。
//
// 采用"先占位、失败归还"的语义：
//   - Claim 成功表示调用方获得了该事件的处理权；
//   - 处理失败时必须 Release，否则消息重投后会被误判为已处理而丢掉事件；
//   - 处理成功则保留占位，后续重投直接跳过。
type EventDedupCache struct {
	client redisEventDedupClient
}

// NewEventDedupCache 创建事件幂等缓存。
func NewEventDedupCache(client redisEventDedupClient) *EventDedupCache {
	return &EventDedupCache{client: client}
}

// Claim 尝试占用幂等键。返回 false 表示该事件已经处理过，调用方应跳过。
func (c *EventDedupCache) Claim(ctx context.Context, key string) (bool, error) {
	if c == nil || c.client == nil || key == "" {
		// 无法判定时视为首次处理，宁可重复执行也不静默丢弃事件。
		return true, nil
	}
	claimed, err := c.client.SetNX(ctx, key, "1", eventDedupTTL).Result()
	if err != nil {
		inframetrics.ObserveCacheRead("event_dedup", 1, 0, err)
		return false, err
	}
	if claimed {
		inframetrics.ObserveCacheRead("event_dedup", 1, 1, nil)
	}
	return claimed, nil
}

// Release 在业务处理失败时归还幂等键，让消息重投后可以重新处理。
func (c *EventDedupCache) Release(ctx context.Context, key string) error {
	if c == nil || c.client == nil || key == "" {
		return nil
	}
	return c.client.Del(ctx, key).Err()
}
