package infracache

import (
	"context"
	"time"

	domainfeed "GCFeed/internal/domain/feed"
	domaininteraction "GCFeed/internal/domain/interaction"
	contract "GCFeed/internal/shared/contract"
)

// FeedCache 是 Feed 与互动缓存的组合门面。
//
// 对外保持既有方法签名不变；内部按职责委托给六个子组件：
// 页缓存、卡片缓存、计数缓存、关注流索引、热榜窗口与行为状态。
type FeedCache struct {
	page   *feedPageCache
	card   *feedCardCache
	stat   *feedStatCache
	follow *followingIndexCache
	hot    *hotRankCache
	action *actionStateCache
}

// NewFeedCache 创建 Feed 结果缓存。
func NewFeedCache(client redisWatchCmdable) *FeedCache {
	return &FeedCache{
		page:   &feedPageCache{client: client},
		card:   &feedCardCache{client: client},
		stat:   &feedStatCache{client: client},
		follow: &followingIndexCache{client: client},
		hot:    &hotRankCache{client: client},
		action: &actionStateCache{client: client},
	}
}

// GetPage 读取缓存中的轻量 Feed 页。
func (c *FeedCache) GetPage(ctx context.Context, key string) (*contract.FeedPage, bool, error) {
	return c.page.GetPage(ctx, key)
}

// SetPage 写入轻量 Feed 页，并设置过期时间。
func (c *FeedCache) SetPage(ctx context.Context, key string, page *contract.FeedPage, ttl time.Duration) error {
	return c.page.SetPage(ctx, key, page, ttl)
}

// GetCards 批量读取视频卡片缓存。
func (c *FeedCache) GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	return c.card.GetCards(ctx, videoIDs)
}

// SetCards 批量写入视频卡片缓存。
func (c *FeedCache) SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error {
	return c.card.SetCards(ctx, cards, ttl)
}

// DeleteCards 删除指定视频的卡片缓存。
func (c *FeedCache) DeleteCards(ctx context.Context, videoIDs []int64) error {
	return c.card.DeleteCards(ctx, videoIDs)
}

// DeleteAuthorCards 按作者维度删除卡片缓存。
func (c *FeedCache) DeleteAuthorCards(ctx context.Context, authorID int64) error {
	return c.card.DeleteAuthorCards(ctx, authorID)
}

// GetStats 批量读取视频计数缓存。
func (c *FeedCache) GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	return c.stat.GetStats(ctx, videoIDs)
}

// SetStats 批量写入视频计数缓存。
func (c *FeedCache) SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error {
	return c.stat.SetStats(ctx, stats, ttl)
}

// SetVideoStat 写入单个视频的计数缓存。
func (c *FeedCache) SetVideoStat(ctx context.Context, stat *domaininteraction.VideoStat) error {
	return c.stat.SetVideoStat(ctx, stat)
}

// ReconcileActionStat 用数据库权威计数重置基数并清空分片增量。
func (c *FeedCache) ReconcileActionStat(ctx context.Context, stat *domaininteraction.VideoStat) error {
	return c.stat.ReconcileActionStat(ctx, stat)
}

// AddInboxItems 向多个用户的 inbox 写入一条新视频索引（推模式）。
func (c *FeedCache) AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	return c.follow.AddInboxItems(ctx, authorID, userIDs, item, maxLen)
}

// AddAuthorOutboxItem 向作者 outbox 写入一条新视频索引（拉模式）。
func (c *FeedCache) AddAuthorOutboxItem(ctx context.Context, authorID int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	return c.follow.AddAuthorOutboxItem(ctx, authorID, item, maxLen)
}

// RemoveInboxAuthor 从用户 inbox 中清理指定作者的条目。
func (c *FeedCache) RemoveInboxAuthor(ctx context.Context, userID int64, authorID int64) error {
	return c.follow.RemoveInboxAuthor(ctx, userID, authorID)
}

// ListFollowingIndexPage 合并 inbox 与作者 outbox，返回关注流索引页。
func (c *FeedCache) ListFollowingIndexPage(ctx context.Context, viewerID int64, pullAuthorIDs []int64, followedAuthorIDs []int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, bool, error) {
	return c.follow.ListFollowingIndexPage(ctx, viewerID, pullAuthorIDs, followedAuthorIDs, cursor, limit)
}

// AddHotScore 把一次互动热度写入 1 分钟粒度的热榜桶。
func (c *FeedCache) AddHotScore(ctx context.Context, videoID int64, scoreDelta int, at time.Time) error {
	return c.hot.AddHotScore(ctx, videoID, scoreDelta, at)
}

// ListHotWindowPage 返回一小时滑动窗口内的热榜页。
func (c *FeedCache) ListHotWindowPage(ctx context.Context, windowEnd time.Time, offset int, limit int) ([]*domainfeed.FeedPageItem, error) {
	return c.hot.ListHotWindowPage(ctx, windowEnd, offset, limit)
}

// SetActionState 写入 Redis 行为状态和实时计数。
func (c *FeedCache) SetActionState(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string, initialStat *domaininteraction.VideoStat) (*contract.ActionStateResult, error) {
	return c.action.SetActionState(ctx, userID, videoID, actionType, active, idempotencyKey, initialStat)
}
