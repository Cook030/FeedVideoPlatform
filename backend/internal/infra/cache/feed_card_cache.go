package infracache

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	domainfeed "GCFeed/internal/domain/feed"
	inframetrics "GCFeed/internal/infra/metrics"

	"github.com/redis/go-redis/v9"
)

// feedCardCache 负责视频卡片缓存与「作者 -> 视频」索引的维护。
type feedCardCache struct {
	client redisWatchCmdable
}

// GetCards 批量读取视频卡片缓存。
func (c *feedCardCache) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	cards := map[int64]*domainfeed.FeedCard{}
	if len(videoIDs) == 0 {
		return cards, nil
	}

	values, err := c.client.MGet(ctx, cacheKeys(videoIDs, feedCardKey)...).Result()
	if err != nil {
		inframetrics.ObserveCacheRead("card", len(videoIDs), 0, err)
		return nil, err
	}
	for index, value := range values {
		content, ok := cacheValueBytes(value)
		if !ok {
			continue
		}
		var card domainfeed.FeedCard
		if err := json.Unmarshal(content, &card); err != nil {
			continue
		}
		if card.VideoID <= 0 {
			card.VideoID = videoIDs[index]
		}
		cards[card.VideoID] = &card
	}
	inframetrics.ObserveCacheRead("card", len(videoIDs), len(cards), nil)
	return cards, nil
}

// SetCards 批量写入视频卡片缓存。
func (c *feedCardCache) SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error {
	pipe := c.client.Pipeline()
	queued := false

	for _, card := range cards {
		if card == nil || card.VideoID <= 0 {
			continue
		}
		content, err := json.Marshal(card)
		if err != nil {
			return err
		}
		pipe.Set(ctx, feedCardKey(card.VideoID), content, ttl)
		if card.AuthorID > 0 {
			// 维护作者到视频的索引，资料变更时可以按作者批量失效卡片。
			authorKey := feedAuthorCardKey(card.AuthorID)
			pipe.SAdd(ctx, authorKey, card.VideoID)
			pipe.Expire(ctx, authorKey, ttl)
		}
		queued = true
	}
	if !queued {
		return nil
	}
	_, err := pipe.Exec(ctx)
	inframetrics.ObserveCacheWrite("card", len(cards), err)
	return err
}

// DeleteCards 删除指定视频的卡片缓存，用于视频删除后立即失效。
func (c *feedCardCache) DeleteCards(ctx context.Context, videoIDs []int64) error {
	keys := make([]string, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		if videoID > 0 {
			keys = append(keys, feedCardKey(videoID))
		}
	}
	if len(keys) == 0 {
		return nil
	}
	err := c.client.Del(ctx, keys...).Err()
	inframetrics.ObserveCacheWrite("card", len(keys), err)
	return err
}

// DeleteAuthorCards 按作者维度删除卡片缓存，用于昵称、头像等资料变更后立即失效。
func (c *feedCardCache) DeleteAuthorCards(ctx context.Context, authorID int64) error {
	if authorID <= 0 {
		return nil
	}
	authorKey := feedAuthorCardKey(authorID)
	members, err := c.client.SMembers(ctx, authorKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	keys := make([]string, 0, len(members)+1)
	keys = append(keys, authorKey)
	for _, member := range members {
		videoID, parseErr := strconv.ParseInt(member, 10, 64)
		if parseErr != nil || videoID <= 0 {
			continue
		}
		keys = append(keys, feedCardKey(videoID))
	}
	err = c.client.Del(ctx, keys...).Err()
	inframetrics.ObserveCacheWrite("card", len(keys), err)
	return err
}
