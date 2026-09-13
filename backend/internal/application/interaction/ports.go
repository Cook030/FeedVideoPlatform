package applicationinteraction

import (
	"context"
	"time"

	domaininteraction "GCFeed/internal/domain/interaction"
	contract "GCFeed/internal/shared/contract"
)

// ActionStateResult 是点赞/收藏快速写路径的返回结果，契约定义在 shared/contract。
// 保留类型别名，使缓存实现与既有调用方无需改动。
type ActionStateResult = contract.ActionStateResult

// HotScoreRecorder 把互动变化投递到热榜分钟桶。
type HotScoreRecorder interface {
	AddHotScore(ctx context.Context, videoID int64, scoreDelta int, at time.Time) error
}

// StatCache 同步 Feed 展示所需的视频互动计数缓存。
type StatCache interface {
	SetVideoStat(ctx context.Context, stat *domaininteraction.VideoStat) error
}

// ActionStateStore 保存点赞收藏的快速状态和计数。
type ActionStateStore interface {
	SetActionState(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string, initialStat *domaininteraction.VideoStat) (*ActionStateResult, error)
}

// ActionEventPublisher 投递点赞收藏变更事件。
type ActionEventPublisher interface {
	PublishActionChanged(ctx context.Context, event *ActionChangedEvent) error
}

// MessageWriter 写入互动触发的站内消息。
type MessageWriter interface {
	CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error)
}

// ActorMessageWriter 可在消息里携带触发互动的用户资料。
type ActorMessageWriter interface {
	CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error)
}
