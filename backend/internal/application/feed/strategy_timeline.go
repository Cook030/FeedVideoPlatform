package applicationfeed

import (
	"context"
	"time"

	domainfeed "GCFeed/internal/domain/feed"

	"golang.org/x/sync/singleflight"
)

// TimelineStrategy 复用现有时间线查询能力，是 latest 等场景的基础实现。
type TimelineStrategy struct {
	scene        domainfeed.Scene
	repo         domainfeed.Repository
	cache        FeedCache
	firstPageTTL time.Duration
	pageTTL      time.Duration
	group        singleflight.Group
}

// NewTimelineStrategy 创建一个时间线排序策略。
func NewTimelineStrategy(scene domainfeed.Scene, repo domainfeed.Repository) *TimelineStrategy {
	return &TimelineStrategy{
		scene:        domainfeed.NormalizeScene(scene),
		repo:         repo,
		firstPageTTL: timelineFirstPageCacheTTL,
		pageTTL:      timelinePageCacheTTL,
	}
}

// Scene 返回当前策略绑定的 Feed 场景。
func (s *TimelineStrategy) Scene() domainfeed.Scene {
	return s.scene
}

// List 使用 cursor+limit 读取时间线 Feed。
func (s *TimelineStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	parsedCursor, err := parseTimelineCursor(req.Cursor)
	if err != nil {
		return nil, err
	}
	limit := normalizeLimit(req.Limit)

	page, err := loadFeedPage(ctx, s.cache, s.scene, req.Cursor, limit, s.firstPageTTL, s.pageTTL, &s.group, func() (*FeedPage, error) {
		return s.listPageFromRepo(ctx, parsedCursor, limit)
	})
	if err != nil {
		return nil, err
	}

	items, err := assembleFeedItems(ctx, s.repo, s.cache, page.Items, req.ViewerID)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	return &FeedResult{
		Scene:      s.scene,
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
	}, nil
}

func (s *TimelineStrategy) listPageFromRepo(ctx context.Context, parsedCursor *domainfeed.TimelineCursor, limit int) (*FeedPage, error) {
	items, err := s.repo.ListTimelinePage(ctx, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadFeedFailed
	}

	// limit+1 是常见分页技巧：多取一条即可判断后面还有没有数据。
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		// 下一页从当前页最后一个元素之后开始，游标保存排序所需的两个字段。
		nextCursor = encodeTimelineCursor(&domainfeed.TimelineCursor{
			PublishedAt: items[len(items)-1].PublishedAt,
			VideoID:     items[len(items)-1].VideoID,
		})
	}

	return &FeedPage{
		Scene:      s.scene,
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
