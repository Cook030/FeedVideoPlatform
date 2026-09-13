package applicationfeed

import (
	"errors"

	domainfeed "GCFeed/internal/domain/feed"
	contract "GCFeed/internal/shared/contract"
)

var ErrLoadFeedFailed = errors.New("failed to load feed")

// FeedRequest 是所有 Feed 场景共用的查询参数。
type FeedRequest struct {
	Scene         domainfeed.Scene
	Cursor        string
	Limit         int
	ViewerID      int64
	ClientContext map[string]string
}

// FeedResult 是游标分页结果，NextCursor 供客户端请求下一页。
type FeedResult struct {
	Scene      domainfeed.Scene
	Items      []*domainfeed.FeedItem
	NextCursor string
	HasMore    bool
}

// FeedPage 是页缓存中的轻量结果，卡片和计数会在组装阶段批量读取。
// 契约定义在 shared/contract，保留类型别名使缓存实现与测试无需改动。
type FeedPage = contract.FeedPage

// timelineCursorPayload 是时间线游标的序列化形式。
type timelineCursorPayload struct {
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
}

// hotCursorPayload 是热榜游标的序列化形式，兼容窗口游标与仓储游标。
type hotCursorPayload struct {
	HotScore    int    `json:"hot_score"`
	PublishedAt string `json:"published_at"`
	VideoID     int64  `json:"video_id"`
	WindowEnd   string `json:"window_end,omitempty"`
	Offset      int    `json:"offset,omitempty"`
}
