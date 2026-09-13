package applicationfeed

import (
	"context"
	"time"

	domainfeed "GCFeed/internal/domain/feed"
	contract "GCFeed/internal/shared/contract"
)

// FeedCache 定义 Feed 页、卡片和计数缓存能力，Redis 实现在基础设施层提供。
type FeedCache interface {
	GetPage(ctx context.Context, key string) (*FeedPage, bool, error)
	SetPage(ctx context.Context, key string, page *FeedPage, ttl time.Duration) error
	GetCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error)
	SetCards(ctx context.Context, cards map[int64]*domainfeed.FeedCard, ttl time.Duration) error
	GetStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error)
	SetStats(ctx context.Context, stats map[int64]*domainfeed.FeedStat, ttl time.Duration) error
	ListHotWindowPage(ctx context.Context, windowEnd time.Time, offset int, limit int) ([]*domainfeed.FeedPageItem, error)
}

// FollowingIndexCache 提供关注流推拉索引的读取能力。
type FollowingIndexCache interface {
	ListFollowingIndexPage(ctx context.Context, viewerID int64, pullAuthorIDs []int64, followedAuthorIDs []int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, bool, error)
}

// Strategy 定义单个 Feed 场景的读取策略。
type Strategy interface {
	Scene() domainfeed.Scene
	List(ctx context.Context, req FeedRequest) (*FeedResult, error)
}

// Recommender 是推荐场景依赖的候选查询端口，契约类型定义在 shared/contract。
type Recommender interface {
	Recommend(ctx context.Context, input contract.CandidateRequest) (*contract.CandidateResult, error)
}
