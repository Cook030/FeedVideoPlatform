package applicationinteraction

import (
	contract "GCFeed/internal/shared/contract"
	"strings"
	"time"
)

// ActionChangedEvent 是点赞/收藏变更事件契约，定义在 shared/contract。
// 保留类型别名，使既有调用方与测试无需改动。
type ActionChangedEvent = contract.ActionChangedEvent

// NewActionChangedEvent 构造点赞收藏变更事件。
func NewActionChangedEvent(userID int64, videoID int64, actionType string, active bool, idempotencyKey string) *ActionChangedEvent {
	return &ActionChangedEvent{
		EventID:        contract.NewEventID(),
		UserID:         userID,
		VideoID:        videoID,
		ActionType:     actionType,
		Active:         active,
		IdempotencyKey: strings.TrimSpace(idempotencyKey),
		OccurredAt:     time.Now().UTC(),
	}
}
