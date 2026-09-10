package test

import (
	applicationexposure "GCFeed/internal/application/exposure"
	applicationrecommendation "GCFeed/internal/application/recommendation"
	"context"
	"testing"
)

type memoryViewEventConsumer struct {
	handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error
}

func (c *memoryViewEventConsumer) ConsumeViewEventRecorded(ctx context.Context, handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error) error {
	c.handler = handler
	return nil
}

// TestViewEventWorkerRegistersConsumer 覆盖修复前的缺陷：曝光事件一直被投递，
// 但没有任何消费者，队列会无限堆积并拖垮 broker。
func TestViewEventWorkerRegistersConsumer(t *testing.T) {
	consumer := &memoryViewEventConsumer{}
	worker := applicationexposure.NewViewEventWorker(applicationrecommendation.New(newMemoryRecommendationRepo()), consumer)

	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("start view event worker: %v", err)
	}
	if consumer.handler == nil {
		t.Fatal("view event worker did not register a consumer")
	}
}

// TestViewEventWorkerAppliesPositiveEventsOnly 只有正向行为才更新兴趣向量，
// 曝光只代表"推给用户看过"，不能当成用户感兴趣。
func TestViewEventWorkerAppliesPositiveEventsOnly(t *testing.T) {
	repo := newMemoryRecommendationRepo()
	worker := applicationexposure.NewViewEventWorker(applicationrecommendation.New(repo), &memoryViewEventConsumer{})
	service := applicationrecommendation.New(repo)

	if err := service.ApplyViewEvent(context.Background(), 42, 1, "complete", 5000, true); err != nil {
		t.Fatalf("apply complete event: %v", err)
	}
	if err := service.ApplyViewEvent(context.Background(), 42, 2, "exposed", 0, false); err != nil {
		t.Fatalf("apply exposed event: %v", err)
	}

	if len(repo.appliedInterest) != 1 {
		t.Fatalf("expected 1 applied interest, got %d", len(repo.appliedInterest))
	}
	applied := repo.appliedInterest[0]
	if applied.UserID != 42 || applied.VideoID != 1 {
		t.Fatalf("unexpected applied interest: %+v", applied)
	}
	if applied.Weight != 3 {
		t.Fatalf("complete event weight = %v, want 3", applied.Weight)
	}
	if worker == nil {
		t.Fatal("worker must not be nil")
	}
}

// TestViewEventWorkerHandleEvent 验证 handler 把事件字段正确透传给画像更新。
func TestViewEventWorkerHandleEvent(t *testing.T) {
	repo := newMemoryRecommendationRepo()
	consumer := &memoryViewEventConsumer{}
	worker := applicationexposure.NewViewEventWorker(applicationrecommendation.New(repo), consumer)
	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("start view event worker: %v", err)
	}

	err := consumer.handler(context.Background(), &applicationexposure.ViewEventRecordedEvent{
		EventID:   "evt-1",
		UserID:    42,
		VideoID:   1,
		EventType: "play",
		WatchMs:   30000,
		Completed: false,
	})
	if err != nil {
		t.Fatalf("handle view event: %v", err)
	}

	if len(repo.appliedInterest) != 1 {
		t.Fatalf("expected 1 applied interest, got %d", len(repo.appliedInterest))
	}
	// play 且观看 30s：1 + 30000/30000 = 2
	if repo.appliedInterest[0].Weight != 2 {
		t.Fatalf("play event weight = %v, want 2", repo.appliedInterest[0].Weight)
	}
}
