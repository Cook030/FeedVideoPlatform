package infracache

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	domaininteraction "GCFeed/internal/domain/interaction"
)

// 热榜滑动窗口、行为缓存 TTL 与计数分片策略。
// 这里的取值属于缓存实现细节，改动需同步核对 Redis key/TTL 契约。
const (
	hotWindowMinutes            = 60
	hotMinuteBucketTTL          = 2 * time.Hour
	hotWindowCacheTTL           = 15 * time.Second
	actionStateTTL              = 30 * 24 * time.Hour
	actionStatTTL               = 24 * time.Hour
	actionStatJSONTTL           = 15 * time.Second
	actionStatCounterShardCount = 16
	followingIndexKeyTTL        = 30 * 24 * time.Hour
)

// cacheKeys 按 builder 批量生成缓存 key。
func cacheKeys(videoIDs []int64, build func(int64) string) []string {
	keys := make([]string, 0, len(videoIDs))
	for _, videoID := range videoIDs {
		keys = append(keys, build(videoID))
	}
	return keys
}

// cacheValueBytes 兼容 string 与 []byte 两种 Redis 返回值。
func cacheValueBytes(value any) ([]byte, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case string:
		return []byte(typed), true
	case []byte:
		return typed, true
	default:
		return nil, false
	}
}

func int64Set(values []int64) map[int64]struct{} {
	set := map[int64]struct{}{}
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func clampRedisCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// —— Feed 页 / 卡片 / 计数 key ——

func feedCardKey(videoID int64) string {
	return fmt.Sprintf("video:card:v1:%d", videoID)
}

func feedAuthorCardKey(authorID int64) string {
	return fmt.Sprintf("video:card:author:v1:%d", authorID)
}

func feedStatKey(videoID int64) string {
	return fmt.Sprintf("video:stat:v1:%d", videoID)
}

// —— 关注流推拉索引 key ——

func followingInboxKey(userID int64) string {
	return fmt.Sprintf("feed:following:inbox:v1:%d", userID)
}

func followingAuthorOutboxKey(authorID int64) string {
	return fmt.Sprintf("feed:following:author:v1:%d", authorID)
}

// —— 热榜滑动窗口 key ——

func hotMinuteKey(at time.Time) string {
	return fmt.Sprintf("feed:hot:minute:v1:%s", at.UTC().Truncate(time.Minute).Format("200601021504"))
}

func hotWindowKey(windowEnd time.Time) string {
	return fmt.Sprintf("feed:hot:window:v1:%d", windowEnd.UTC().Truncate(time.Minute).Unix())
}

func hotWindowMinuteKeys(windowEnd time.Time) []string {
	keys := make([]string, 0, hotWindowMinutes)
	for index := hotWindowMinutes - 1; index >= 0; index-- {
		keys = append(keys, hotMinuteKey(windowEnd.Add(-time.Duration(index)*time.Minute)))
	}
	return keys
}

func hotRankMember(videoID int64) string {
	return fmt.Sprintf("%020d", videoID)
}

func hotRankVideoID(member string) (int64, bool) {
	value := strings.TrimLeft(member, "0")
	if value == "" {
		return 0, false
	}
	videoID, err := strconv.ParseInt(value, 10, 64)
	return videoID, err == nil && videoID > 0
}

// —— 互动行为与计数分片 key ——

func interactionActionKey(userID int64, videoID int64, actionType string) string {
	return fmt.Sprintf("interaction:action:v1:%d:%d:%s", userID, videoID, strings.ToLower(actionType))
}

func interactionStatCounterKey(videoID int64) string {
	return fmt.Sprintf("video:stat:counter:v1:%d", videoID)
}

func interactionStatCounterBaseKey(videoID int64) string {
	return fmt.Sprintf("%s:base", interactionStatCounterKey(videoID))
}

func interactionStatCounterShardKey(videoID int64, shard int) string {
	return fmt.Sprintf("%s:shard:%02d", interactionStatCounterKey(videoID), shard)
}

func interactionStatCounterShardKeys(videoID int64) []string {
	keys := make([]string, 0, actionStatCounterShardCount)
	for shard := 0; shard < actionStatCounterShardCount; shard++ {
		keys = append(keys, interactionStatCounterShardKey(videoID, shard))
	}
	return keys
}

func interactionStatCounterShardIndex(userID int64) int {
	if userID <= 0 {
		return 0
	}
	return int(userID % actionStatCounterShardCount)
}

func interactionStatField(actionType string) string {
	if actionType == domaininteraction.ActionTypeLike {
		return "like_count"
	}
	return "favorite_count"
}
