package infracache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisWatchCmdable 是 Feed 缓存需要的最小 Redis 能力集合。
type redisWatchCmdable interface {
	redis.Cmdable
	Pipeline() redis.Pipeliner
	Watch(ctx context.Context, fn func(*redis.Tx) error, keys ...string) error
}

type redisActionStatReader interface {
	HGetAll(ctx context.Context, key string) *redis.MapStringStringCmd
	Get(ctx context.Context, key string) *redis.StringCmd
}

type redisActionStatWriter interface {
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
}

type redisStatCacheClient interface {
	redisActionStatReader
	redisActionStatWriter
	MGet(ctx context.Context, keys ...string) *redis.SliceCmd
}
