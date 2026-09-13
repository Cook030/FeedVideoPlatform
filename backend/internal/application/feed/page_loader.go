package applicationfeed

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	domainfeed "GCFeed/internal/domain/feed"

	"golang.org/x/sync/singleflight"
)

// loadFeedPage 读取页缓存；未命中时用 singleflight 合并同 key 的回源请求。
func loadFeedPage(ctx context.Context, cache FeedCache, scene domainfeed.Scene, cursor string, limit int, firstPageTTL time.Duration, pageTTL time.Duration, group *singleflight.Group, load func() (*FeedPage, error)) (*FeedPage, error) {
	if cache == nil || group == nil {
		return load()
	}

	cacheKey := feedPageCacheKey(scene, cursor, limit)
	if page, ok, err := cache.GetPage(ctx, cacheKey); err == nil && ok {
		return page, nil
	}

	value, err, _ := group.Do(cacheKey, func() (any, error) {
		if page, ok, err := cache.GetPage(ctx, cacheKey); err == nil && ok {
			return page, nil
		}
		page, err := load()
		if err != nil {
			return nil, err
		}
		_ = cache.SetPage(ctx, cacheKey, page, feedPageCacheTTL(cursor, cacheKey, firstPageTTL, pageTTL))
		return page, nil
	})
	if err != nil {
		return nil, err
	}
	page, ok := value.(*FeedPage)
	if !ok {
		return nil, ErrLoadFeedFailed
	}
	return page, nil
}

// assembleFeedItems 批量补齐卡片、计数与当前用户的互动状态，组装成 Feed 条目。
func assembleFeedItems(ctx context.Context, repo domainfeed.Repository, cache FeedCache, pageItems []*domainfeed.FeedPageItem, viewerID int64) ([]*domainfeed.FeedItem, error) {
	videoIDs := feedPageVideoIDs(pageItems)
	if len(videoIDs) == 0 {
		return []*domainfeed.FeedItem{}, nil
	}

	cards := map[int64]*domainfeed.FeedCard{}
	stats := map[int64]*domainfeed.FeedStat{}
	if cache != nil {
		if cachedCards, err := cache.GetCards(ctx, videoIDs); err == nil {
			cards = cachedCards
		}
		if cachedStats, err := cache.GetStats(ctx, videoIDs); err == nil {
			stats = cachedStats
		}
	}

	missingCardIDs := missingCardIDs(videoIDs, cards)
	if len(missingCardIDs) > 0 {
		loadedCards, err := repo.BatchGetFeedCards(ctx, missingCardIDs)
		if err != nil {
			return nil, err
		}
		mergeCards(cards, loadedCards)
		if cache != nil {
			_ = cache.SetCards(ctx, loadedCards, feedCardCacheTTL)
		}
	}

	missingStatIDs := missingStatIDs(videoIDs, stats)
	if len(missingStatIDs) > 0 {
		loadedStats, err := repo.BatchGetFeedStats(ctx, missingStatIDs)
		if err != nil {
			return nil, err
		}
		mergeStats(stats, loadedStats)
		if cache != nil {
			_ = cache.SetStats(ctx, loadedStats, feedStatCacheTTL)
		}
	}

	viewerActions := map[int64]*domainfeed.ViewerActionState{}
	if viewerID > 0 {
		loadedViewerActions, err := repo.BatchGetViewerActionStates(ctx, viewerID, videoIDs)
		if err != nil {
			return nil, err
		}
		viewerActions = loadedViewerActions
	}

	items := make([]*domainfeed.FeedItem, 0, len(pageItems))
	for _, pageItem := range pageItems {
		if pageItem == nil {
			continue
		}
		card, ok := cards[pageItem.VideoID]
		if !ok || card == nil {
			continue
		}
		stat := stats[pageItem.VideoID]
		if stat == nil {
			stat = &domainfeed.FeedStat{VideoID: pageItem.VideoID}
		}
		publishedAt := pageItem.PublishedAt
		if publishedAt.IsZero() {
			publishedAt = card.PublishedAt
		}
		item := domainfeed.RestoreFeedItem(
			card.VideoID,
			card.AuthorID,
			card.AuthorNickname,
			card.AuthorAvatarURL,
			card.Title,
			card.Description,
			card.MediaURL,
			card.CoverURL,
			stat.LikeCount,
			stat.CommentCount,
			stat.FavoriteCount,
			publishedAt,
		)
		if action := viewerActions[item.VideoID]; action != nil {
			item.Liked = action.Liked
			item.Favorited = action.Favorited
		}
		item.HotScore = pageItem.HotScore
		items = append(items, item)
	}
	return items, nil
}

func feedPageVideoIDs(items []*domainfeed.FeedPageItem) []int64 {
	videoIDs := make([]int64, 0, len(items))
	seen := map[int64]struct{}{}
	for _, item := range items {
		if item == nil || item.VideoID <= 0 {
			continue
		}
		if _, ok := seen[item.VideoID]; ok {
			continue
		}
		seen[item.VideoID] = struct{}{}
		videoIDs = append(videoIDs, item.VideoID)
	}
	return videoIDs
}

func missingCardIDs(videoIDs []int64, cards map[int64]*domainfeed.FeedCard) []int64 {
	missing := make([]int64, 0)
	for _, videoID := range videoIDs {
		if cards[videoID] == nil {
			missing = append(missing, videoID)
		}
	}
	return missing
}

func missingStatIDs(videoIDs []int64, stats map[int64]*domainfeed.FeedStat) []int64 {
	missing := make([]int64, 0)
	for _, videoID := range videoIDs {
		if stats[videoID] == nil {
			missing = append(missing, videoID)
		}
	}
	return missing
}

func mergeCards(target map[int64]*domainfeed.FeedCard, source map[int64]*domainfeed.FeedCard) {
	for videoID, card := range source {
		if card != nil {
			target[videoID] = card
		}
	}
}

func mergeStats(target map[int64]*domainfeed.FeedStat, source map[int64]*domainfeed.FeedStat) {
	for videoID, stat := range source {
		if stat != nil {
			target[videoID] = stat
		}
	}
}

func feedPageCacheKey(scene domainfeed.Scene, cursor string, limit int) string {
	scene = domainfeed.NormalizeScene(scene)
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return fmt.Sprintf("feed:page:v1:%s:limit:%d:first", scene, limit)
	}

	sum := sha1.Sum([]byte(cursor))
	return fmt.Sprintf("feed:page:v1:%s:limit:%d:cursor:%s", scene, limit, hex.EncodeToString(sum[:]))
}

func feedPageCacheTTL(cursor string, cacheKey string, firstPageTTL time.Duration, pageTTL time.Duration) time.Duration {
	ttl := pageTTL
	if strings.TrimSpace(cursor) == "" {
		ttl = firstPageTTL
	}
	if ttl <= 0 {
		return 0
	}

	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(cacheKey))
	jitterPercent := 10 + int(hasher.Sum32()%11)
	return ttl + time.Duration(jitterPercent)*ttl/100
}
