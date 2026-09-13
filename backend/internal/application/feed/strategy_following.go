package applicationfeed

import (
	"context"

	domainfeed "GCFeed/internal/domain/feed"
)

// FollowingStrategy 使用推拉混合模式读取关注流。
type FollowingStrategy struct {
	repo           domainfeed.Repository
	cache          FeedCache
	followingIndex FollowingIndexCache
}

// NewFollowingStrategy 创建关注流推拉混合策略。
func NewFollowingStrategy(repo domainfeed.Repository) *FollowingStrategy {
	return &FollowingStrategy{repo: repo}
}

// Scene 返回关注流场景。
func (s *FollowingStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneFollowing
}

// List 根据当前登录用户读取关注流。
func (s *FollowingStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if req.ViewerID <= 0 {
		return nil, domainfeed.ErrViewerRequired
	}
	parsedCursor, err := parseTimelineCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)

	page, err := s.listPageFromRepo(ctx, req.ViewerID, parsedCursor, limit)
	if err != nil {
		return nil, err
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items, req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	return &FeedResult{
		Scene:      domainfeed.SceneFollowing,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (s *FollowingStrategy) listPageFromRepo(ctx context.Context, viewerID int64, parsedCursor *domainfeed.TimelineCursor, limit int) (*FeedPage, error) {
	items, err := s.listFollowingItems(ctx, viewerID, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeTimelineCursor(&domainfeed.TimelineCursor{
			PublishedAt: items[len(items)-1].PublishedAt,
			VideoID:     items[len(items)-1].VideoID,
		})
	}

	return &FeedPage{
		Scene:      domainfeed.SceneFollowing,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// listFollowingItems 优先读取推拉混合索引，索引不完整时回源仓储。
func (s *FollowingStrategy) listFollowingItems(ctx context.Context, viewerID int64, parsedCursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	if s.followingIndex != nil {
		pullAuthorIDs, err := s.repo.ListFollowingPullAuthorIDs(ctx, viewerID)
		if err != nil {
			return nil, err
		}
		followedAuthorIDs, err := s.repo.ListFollowingAuthorIDs(ctx, viewerID)
		if err != nil {
			return nil, err
		}
		items, ok, err := s.followingIndex.ListFollowingIndexPage(ctx, viewerID, pullAuthorIDs, followedAuthorIDs, parsedCursor, limit)
		if err != nil {
			return nil, err
		}
		if ok {
			return items, nil
		}
	}
	return s.repo.ListFollowingPage(ctx, viewerID, parsedCursor, limit)
}
