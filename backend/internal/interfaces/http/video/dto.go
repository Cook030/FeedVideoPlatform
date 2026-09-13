package interfaceshttpvideo

import (
	"time"

	domainvideo "GCFeed/internal/domain/video"
)

// CreateVideoRequest 是发布视频的 JSON 请求体。
type CreateVideoRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	MediaURL    string `json:"media_url"`
	CoverURL    string `json:"cover_url"`
}

// videoResponse 是视频详情响应，包含视频主体字段和互动计数。
type videoResponse struct {
	ID            int64      `json:"id"`
	AuthorID      int64      `json:"author_id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	MediaURL      string     `json:"media_url"`
	CoverURL      string     `json:"cover_url"`
	Status        int        `json:"status"`
	LikeCount     int        `json:"like_count"`
	CommentCount  int        `json:"comment_count"`
	FavoriteCount int        `json:"favorite_count"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// videoListResponse 是 offset 分页列表响应。
type videoListResponse struct {
	Items  []videoResponse `json:"items"`
	Limit  int             `json:"limit"`
	Offset int             `json:"offset"`
}

// videoResponseFromDomain 把领域视频转换成 HTTP JSON 响应。
func videoResponseFromDomain(video *domainvideo.Video) videoResponse {
	return videoResponse{
		ID:            video.ID,
		AuthorID:      video.AuthorID,
		Title:         video.Title,
		Description:   video.Description,
		MediaURL:      video.MediaURL,
		CoverURL:      video.CoverURL,
		Status:        video.Status,
		LikeCount:     video.LikeCount,
		CommentCount:  video.CommentCount,
		FavoriteCount: video.FavoriteCount,
		PublishedAt:   video.PublishedAt,
		CreatedAt:     video.CreatedAt,
		UpdatedAt:     video.UpdatedAt,
	}
}

// videoListResponseFromDomain 组装列表响应，并回显本次分页参数。
func videoListResponseFromDomain(videos []*domainvideo.Video, limit, offset int) videoListResponse {
	items := make([]videoResponse, 0, len(videos))
	for _, video := range videos {
		items = append(items, videoResponseFromDomain(video))
	}
	return videoListResponse{
		Items:  items,
		Limit:  limit,
		Offset: offset,
	}
}
