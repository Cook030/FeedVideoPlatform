package applicationinteraction

import (
	domaininteraction "GCFeed/internal/domain/interaction"
	contract "GCFeed/internal/shared/contract"
	"context"
	"time"
)

type ActionEventConsumer interface {
	ConsumeActionChanged(ctx context.Context, handler func(context.Context, *ActionChangedEvent) error) error
}

// StatReconciler 用数据库权威计数收敛 Redis 中的点赞收藏基数与分片增量。
type StatReconciler interface {
	ReconcileActionStat(ctx context.Context, stat *domaininteraction.VideoStat) error
}

type ActionWorker struct {
	repo       domaininteraction.Repository
	consumer   ActionEventConsumer
	reconciler StatReconciler
	observer   contract.WorkerObserver
}

func NewActionWorker(repo domaininteraction.Repository, consumer ActionEventConsumer, reconcilers ...StatReconciler) *ActionWorker {
	worker := &ActionWorker{
		repo:     repo,
		consumer: consumer,
	}
	for _, reconciler := range reconcilers {
		if reconciler != nil {
			worker.reconciler = reconciler
		}
	}
	return worker
}

// WithObserver 注入后台任务观测端口；未注入时不采集指标。
func (w *ActionWorker) WithObserver(observer contract.WorkerObserver) *ActionWorker {
	if w != nil {
		w.observer = observer
	}
	return w
}

func (w *ActionWorker) observeWorkerJob(job string, duration time.Duration, err error) {
	if w == nil || w.observer == nil {
		return
	}
	w.observer.ObserveWorkerJob(job, duration, err)
}

func (w *ActionWorker) Start(ctx context.Context) error {
	if w == nil || w.consumer == nil {
		return nil
	}
	return w.consumer.ConsumeActionChanged(ctx, w.HandleActionChanged)
}

func (w *ActionWorker) HandleActionChanged(ctx context.Context, event *ActionChangedEvent) error {
	start := time.Now()
	var err error
	defer func() {
		w.observeWorkerJob("interaction_action_changed", time.Since(start), err)
	}()

	if event == nil {
		return nil
	}
	_, _, _, err = w.repo.SetAction(ctx, event.UserID, event.VideoID, event.ActionType, event.Active, event.IdempotencyKey)
	if err != nil {
		return err
	}
	w.reconcileStat(ctx, event.VideoID)
	return nil
}

// reconcileStat 用落库后的权威计数重置 Redis，避免基数与分片增量长期漂移。
func (w *ActionWorker) reconcileStat(ctx context.Context, videoID int64) {
	if w.reconciler == nil || videoID <= 0 {
		return
	}
	stat, err := w.repo.GetVideoStat(ctx, videoID)
	if err != nil {
		return
	}
	_ = w.reconciler.ReconcileActionStat(ctx, stat)
}
