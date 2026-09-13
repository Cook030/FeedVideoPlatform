package infracache

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	domainfeed "GCFeed/internal/domain/feed"

	"github.com/redis/go-redis/v9"
)

// followingIndexCache 维护关注流的推拉混合索引：普通作者写 inbox，大 V 写 outbox。
type followingIndexCache struct {
	client redisWatchCmdable
}

// AddInboxItems 向多个用户的 inbox 写入一条新视频索引（推模式）。
func (c *followingIndexCache) AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	if authorID <= 0 || item == nil || item.VideoID <= 0 || item.PublishedAt.IsZero() || len(userIDs) == 0 {
		return nil
	}
	if maxLen <= 0 {
		maxLen = 1000
	}
	pipe := c.client.Pipeline()
	score := followingIndexScore(item.PublishedAt, item.VideoID)
	member := followingIndexMember(item.VideoID, authorID, item.PublishedAt)
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		key := followingInboxKey(userID)
		pipe.ZAdd(ctx, key, redis.Z{Score: score, Member: member})
		pipe.ZRemRangeByRank(ctx, key, 0, -maxLen-1)
		pipe.Expire(ctx, key, followingIndexKeyTTL)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// AddAuthorOutboxItem 向作者 outbox 写入一条新视频索引（拉模式）。
func (c *followingIndexCache) AddAuthorOutboxItem(ctx context.Context, authorID int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	if authorID <= 0 || item == nil || item.VideoID <= 0 || item.PublishedAt.IsZero() {
		return nil
	}
	if maxLen <= 0 {
		maxLen = 500
	}
	key := followingAuthorOutboxKey(authorID)
	score := followingIndexScore(item.PublishedAt, item.VideoID)
	member := followingIndexMember(item.VideoID, authorID, item.PublishedAt)
	pipe := c.client.Pipeline()
	pipe.ZAdd(ctx, key, redis.Z{Score: score, Member: member})
	pipe.ZRemRangeByRank(ctx, key, 0, -maxLen-1)
	pipe.Expire(ctx, key, followingIndexKeyTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// RemoveInboxAuthor 从用户 inbox 中清理指定作者的条目，用于取关后立即停止展示其视频。
func (c *followingIndexCache) RemoveInboxAuthor(ctx context.Context, userID int64, authorID int64) error {
	if userID <= 0 || authorID <= 0 {
		return nil
	}
	key := followingInboxKey(userID)
	members, err := c.client.ZRange(ctx, key, 0, -1).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	remove := make([]any, 0)
	for _, member := range members {
		item, ok := feedPageItemFromFollowingMember(member)
		if !ok || item.AuthorID != authorID {
			continue
		}
		remove = append(remove, member)
	}
	if len(remove) == 0 {
		return nil
	}
	return c.client.ZRem(ctx, key, remove...).Err()
}

// ListFollowingIndexPage 合并 inbox 和作者 outbox；只有当索引覆盖了全部关注作者时才视为完整，否则回源数据库。
func (c *followingIndexCache) ListFollowingIndexPage(ctx context.Context, viewerID int64, pullAuthorIDs []int64, followedAuthorIDs []int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, bool, error) {
	if viewerID <= 0 || limit <= 0 {
		return []*domainfeed.FeedPageItem{}, false, nil
	}
	keys := []string{followingInboxKey(viewerID)}
	for _, authorID := range pullAuthorIDs {
		if authorID > 0 {
			keys = append(keys, followingAuthorOutboxKey(authorID))
		}
	}

	pipe := c.client.Pipeline()
	cardinalityCommands := make([]*redis.IntCmd, 0, len(keys))
	rangeCommands := make([]*redis.StringSliceCmd, 0, len(keys))
	minScore := "-inf"
	maxScore := "+inf"
	if cursor != nil {
		maxScore = fmt.Sprintf("(%f", followingIndexScore(cursor.PublishedAt, cursor.VideoID))
	}
	for _, key := range keys {
		cardinalityCommands = append(cardinalityCommands, pipe.ZCard(ctx, key))
		rangeCommands = append(rangeCommands, pipe.ZRevRangeByScore(ctx, key, &redis.ZRangeBy{
			Min:   minScore,
			Max:   maxScore,
			Count: int64(limit),
		}))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, false, err
	}

	counts := make([]int64, 0, len(cardinalityCommands))
	for _, cmd := range cardinalityCommands {
		count, err := cmd.Result()
		if err != nil && err != redis.Nil {
			return nil, false, err
		}
		counts = append(counts, count)
	}

	inboxCount := counts[0]
	hasOutbox := false
	for _, count := range counts[1:] {
		if count > 0 {
			hasOutbox = true
			break
		}
	}
	if inboxCount == 0 && !hasOutbox {
		// 索引完全为空，回源数据库冷启动。
		return nil, false, nil
	}
	if inboxCount == 0 && domainfeed.HasSmallAuthors(followedAuthorIDs, pullAuthorIDs) {
		// 关注了非大 V 作者却没有任何 inbox 数据，说明索引只落了一部分，不能当作完整结果。
		return nil, false, nil
	}

	allowedAuthors := int64Set(followedAuthorIDs)
	seen := map[int64]struct{}{}
	items := make([]*domainfeed.FeedPageItem, 0, limit*len(rangeCommands))
	for _, cmd := range rangeCommands {
		members, err := cmd.Result()
		if err != nil && err != redis.Nil {
			return nil, false, err
		}
		for _, member := range members {
			item, ok := feedPageItemFromFollowingMember(member)
			if !ok {
				continue
			}
			if item.AuthorID > 0 {
				if _, followed := allowedAuthors[item.AuthorID]; !followed {
					continue
				}
			}
			if _, exists := seen[item.VideoID]; exists {
				continue
			}
			seen[item.VideoID] = struct{}{}
			items = append(items, item)
		}
	}
	domainfeed.SortPageItemsByTimeline(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, true, nil
}

func followingIndexScore(publishedAt time.Time, videoID int64) float64 {
	return float64(publishedAt.UTC().Unix()*1000000 + videoID%1000000)
}

func followingIndexMember(videoID int64, authorID int64, publishedAt time.Time) string {
	return fmt.Sprintf("%d:%d:%s", videoID, authorID, publishedAt.UTC().Format(time.RFC3339Nano))
}

func feedPageItemFromFollowingMember(member string) (*domainfeed.FeedPageItem, bool) {
	parts := strings.SplitN(member, ":", 3)
	if len(parts) != 2 && len(parts) != 3 {
		return nil, false
	}
	videoID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || videoID <= 0 {
		return nil, false
	}
	authorID := int64(0)
	publishedAtIndex := 1
	if len(parts) == 3 {
		authorID, _ = strconv.ParseInt(parts[1], 10, 64)
		publishedAtIndex = 2
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, parts[publishedAtIndex])
	if err != nil || publishedAt.IsZero() {
		return nil, false
	}
	return &domainfeed.FeedPageItem{
		VideoID:     videoID,
		AuthorID:    authorID,
		PublishedAt: publishedAt,
	}, true
}


