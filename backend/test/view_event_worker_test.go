package test

import (
	applicationexposure "GCFeed/internal/application/exposure"
	applicationrecommendation "GCFeed/internal/application/recommendation"
	"context"
	"errors"
	"sync"
	"testing"
)

type memoryViewEventConsumer struct {
	handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error
}

func (c *memoryViewEventConsumer) ConsumeViewEventRecorded(ctx context.Context, handler func(context.Context, *applicationexposure.ViewEventRecordedEvent) error) error {
	c.handler = handler
	return nil
}

// memoryEventDeduplicator 是事件去重的内存实现。
type memoryEventDeduplicator struct {
	mu      sync.Mutex
	claimed map[string]bool
}

func newMemoryEventDeduplicator() *memoryEventDeduplicator {
	return &memoryEventDeduplicator{claimed: map[string]bool{}}
}

func (d *memoryEventDeduplicator) Claim(ctx context.Context, key string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if key == "" {
		return true, nil
	}
	if d.claimed[key] {
		return false, nil
	}
	d.claimed[key] = true
	return true, nil
}

func (d *memoryEventDeduplicator) Release(ctx context.Context, key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.claimed, key)
	return nil
}

// flakyInterestUpdater 前 failTimes 次调用失败，用于验证失败时会归还幂等键。
type flakyInterestUpdater struct {
	mu        sync.Mutex
	failTimes int
	calls     int
}

func (u *flakyInterestUpdater) ApplyViewEvent(ctx context.Context, userID int64, videoID int64, eventType string, watchMs int, completed bool) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	if u.calls <= u.failTimes {
		return errors.New("updater failed")
	}
	return nil
}

func (u *flakyInterestUpdater) CallCount() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.calls
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
		EventID:     "evt-1",
		ViewEventID: 1001,
		UserID:      42,
		VideoID:     1,
		EventType:   "play",
		WatchMs:     30000,
		Completed:   false,
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

// TestViewEventWorkerSkipsDuplicateEvent 同一条观看记录被重投时只累计一次。
func TestViewEventWorkerSkipsDuplicateEvent(t *testing.T) {
	repo := newMemoryRecommendationRepo()
	worker := applicationexposure.NewViewEventWorker(
		applicationrecommendation.New(repo),
		&memoryViewEventConsumer{},
		newMemoryEventDeduplicator(),
	)

	event := &applicationexposure.ViewEventRecordedEvent{
		EventID:     "evt-dup",
		ViewEventID: 9001,
		UserID:      42,
		VideoID:     1,
		EventType:   "play",
		WatchMs:     15000,
	}
	for attempt := 0; attempt < 3; attempt++ {
		if err := worker.HandleViewEventRecorded(context.Background(), event); err != nil {
			t.Fatalf("handle attempt %d: %v", attempt+1, err)
		}
	}

	if len(repo.appliedInterest) != 1 {
		t.Fatalf("duplicate event applied %d times, want 1", len(repo.appliedInterest))
	}
}

// TestViewEventWorkerDedupKeyIgnoresEventIDForSameRecord 同一条观看记录即使被
// 重复发布（EventID 不同），也应该只累计一次。
func TestViewEventWorkerDedupKeyIgnoresEventIDForSameRecord(t *testing.T) {
	repo := newMemoryRecommendationRepo()
	worker := applicationexposure.NewViewEventWorker(
		applicationrecommendation.New(repo),
		&memoryViewEventConsumer{},
		newMemoryEventDeduplicator(),
	)

	for _, eventID := range []string{"evt-a", "evt-b"} {
		event := &applicationexposure.ViewEventRecordedEvent{
			EventID:     eventID,
			ViewEventID: 9002,
			UserID:      42,
			VideoID:     1,
			EventType:   "complete",
		}
		if err := worker.HandleViewEventRecorded(context.Background(), event); err != nil {
			t.Fatalf("handle %s: %v", eventID, err)
		}
	}

	if len(repo.appliedInterest) != 1 {
		t.Fatalf("same record published twice applied %d times, want 1", len(repo.appliedInterest))
	}
}

// TestViewEventWorkerReleasesClaimOnFailure 处理失败必须归还幂等键，
// 否则消息重投会被误判为已处理，事件被静默丢弃。
func TestViewEventWorkerReleasesClaimOnFailure(t *testing.T) {
	updater := &flakyInterestUpdater{failTimes: 1}
	worker := applicationexposure.NewViewEventWorker(updater, &memoryViewEventConsumer{}, newMemoryEventDeduplicator())
	event := &applicationexposure.ViewEventRecordedEvent{
		EventID:     "evt-retry",
		ViewEventID: 9003,
		UserID:      42,
		VideoID:     1,
		EventType:   "play",
		WatchMs:     5000,
	}

	if err := worker.HandleViewEventRecorded(context.Background(), event); err == nil {
		t.Fatal("first attempt must fail")
	}
	// 重投：幂等键已归还，应重新处理并成功。
	if err := worker.HandleViewEventRecorded(context.Background(), event); err != nil {
		t.Fatalf("retry after failure: %v", err)
	}
	if count := updater.CallCount(); count != 2 {
		t.Fatalf("updater called %d times, want 2 (failed attempt + retry)", count)
	}
	// 成功后保留幂等键，再次重投应被跳过。
	if err := worker.HandleViewEventRecorded(context.Background(), event); err != nil {
		t.Fatalf("third attempt: %v", err)
	}
	if count := updater.CallCount(); count != 2 {
		t.Fatalf("duplicate after success must be skipped, updater called %d times", count)
	}
}

// TestViewEventWorkerWorksWithoutDeduplicator 未注入去重时保持原有行为。
func TestViewEventWorkerWorksWithoutDeduplicator(t *testing.T) {
	repo := newMemoryRecommendationRepo()
	worker := applicationexposure.NewViewEventWorker(applicationrecommendation.New(repo), &memoryViewEventConsumer{})
	event := &applicationexposure.ViewEventRecordedEvent{
		EventID:     "evt-nodedup",
		ViewEventID: 9004,
		UserID:      42,
		VideoID:     1,
		EventType:   "play",
		WatchMs:     5000,
	}
	for attempt := 0; attempt < 2; attempt++ {
		if err := worker.HandleViewEventRecorded(context.Background(), event); err != nil {
			t.Fatalf("handle attempt %d: %v", attempt+1, err)
		}
	}
	if len(repo.appliedInterest) != 2 {
		t.Fatalf("without deduplicator applied %d times, want 2", len(repo.appliedInterest))
	}
}
