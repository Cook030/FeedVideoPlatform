// Package contract 承载跨限界上下文的纯数据契约。
//
// 这些类型被 application 子包与 infra 适配器共同引用。放在中立包是为了
// 避免 infra 依赖具体业务子包，也避免 application 子包互相 import。
// contract 只依赖 domain，不依赖 application/infra/interfaces。
//
// 注意：本包中的 JSON tag 属于对外契约（MQ 消息格式），改动必须走契约比对。
package contract

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// ActionChangedEvent 描述一次点赞或收藏状态变更，由互动写入、异步落库消费。
type ActionChangedEvent struct {
	EventID        string    `json:"event_id"`
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	ActionType     string    `json:"action_type"`
	Active         bool      `json:"active"`
	IdempotencyKey string    `json:"idempotency_key"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// PublishedEvent 描述一次视频发布事件，供关注流扇出与向量化任务消费。
type PublishedEvent struct {
	EventID     string    `json:"event_id"`
	VideoID     int64     `json:"video_id"`
	AuthorID    int64     `json:"author_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	MediaURL    string    `json:"media_url"`
	CoverURL    string    `json:"cover_url"`
	PublishedAt time.Time `json:"published_at"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// ViewEventRecordedEvent 描述观看行为已落库事件，供推荐画像 worker 消费。
type ViewEventRecordedEvent struct {
	EventID       string    `json:"event_id"`
	ViewEventID   int64     `json:"view_event_id"`
	UserID        int64     `json:"user_id"`
	VideoID       int64     `json:"video_id"`
	Scene         string    `json:"scene"`
	RequestID     string    `json:"request_id,omitempty"`
	EventType     string    `json:"event_type"`
	WatchMs       int       `json:"watch_ms"`
	Completed     bool      `json:"completed"`
	RecordedAt    time.Time `json:"recorded_at"`
	OccurredAt    time.Time `json:"occurred_at"`
	ExposureCount int       `json:"exposure_count,omitempty"`
}

// NewEventID 生成事件唯一 ID，随机源不可用时退化为时间戳。
func NewEventID() string {
	content := make([]byte, 12)
	if _, err := rand.Read(content); err == nil {
		return hex.EncodeToString(content)
	}
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
