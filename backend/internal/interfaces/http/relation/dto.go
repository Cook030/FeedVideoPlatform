package interfaceshttprelation

import (
	"time"

	applicationrelation "GCFeed/internal/application/relation"
)

// followResponse 是关注或取关后的关系状态响应。
type followResponse struct {
	UserID         int64 `json:"user_id"`
	TargetUserID   int64 `json:"target_user_id"`
	Status         int   `json:"status"`
	Following      bool  `json:"following"`
	FollowingCount int   `json:"following_count"`
	FollowerCount  int   `json:"follower_count"`
}

// relationUserResponse 是关注列表和粉丝列表中的用户项。
type relationUserResponse struct {
	UserID     int64     `json:"user_id"`
	Nickname   string    `json:"nickname"`
	AvatarURL  string    `json:"avatar_url"`
	Bio        string    `json:"bio"`
	FollowedAt time.Time `json:"followed_at"`
}

// relationListResponse 是关系列表游标分页响应。
type relationListResponse struct {
	Items      []relationUserResponse `json:"items"`
	NextCursor string                 `json:"next_cursor"`
	HasMore    bool                   `json:"has_more"`
}

// followResponseFromResult 把应用层关注结果转换为 HTTP 响应。
func followResponseFromResult(result *applicationrelation.FollowResult) followResponse {
	return followResponse{
		UserID:         result.UserID,
		TargetUserID:   result.TargetUserID,
		Status:         result.Status,
		Following:      result.Following,
		FollowingCount: result.FollowingCount,
		FollowerCount:  result.FollowerCount,
	}
}

// relationListResponseFromResult 把应用层关系列表转换为 HTTP 响应。
func relationListResponseFromResult(result *applicationrelation.ListResult) relationListResponse {
	items := make([]relationUserResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, relationUserResponse{
			UserID:     item.UserID,
			Nickname:   item.Nickname,
			AvatarURL:  item.AvatarURL,
			Bio:        item.Bio,
			FollowedAt: item.FollowedAt,
		})
	}
	return relationListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}
