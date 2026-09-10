package applicationexposure

import (
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
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

// ViewEventWorker 消费观看行为事件并维护推荐画像。
//
// 曝光服务一直在投递 view.event.recorded，但此前没有任何消费者，
// 队列只进不出会持续堆积并最终拖垮整个 broker。
type ViewEventWorker struct {
	updater  InterestProfileUpdater
	consumer ViewEventConsumer
}

// NewViewEventWorker 创建观看行为事件 worker。
func NewViewEventWorker(updater InterestProfileUpdater, consumer ViewEventConsumer) *ViewEventWorker {
	return &ViewEventWorker{updater: updater, consumer: consumer}
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
	return w.updater.ApplyViewEvent(ctx, event.UserID, event.VideoID, event.EventType, event.WatchMs, event.Completed)
}
