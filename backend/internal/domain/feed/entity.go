package domainfeed

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	MaxLimit = 100

	// BigCreatorFollowerThreshold 定义大 V 阈值，粉丝数达到该值的作者走关注流拉模式。
	BigCreatorFollowerThreshold = 10000

	// 热榜权重：评论权重最高，收藏次之，点赞提供基础热度。
	// 这是热榜打分口径的唯一权威定义，Go 与 SQL 两侧都必须引用这里。
	HotScoreLikeWeight     = 3
	HotScoreCommentWeight  = 5
	HotScoreFavoriteWeight = 4
)

// Scene 表示不同 Feed 场景，应用层通过场景选择对应策略。
type Scene string

const (
	SceneTimeline  Scene = "timeline"
	SceneRecommend Scene = "recommend"
	SceneFollowing Scene = "following"
	SceneHot       Scene = "hot"

	DefaultScene = SceneTimeline
)

// FeedItem 是 Feed 页面需要展示的一条视频卡片数据。
type FeedItem struct {
	VideoID         int64
	AuthorID        int64
	AuthorNickname  string
	AuthorAvatarURL string
	Title           string
	Description     string
	MediaURL        string
	CoverURL        string
	LikeCount       int
	CommentCount    int
	FavoriteCount   int
	Liked           bool
	Favorited       bool
	HotScore        int
	PublishedAt     time.Time
}

// FeedPageItem 是 Feed 页缓存中的轻量条目，只保存排序和组装所需字段。
type FeedPageItem struct {
	VideoID     int64
	AuthorID    int64
	PublishedAt time.Time
	HotScore    int
}

// FeedCard 保存视频卡片中相对稳定的展示字段。
type FeedCard struct {
	VideoID         int64
	AuthorID        int64
	AuthorNickname  string
	AuthorAvatarURL string
	Title           string
	Description     string
	MediaURL        string
	CoverURL        string
	PublishedAt     time.Time
}

// FeedStat 保存视频卡片中的高频计数字段。
type FeedStat struct {
	VideoID       int64
	LikeCount     int
	CommentCount  int
	FavoriteCount int
}

// ViewerActionState 保存当前用户对一批视频的互动状态。
type ViewerActionState struct {
	VideoID   int64
	Liked     bool
	Favorited bool
}

// TimelineCursor 保存时间线分页所需的排序字段。
type TimelineCursor struct {
	PublishedAt time.Time
	VideoID     int64
}

// HotCursor 保存热榜分页所需的排序字段。
type HotCursor struct {
	HotScore    int
	PublishedAt time.Time
	VideoID     int64
	WindowEnd   time.Time
	Offset      int
}

// NormalizeScene 统一 scene 参数格式，空值使用默认 Feed 场景。
func NormalizeScene(scene Scene) Scene {
	value := strings.TrimSpace(strings.ToLower(string(scene)))
	if value == "" {
		return DefaultScene
	}
	return Scene(value)
}

// RestoreFeedItem 从查询结果恢复 FeedItem，并清洗展示用字符串。
func RestoreFeedItem(videoID int64, authorID int64, authorNickname string, authorAvatarURL string, title string, description string, mediaURL string, coverURL string, likeCount int, commentCount int, favoriteCount int, publishedAt time.Time) *FeedItem {
	return &FeedItem{
		VideoID:         videoID,
		AuthorID:        authorID,
		AuthorNickname:  strings.TrimSpace(authorNickname),
		AuthorAvatarURL: strings.TrimSpace(authorAvatarURL),
		Title:           strings.TrimSpace(title),
		Description:     strings.TrimSpace(description),
		MediaURL:        strings.TrimSpace(mediaURL),
		CoverURL:        strings.TrimSpace(coverURL),
		LikeCount:       likeCount,
		CommentCount:    commentCount,
		FavoriteCount:   favoriteCount,
		HotScore:        ScoreHotFeedItem(likeCount, commentCount, favoriteCount),
		PublishedAt:     publishedAt,
	}
}

// ScoreHotFeedItem 计算热榜排序分。
func ScoreHotFeedItem(likeCount int, commentCount int, favoriteCount int) int {
	return likeCount*HotScoreLikeWeight + commentCount*HotScoreCommentWeight + favoriteCount*HotScoreFavoriteWeight
}

// HotScoreSQLExpression 生成与 ScoreHotFeedItem 口径一致的 SQL 打分表达式，
// 列名由调用方传入以适配各自的表别名。
func HotScoreSQLExpression(likeColumn string, commentColumn string, favoriteColumn string) string {
	return fmt.Sprintf(
		"COALESCE(%s, 0) * %d + COALESCE(%s, 0) * %d + COALESCE(%s, 0) * %d",
		likeColumn, HotScoreLikeWeight,
		commentColumn, HotScoreCommentWeight,
		favoriteColumn, HotScoreFavoriteWeight,
	)
}

// HasSmallAuthors 判断关注列表里是否存在走 inbox 推模式的小作者。
//
// 关注了非大 V 作者却没有任何 inbox 数据，说明索引只落了一部分，不能当作完整结果。
func HasSmallAuthors(followedAuthorIDs []int64, pullAuthorIDs []int64) bool {
	if len(followedAuthorIDs) == 0 {
		return false
	}
	pullAuthors := make(map[int64]struct{}, len(pullAuthorIDs))
	for _, authorID := range pullAuthorIDs {
		pullAuthors[authorID] = struct{}{}
	}
	for _, authorID := range followedAuthorIDs {
		if _, ok := pullAuthors[authorID]; !ok {
			return true
		}
	}
	return false
}

// SortPageItemsByTimeline 按发布时间倒序、VideoID 倒序排列，
// 与仓储分页口径保持一致，避免缓存合并结果与数据库结果顺序不同。
func SortPageItemsByTimeline(items []*FeedPageItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].VideoID > items[j].VideoID
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
