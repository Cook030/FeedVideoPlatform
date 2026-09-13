package applicationfeed

import (
	"context"
	"strings"
	"time"

	domainfeed "GCFeed/internal/domain/feed"
)

// HotStrategy 使用互动热度读取热榜 Feed。
type HotStrategy struct {
	repo  domainfeed.Repository
	cache FeedCache
}

// NewHotStrategy 创建热榜排序策略。
func NewHotStrategy(repo domainfeed.Repository) *HotStrategy {
	return &HotStrategy{repo: repo}
}

// Scene 返回热榜场景。
func (s *HotStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneHot
}

// List 读取热榜；Redis 场景使用最近一小时分钟桶，基础场景使用仓储累计热度。
func (s *HotStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	parsedCursor, err := parseHotCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)

	var page *FeedPage
	if s.cache != nil {
		if strings.TrimSpace(req.Cursor) != "" && (parsedCursor == nil || parsedCursor.WindowEnd.IsZero()) {
			return nil, domainfeed.ErrInvalidCursor
		}
		page, err = s.listPageFromHotWindow(ctx, parsedCursor, limit)
	} else {
		page, err = s.listPageFromRepo(ctx, parsedCursor, limit)
	}
	if err != nil {
		return nil, err
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items, req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	return &FeedResult{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (s *HotStrategy) listPageFromHotWindow(ctx context.Context, parsedCursor *domainfeed.HotCursor, limit int) (*FeedPage, error) {
	windowEnd := time.Now().UTC().Truncate(time.Minute)
	offset := 0
	if parsedCursor != nil && !parsedCursor.WindowEnd.IsZero() {
		windowEnd = parsedCursor.WindowEnd.UTC().Truncate(time.Minute)
		offset = parsedCursor.Offset
	}

	items, err := s.cache.ListHotWindowPage(ctx, windowEnd, offset, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeHotWindowCursor(&domainfeed.HotCursor{
			WindowEnd: windowEnd,
			Offset:    offset + len(items),
		})
	}

	return &FeedPage{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

func (s *HotStrategy) listPageFromRepo(ctx context.Context, parsedCursor *domainfeed.HotCursor, limit int) (*FeedPage, error) {
	items, err := s.repo.ListHotPage(ctx, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeHotCursor(&domainfeed.HotCursor{
			HotScore:    last.HotScore,
			PublishedAt: last.PublishedAt,
			VideoID:     last.VideoID,
		})
	}

	return &FeedPage{
		Scene:      domainfeed.SceneHot,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
