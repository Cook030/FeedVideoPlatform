package applicationfeed

import (
	"context"
	"time"

	domainfeed "GCFeed/internal/domain/feed"
	contract "GCFeed/internal/shared/contract"
)

const defaultFeedLimit = 10
const timelineFirstPageCacheTTL = 5 * time.Second
const timelinePageCacheTTL = 45 * time.Second
const feedCardCacheTTL = 15 * time.Minute
const feedStatCacheTTL = 15 * time.Second

// Service 通过 scene 策略注册表分发不同 Feed 场景。
type Service struct {
	repo         domainfeed.Repository
	strategies   map[domainfeed.Scene]Strategy
	defaultScene domainfeed.Scene
	observer     contract.FeedObserver
}

// Option 用于在装配阶段注册额外 Feed 策略。
type Option func(*Service)

// WithStrategy 注册一个额外 Feed 策略。
func WithStrategy(strategy Strategy) Option {
	return func(s *Service) {
		s.RegisterStrategy(strategy)
	}
}

// WithObserver 注入 Feed 观测端口；未注入时不采集指标。
func WithObserver(observer contract.FeedObserver) Option {
	return func(s *Service) {
		s.observer = observer
	}
}

// WithFeedCache 为 Feed 页、卡片和计数启用读缓存。
func WithFeedCache(cache FeedCache) Option {
	return func(s *Service) {
		for _, strategy := range s.strategies {
			switch typed := strategy.(type) {
			case *TimelineStrategy:
				typed.cache = cache
			case *HotStrategy:
				typed.cache = cache
			case *FollowingStrategy:
				typed.cache = cache
				if index, ok := cache.(FollowingIndexCache); ok {
					typed.followingIndex = index
				}
			case *RecommendStrategy:
				typed.cache = cache
			}
		}
	}
}

// WithRecommender 注册推荐 Feed 策略。
func WithRecommender(recommender Recommender) Option {
	return func(s *Service) {
		if recommender != nil {
			s.RegisterStrategy(NewRecommendStrategy(s.repo, recommender))
		}
	}
}

// New 注入 Feed 仓储并注册默认时间线策略。
func New(repo domainfeed.Repository, options ...Option) *Service {
	service := &Service{
		repo:         repo,
		strategies:   map[domainfeed.Scene]Strategy{},
		defaultScene: domainfeed.DefaultScene,
	}
	service.RegisterStrategy(NewTimelineStrategy(domainfeed.SceneTimeline, repo))
	service.RegisterStrategy(NewHotStrategy(repo))
	service.RegisterStrategy(NewFollowingStrategy(repo))
	for _, option := range options {
		option(service)
	}
	return service
}

// RegisterStrategy 把 scene 和具体策略绑定，新增 Feed 类型时在装配层调用。
func (s *Service) RegisterStrategy(strategy Strategy) {
	if strategy == nil {
		return
	}
	scene := domainfeed.NormalizeScene(strategy.Scene())
	s.strategies[scene] = strategy
}

// GetFeed 根据 scene 选择策略并返回分页结果。
func (s *Service) GetFeed(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	start := time.Now()
	req.Scene = domainfeed.NormalizeScene(req.Scene)
	if req.Scene == "" {
		req.Scene = s.defaultScene
	}

	strategy, ok := s.strategies[req.Scene]
	if !ok {
		s.observeFeed(string(req.Scene), time.Since(start), 0, domainfeed.ErrUnsupportedScene)
		return nil, domainfeed.ErrUnsupportedScene
	}
	result, err := strategy.List(ctx, req)
	itemCount := 0
	if result != nil {
		itemCount = len(result.Items)
	}
	s.observeFeed(string(req.Scene), time.Since(start), itemCount, err)
	return result, err
}

// observeFeed 通过端口采集指标，未注入观测实现时静默跳过。
func (s *Service) observeFeed(scene string, duration time.Duration, itemCount int, err error) {
	if s == nil || s.observer == nil {
		return
	}
	s.observer.ObserveFeed(scene, duration, itemCount, err)
}

// GetTimelineFeed 使用 cursor+limit 读取时间线 Feed。
func (s *Service) GetTimelineFeed(ctx context.Context, cursor string, limit int) (*FeedResult, error) {
	return s.GetFeed(ctx, FeedRequest{
		Scene:  domainfeed.SceneTimeline,
		Cursor: cursor,
		Limit:  limit,
	})
}

// RefreshFeed 从第一页重新加载默认 Feed，适合下拉刷新场景。
func (s *Service) RefreshFeed(ctx context.Context, limit int) (*FeedResult, error) {
	return s.GetTimelineFeed(ctx, "", limit)
}
