package applicationembedding

import (
	contract "GCFeed/internal/shared/contract"
	"context"
	"time"
)

// PublishedEventConsumer 消费视频发布事件。
type PublishedEventConsumer interface {
	ConsumeVideoPublishedForEmbedding(ctx context.Context, handler func(context.Context, *contract.PublishedEvent) error) error
}

type VideoEmbeddingWorker struct {
	service  *Service
	consumer PublishedEventConsumer
	observer contract.WorkerObserver
}

func NewVideoEmbeddingWorker(service *Service, consumer PublishedEventConsumer) *VideoEmbeddingWorker {
	return &VideoEmbeddingWorker{
		service:  service,
		consumer: consumer,
	}
}

// WithObserver 注入后台任务观测端口；未注入时不采集指标。
func (w *VideoEmbeddingWorker) WithObserver(observer contract.WorkerObserver) *VideoEmbeddingWorker {
	if w != nil {
		w.observer = observer
	}
	return w
}

func (w *VideoEmbeddingWorker) observeWorkerJob(job string, duration time.Duration, err error) {
	if w == nil || w.observer == nil {
		return
	}
	w.observer.ObserveWorkerJob(job, duration, err)
}

func (w *VideoEmbeddingWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeVideoPublishedForEmbedding(ctx, w.HandleVideoPublished)
}

func (w *VideoEmbeddingWorker) HandleVideoPublished(ctx context.Context, event *contract.PublishedEvent) (err error) {
	start := time.Now()
	defer func() {
		w.observeWorkerJob("video_embedding", time.Since(start), err)
	}()

	if w == nil || w.service == nil || event == nil {
		return nil
	}
	_, err = w.service.GenerateForPublishedVideo(ctx, event)
	return err
}
