package infracache

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	domainfeed "GCFeed/internal/domain/feed"
	domaininteraction "GCFeed/internal/domain/interaction"
	inframetrics "GCFeed/internal/infra/metrics"

	"github.com/redis/go-redis/v9"
)

// feedStatCache 负责视频计数缓存：JSON 快照 + 分片增量累加 + 周期性收敛。
type feedStatCache struct {
	client redisWatchCmdable
}

// GetStats 批量读取视频计数缓存。
func (c *feedStatCache) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	return getStats(ctx, c.client, videoIDs)
}

// SetStats 批量写入视频计数缓存。
func (c *feedStatCache) SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error {
	pipe := c.client.Pipeline()
	queued := false

	for _, stat := range stats {
		if stat == nil || stat.VideoID <= 0 {
			continue
		}
		content, err := json.Marshal(stat)
		if err != nil {
			return err
		}
		pipe.Set(ctx, feedStatKey(stat.VideoID), content, ttl)
		queued = true
	}
	if !queued {
		return nil
	}
	_, err := pipe.Exec(ctx)
	inframetrics.ObserveCacheWrite("stat", len(stats), err)
	return err
}

// SetVideoStat 写入单个视频的计数缓存，用于评论写入后刷新 Feed 展示。
func (c *feedStatCache) SetVideoStat(ctx context.Context, stat *domaininteraction.VideoStat) error {
	if stat == nil || stat.VideoID <= 0 {
		return nil
	}
	err := setActionStatJSON(ctx, c.client, feedStatKey(stat.VideoID), videoStatToFeedStat(stat))
	inframetrics.ObserveCacheWrite("stat", 1, err)
	return err
}

// ReconcileActionStat 用数据库权威计数重置基数并清空分片增量，让 Redis 计数周期性收敛回真实值。
func (c *feedStatCache) ReconcileActionStat(ctx context.Context, stat *domaininteraction.VideoStat) error {
	if stat == nil || stat.VideoID <= 0 {
		return nil
	}
	counterBaseKey := interactionStatCounterBaseKey(stat.VideoID)
	pipe := c.client.Pipeline()
	pipe.HSet(ctx, counterBaseKey, map[string]any{
		"like_count":     stat.LikeCount,
		"comment_count":  stat.CommentCount,
		"favorite_count": stat.FavoriteCount,
	})
	pipe.Expire(ctx, counterBaseKey, actionStatTTL)
	pipe.Del(ctx, interactionStatCounterShardKeys(stat.VideoID)...)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return err
	}
	return setActionStatJSON(ctx, c.client, feedStatKey(stat.VideoID), videoStatToFeedStat(stat))
}

func getStats(ctx context.Context, client redisStatCacheClient, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	stats := map[int64]*domainfeed.FeedStat{}
	if len(videoIDs) == 0 {
		return stats, nil
	}

	values, err := client.MGet(ctx, cacheKeys(videoIDs, feedStatKey)...).Result()
	if err != nil {
		inframetrics.ObserveCacheRead("stat", len(videoIDs), 0, err)
		return nil, err
	}
	for index, value := range values {
		content, ok := cacheValueBytes(value)
		if !ok {
			continue
		}
		var stat domainfeed.FeedStat
		if err := json.Unmarshal(content, &stat); err != nil {
			continue
		}
		if stat.VideoID <= 0 {
			stat.VideoID = videoIDs[index]
		}
		stats[stat.VideoID] = &stat
	}
	for _, videoID := range videoIDs {
		if stats[videoID] != nil {
			continue
		}
		stat, ok, err := actionStatFromCache(ctx, client, videoID)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		stats[videoID] = stat
		_ = setActionStatJSON(ctx, client, feedStatKey(videoID), stat)
	}
	inframetrics.ObserveCacheRead("stat", len(videoIDs), len(stats), nil)
	return stats, nil
}

func videoStatToFeedStat(stat *domaininteraction.VideoStat) *domainfeed.FeedStat {
	if stat == nil {
		return nil
	}
	return &domainfeed.FeedStat{
		VideoID:       stat.VideoID,
		LikeCount:     stat.LikeCount,
		CommentCount:  stat.CommentCount,
		FavoriteCount: stat.FavoriteCount,
	}
}

func actionStat(ctx context.Context, client redisActionStatReader, counterBaseKey string, counterShardKeys []string, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, error) {
	stat, _, err := actionStatWithPresence(ctx, client, counterBaseKey, counterShardKeys, jsonKey, videoID, initialStat)
	if err != nil {
		return nil, err
	}
	if stat == nil {
		return &domainfeed.FeedStat{VideoID: videoID}, nil
	}
	return stat, nil
}

func actionStatFromCache(ctx context.Context, client redisActionStatReader, videoID int64) (*domainfeed.FeedStat, bool, error) {
	return actionStatWithPresence(ctx, client, interactionStatCounterBaseKey(videoID), interactionStatCounterShardKeys(videoID), feedStatKey(videoID), videoID, nil)
}

