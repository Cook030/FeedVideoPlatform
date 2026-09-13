package applicationfeed

import (
	"context"
	"errors"

	domainfeed "GCFeed/internal/domain/feed"
	contract "GCFeed/internal/shared/contract"
)

// RecommendStrategy 使用推荐服务读取排序后的候选，再复用 Feed 卡片组装。
type RecommendStrategy struct {
	repo        domainfeed.Repository
	cache       FeedCache
	recommender Recommender
}

// NewRecommendStrategy 创建推荐 Feed 策略。
func NewRecommendStrategy(repo domainfeed.Repository, recommender Recommender) *RecommendStrategy {
	return &RecommendStrategy{
		repo:        repo,
		recommender: recommender,
	}
}

// Scene 返回推荐场景。
func (s *RecommendStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneRecommend
}

// List 读取推荐候选，并按推荐服务给出的顺序组装 Feed 卡片。
func (s *RecommendStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if req.ViewerID <= 0 {
		return nil, domainfeed.ErrViewerRequired
	}
	limit := normalizeLimit(req.Limit)
	result, err := s.recommender.Recommend(ctx, contract.CandidateRequest{
		UserID:    req.ViewerID,
		Scene:     string(domainfeed.SceneRecommend),
		RequestID: clientContextValue(req.ClientContext, "request_id"),
		Cursor:    req.Cursor,
		Limit:     limit,
	})
	if err != nil {
		if errors.Is(err, contract.ErrRecommendationUnavailable) {
			return nil, ErrLoadFeedFailed
		}
		return nil, err
	}

	pageItems := make([]*domainfeed.FeedPageItem, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		pageItems = append(pageItems, &domainfeed.FeedPageItem{
			VideoID:     candidate.VideoID,
			AuthorID:    candidate.AuthorID,
			PublishedAt: candidate.PublishedAt,
			HotScore:    candidate.HotScore,
		})
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, pageItems, req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}
	return &FeedResult{
		Scene:      domainfeed.SceneRecommend,
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}
