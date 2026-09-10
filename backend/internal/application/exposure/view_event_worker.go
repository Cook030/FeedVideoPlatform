package applicationexposure

import (
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"fmt"
	"time"
)

// ViewEventConsumer 消费观看行为事件。
type ViewEventConsumer interface {
	ConsumeViewEventRecorded(ctx context.Context, handler func(context.Context, *ViewEventRecordedEvent) error) error
}

// InterestProfileUpdater 用观看行为增量更新用户兴趣画像。
type InterestProfileUpdater interface {
	ApplyViewEvent(ctx context.Context, userID int64, videoID int64, eventType string, watchMs int, completed bool) error
}

// EventDeduplicator 按幂等键去重，防止消息重投导致副作用重复执行。
type EventDeduplicator interface {
	// Claim 占用幂等键，返回 false 表示事件已处理过，调用方应跳过。
	Claim(ctx context.Context, key string) (bool, error)
	// Release 在处理失败时归还幂等键，让消息重投后可以重新处理。
	Release(ctx context.Context, key string) error
}

// ViewEventWorker 消费观看行为事件并维护推荐画像。
//
// 曝光服务一直在投递 view.event.recorded，但此前没有任何消费者，
// 队列只进不出会持续堆积并最终拖垮整个 broker。
//
// 兴趣向量是"读-改-写"的幂等累加，消息重投会重复累加权重，因此接入
// EventDeduplicator 做事件级去重；未注入时保持原有的直接处理行为。
type ViewEventWorker struct {
	updater  InterestProfileUpdater
	consumer ViewEventConsumer
	dedup    EventDeduplicator
}

// NewViewEventWorker 创建观看行为事件 worker，可选注入事件去重能力。
func NewViewEventWorker(updater InterestProfileUpdater, consumer ViewEventConsumer, deduplicators ...EventDeduplicator) *ViewEventWorker {
	worker := &ViewEventWorker{updater: updater, consumer: consumer}
	for _, deduplicator := range deduplicators {
		if deduplicator != nil {
			worker.dedup = deduplicator
		}
	}
	return worker
}

func (w *ViewEventWorker) Start(ctx context.Context) error {
	if w == nil || w.updater == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeViewEventRecorded(ctx, w.HandleViewEventRecorded)
}

func (w *ViewEventWorker) HandleViewEventRecorded(ctx context.Context, event *ViewEventRecordedEvent) (err error) {
	start := time.Now()
	defer func() {
		inframetrics.ObserveWorkerJob("view_event_recorded", time.Since(start), err)
	}()

	if event == nil {
		return nil
	}

	dedupKey := viewEventDedupKey(event)
	if w.dedup != nil {
		claimed, claimErr := w.dedup.Claim(ctx, dedupKey)
		if claimErr != nil {
			// 去重不可用时降级为继续处理：重复累加权重的影响小于丢事件。
			inframetrics.ObserveWorkerJob("view_event_recorded_dedup_error", time.Since(start), claimErr)
		} else if !claimed {
			inframetrics.ObserveWorkerJob("view_event_recorded_duplicate", time.Since(start), nil)
			return nil
		}
	}

	if err := w.updater.ApplyViewEvent(ctx, event.UserID, event.VideoID, event.EventType, event.WatchMs, event.Completed); err != nil {
		if w.dedup != nil {
			// 归还幂等键，否则消息重投会被误判为已处理而丢掉事件。
			_ = w.dedup.Release(ctx, dedupKey)
		}
		return err
	}
	return nil
}

// viewEventDedupKey 优先用落库记录 ID 做幂等键：同一条观看记录即使被重复发布
// （EventID 不同）也只会累计一次；缺少记录 ID 时退回事件 ID。
func viewEventDedupKey(event *ViewEventRecordedEvent) string {
	if event.ViewEventID > 0 {
		return fmt.Sprintf("rec:consumed_view_event:v1:%d", event.ViewEventID)
	}
	if event.EventID != "" {
		return "rec:consumed_view_event:v1:e:" + event.EventID
	}
	return ""
}
