package applicationvideo

import (
	domainvideo "GCFeed/internal/domain/video"
	contract "GCFeed/internal/shared/contract"
	"strings"
	"time"
)

// PublishedEvent 是视频发布事件契约，定义在 shared/contract。
// 保留类型别名，使既有调用方与测试无需改动。
type PublishedEvent = contract.PublishedEvent

func NewPublishedEvent(video *domainvideo.Video) *PublishedEvent {
	if video == nil || video.PublishedAt == nil {
		return nil
	}
	return &PublishedEvent{
		EventID:     contract.NewEventID(),
		VideoID:     video.ID,
		AuthorID:    video.AuthorID,
		Title:       strings.TrimSpace(video.Title),
		Description: strings.TrimSpace(video.Description),
		Tags:        append([]string(nil), video.Tags...),
		MediaURL:    strings.TrimSpace(video.MediaURL),
		CoverURL:    strings.TrimSpace(video.CoverURL),
		PublishedAt: video.PublishedAt.UTC(),
		OccurredAt:  time.Now().UTC(),
	}
}