func actionStatWithPresence(ctx context.Context, client redisActionStatReader, counterBaseKey string, counterShardKeys []string, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, bool, error) {
	stat := &domainfeed.FeedStat{VideoID: videoID}
	found := false
	values, err := client.HGetAll(ctx, counterBaseKey).Result()
	if err != nil {
		return nil, false, err
	}
	if len(values) > 0 {
		applyActionStatFields(stat, values)
		found = true
	} else {
		fallbackStat, ok, err := actionStatFallback(ctx, client, jsonKey, videoID, initialStat)
		if err != nil {
			return nil, false, err
		}
		if ok {
			stat = fallbackStat
			found = true
		}
	}
	shardFound, err := applyActionStatShardDeltas(ctx, client, stat, counterShardKeys)
	if err != nil {
		return nil, false, err
	}
	found = found || shardFound
	if !found {
		return nil, false, nil
	}
	return stat, true, nil
}

func actionStatFallback(ctx context.Context, client redisActionStatReader, jsonKey string, videoID int64, initialStat *domaininteraction.VideoStat) (*domainfeed.FeedStat, bool, error) {
	stat := &domainfeed.FeedStat{VideoID: videoID}
	content, err := client.Get(ctx, jsonKey).Bytes()
	if err == redis.Nil {
		if initialStat != nil {
			stat.LikeCount = initialStat.LikeCount
			stat.CommentCount = initialStat.CommentCount
			stat.FavoriteCount = initialStat.FavoriteCount
			return stat, true, nil
		}
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(content, stat); err != nil {
		return nil, false, nil
	}
	if stat.VideoID <= 0 {
		stat.VideoID = videoID
	}
	return stat, true, nil
}

func actionStatBaseInit(videoID int64, initialStat *domaininteraction.VideoStat) *domaininteraction.VideoStat {
	if initialStat != nil {
		return initialStat
	}
	return &domaininteraction.VideoStat{VideoID: videoID}
}

func queueActionStatBaseInit(ctx context.Context, pipe redis.Pipeliner, counterBaseKey string, initialStat *domaininteraction.VideoStat) {
	stat := &domaininteraction.VideoStat{}
	if initialStat != nil {
		stat = initialStat
	}
	pipe.HSetNX(ctx, counterBaseKey, "like_count", stat.LikeCount)
	pipe.HSetNX(ctx, counterBaseKey, "comment_count", stat.CommentCount)
	pipe.HSetNX(ctx, counterBaseKey, "favorite_count", stat.FavoriteCount)
}

func setActionStatJSON(ctx context.Context, client redisActionStatWriter, jsonKey string, stat *domainfeed.FeedStat) error {
	content, err := json.Marshal(stat)
	if err != nil {
		return err
	}
	return client.Set(ctx, jsonKey, content, actionStatJSONTTL).Err()
}

func applyActionStatShardDeltas(ctx context.Context, client redisActionStatReader, stat *domainfeed.FeedStat, shardKeys []string) (bool, error) {
	if stat == nil || len(shardKeys) == 0 {
		return false, nil
	}

	shardValues, err := loadActionStatShardValues(ctx, client, shardKeys)
	if err != nil {
		return false, err
	}
	found := false
	likeDelta := 0
	favoriteDelta := 0
	for _, values := range shardValues {
		if len(values) > 0 {
			found = true
		}
		likeDelta += actionStatFieldInt(values, "like_count")
		favoriteDelta += actionStatFieldInt(values, "favorite_count")
	}
	stat.LikeCount = clampRedisCount(stat.LikeCount + likeDelta)
	stat.FavoriteCount = clampRedisCount(stat.FavoriteCount + favoriteDelta)
	return found, nil
}

func loadActionStatShardValues(ctx context.Context, client redisActionStatReader, shardKeys []string) ([]map[string]string, error) {
	type pipelineProvider interface {
		Pipeline() redis.Pipeliner
	}

	if provider, ok := client.(pipelineProvider); ok {
		pipe := provider.Pipeline()
		cmds := make([]*redis.MapStringStringCmd, 0, len(shardKeys))
		for _, key := range shardKeys {
			cmds = append(cmds, pipe.HGetAll(ctx, key))
		}
		if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
			return nil, err
		}
		values := make([]map[string]string, 0, len(cmds))
		for _, cmd := range cmds {
			value, err := cmd.Result()
			if err != nil && err != redis.Nil {
				return nil, err
			}
			values = append(values, value)
		}
		return values, nil
	}

	values := make([]map[string]string, 0, len(shardKeys))
	for _, key := range shardKeys {
		value, err := client.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func applyActionStatFields(stat *domainfeed.FeedStat, values map[string]string) {
	if stat == nil {
		return
	}
	stat.LikeCount = actionStatFieldInt(values, "like_count")
	stat.CommentCount = actionStatFieldInt(values, "comment_count")
	stat.FavoriteCount = actionStatFieldInt(values, "favorite_count")
}

func actionStatFieldInt(values map[string]string, field string) int {
	value, _ := strconv.Atoi(values[field])
	return value
}
