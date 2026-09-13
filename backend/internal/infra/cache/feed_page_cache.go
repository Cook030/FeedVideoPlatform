package infracache

import (
	"context"
	"encoding/json"
	"time"

	inframetrics "GCFeed/internal/infra/metrics"
	contract "GCFeed/internal/shared/contract"

	"github.com/redis/go-redis/v9"
)

// feedPageCache 负责 Feed 轻量页缓存，只做序列化与读写，不感知业务场景。
type feedPageCache struct {
	client redisWatchCmdable
}

// GetPage 读取缓存中的轻量 Feed 页。
func (c *feedPageCache) GetPage(ctx context.Context, key string) (*contract.FeedPage, bool, error) {
	content, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		inframetrics.ObserveCacheRead("page", 1, 0, nil)
		return nil, false, nil
	}
	if err != nil {
		inframetrics.ObserveCacheRead("page", 1, 0, err)
		return nil, false, err
	}

	var page contract.FeedPage
	if err := json.Unmarshal(content, &page); err != nil {
		inframetrics.ObserveCacheRead("page", 1, 0, err)
		return nil, false, err
	}
	inframetrics.ObserveCacheRead("page", 1, 1, nil)
	return &page, true, nil
}

// SetPage 写入轻量 Feed 页，并设置过期时间。
func (c *feedPageCache) SetPage(ctx context.Context, key string, page *contract.FeedPage, ttl time.Duration) error {
	content, err := json.Marshal(page)
	if err != nil {
		inframetrics.ObserveCacheWrite("page", 1, err)
		return err
	}
	err = c.client.Set(ctx, key, content, ttl).Err()
	inframetrics.ObserveCacheWrite("page", 1, err)
	return err
}
