package applicationexposure

import (
	domainexposure "GCFeed/internal/domain/exposure"
	contract "GCFeed/internal/shared/contract"
	"time"
)

// ViewEventRecordedEvent 是观看行为已落库事件契约，定义在 shared/contract。
// 保留类型别名，使既有调用方与测试无需改动。
type ViewEventRecordedEvent = contract.ViewEventRecordedEvent

func NewViewEventRecordedEvent(event *domainexposure.ViewEvent, exposure *domainexposure.Exposure) *ViewEventRecordedEvent {
	if event == nil {
		return nil
	}
	message := &ViewEventRecordedEvent{
		EventID:     contract.NewEventID(),
		ViewEventID: event.ID,
		UserID:      event.UserID,
		VideoID:     event.VideoID,
		Scene:       event.Scene,
		RequestID:   event.RequestID,
		EventType:   event.EventType,
		WatchMs:     event.WatchMs,
		Completed:   event.Completed,
		RecordedAt:  event.CreatedAt.UTC(),
		OccurredAt:  time.Now().UTC(),
	}
	if exposure != nil {
		message.ExposureCount = exposure.ExposureCount
	}
	return message
}

