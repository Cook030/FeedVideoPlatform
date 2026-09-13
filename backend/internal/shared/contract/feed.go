package contract

import domainfeed "GCFeed/internal/domain/feed"

// FeedPage 是 Feed 页缓存中的轻量结果，卡片与计数在组装阶段批量读取。
type FeedPage struct {
	Scene      domainfeed.Scene
	Items      []*domainfeed.FeedPageItem
	NextCursor string
	HasMore    bool
}
