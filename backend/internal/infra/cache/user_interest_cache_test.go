package infracache

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/redis/go-redis/v9"
)

type userInterestFakeRedis struct {
	values  map[string]string
	setArgs []redis.SetArgs
}

func newUserInterestFakeRedis() *userInterestFakeRedis {
	return &userInterestFakeRedis{values: map[string]string{}}
}

func (r *userInterestFakeRedis) Get(ctx context.Context, key string) *redis.StringCmd {
	value, ok := r.values[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(value, nil)
}

func (r *userInterestFakeRedis) SetArgs(ctx context.Context, key string, value interface{}, a redis.SetArgs) *redis.StatusCmd {
	// 模拟 Redis 的 XX 语义：key 不存在时不写入。
	if a.Mode == "XX" {
		if _, ok := r.values[key]; !ok {
			return redis.NewStatusResult("", nil)
		}
	}
	switch typed := value.(type) {
	case string:
		r.values[key] = typed
	case []byte:
		r.values[key] = string(typed)
	default:
		content, _ := json.Marshal(typed)
		r.values[key] = string(content)
	}
	r.setArgs = append(r.setArgs, a)
	return redis.NewStatusResult("OK", nil)
}

func (r *userInterestFakeRedis) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	removed := int64(0)
	for _, key := range keys {
		if _, ok := r.values[key]; ok {
			delete(r.values, key)
			removed++
		}
	}
	return redis.NewIntResult(removed, nil)
}

func TestUserInterestCacheStoreAndLoad(t *testing.T) {
	ctx := context.Background()
	client := newUserInterestFakeRedis()
	cache := NewUserInterestCache(client)

	if err := cache.Store(ctx, 42, []float64{1, 0}, 2); err != nil {
		t.Fatalf("store: %v", err)
	}

	vector, ok, err := cache.Load(ctx, 42)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !ok {
		t.Fatal("expected cache hit after store")
	}
	assertVector(t, vector, []float64{1, 0})

	// 回填基线必须无视 key 是否存在，否则首次写入会被 XX 挡掉。
	if len(client.setArgs) != 1 || client.setArgs[0].Mode != "" {
		t.Fatalf("store must write unconditionally, got %+v", client.setArgs)
	}
	if client.setArgs[0].TTL != userInterestTTL {
		t.Fatalf("store TTL = %v, want %v", client.setArgs[0].TTL, userInterestTTL)
	}
}

// TestUserInterestCacheApplyRequiresBaseline 覆盖关键语义：缓存不存在时不能写入增量，
// 否则快照里只剩最近一条行为，与"最近 30 天加权平均"的回源语义不符。
func TestUserInterestCacheApplyRequiresBaseline(t *testing.T) {
	ctx := context.Background()
	client := newUserInterestFakeRedis()
	cache := NewUserInterestCache(client)

	if err := cache.Apply(ctx, 42, []float64{0, 1}, 1); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(client.values) != 0 {
		t.Fatalf("apply without baseline must not write, got %+v", client.values)
	}

	_, ok, err := cache.Load(ctx, 42)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ok {
		t.Fatal("cache must stay empty without baseline")
	}
}

func TestUserInterestCacheApplyAddsWeightedVector(t *testing.T) {
	ctx := context.Background()
	client := newUserInterestFakeRedis()
	cache := NewUserInterestCache(client)

	if err := cache.Store(ctx, 42, []float64{1, 0}, 2); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := cache.Apply(ctx, 42, []float64{0, 1}, 1); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// 基线 [1,0]*2 + 增量 [0,1]*1 => [2,1]/3
	vector, ok, err := cache.Load(ctx, 42)
	if err != nil || !ok {
		t.Fatalf("load after apply: ok=%v err=%v", ok, err)
	}
	assertVector(t, vector, []float64{2.0 / 3.0, 1.0 / 3.0})

	last := client.setArgs[len(client.setArgs)-1]
	if last.Mode != "XX" {
		t.Fatalf("apply must use XX mode to avoid overwriting an expired baseline, got %q", last.Mode)
	}
}

// TestUserInterestCacheApplySkipsDimensionMismatch 向量模型升级换维度时放弃增量，
// 等 TTL 或下次回源重建，避免把不同维度的向量拼在一起。
func TestUserInterestCacheApplySkipsDimensionMismatch(t *testing.T) {
	ctx := context.Background()
	client := newUserInterestFakeRedis()
	cache := NewUserInterestCache(client)

	if err := cache.Store(ctx, 42, []float64{1, 0}, 1); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := cache.Apply(ctx, 42, []float64{0, 1, 0}, 1); err != nil {
		t.Fatalf("apply: %v", err)
	}

	vector, ok, err := cache.Load(ctx, 42)
	if err != nil || !ok {
		t.Fatalf("load: ok=%v err=%v", ok, err)
	}
	assertVector(t, vector, []float64{1, 0})
}

func TestUserInterestCacheInvalidate(t *testing.T) {
	ctx := context.Background()
	client := newUserInterestFakeRedis()
	cache := NewUserInterestCache(client)

	if err := cache.Store(ctx, 42, []float64{1, 0}, 1); err != nil {
		t.Fatalf("store: %v", err)
	}
	if err := cache.Invalidate(ctx, 42); err != nil {
		t.Fatalf("invalidate: %v", err)
	}

	_, ok, err := cache.Load(ctx, 42)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if ok {
		t.Fatal("expected cache miss after invalidate")
	}
}

func TestUserInterestCacheIgnoresInvalidInput(t *testing.T) {
	ctx := context.Background()
	client := newUserInterestFakeRedis()
	cache := NewUserInterestCache(client)

	cases := []struct {
		name string
		run  func() error
	}{
		{"store empty vector", func() error { return cache.Store(ctx, 42, nil, 1) }},
		{"store zero weight", func() error { return cache.Store(ctx, 42, []float64{1}, 0) }},
		{"apply empty vector", func() error { return cache.Apply(ctx, 42, nil, 1) }},
		{"apply zero weight", func() error { return cache.Apply(ctx, 42, []float64{1}, 0) }},
		{"invalid user", func() error { return cache.Store(ctx, 0, []float64{1}, 1) }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			if err := item.run(); err != nil {
				t.Fatalf("must be a no-op, got %v", err)
			}
		})
	}
	if len(client.values) != 0 {
		t.Fatalf("invalid input must not write, got %+v", client.values)
	}

	// nil 缓存与 nil client 都必须安全降级，让推荐链路回源。
	var nilCache *UserInterestCache
	if _, ok, err := nilCache.Load(ctx, 42); ok || err != nil {
		t.Fatalf("nil cache load = ok:%v err:%v", ok, err)
	}
	if err := NewUserInterestCache(nil).Store(ctx, 42, []float64{1}, 1); err != nil {
		t.Fatalf("nil client store: %v", err)
	}
}

// TestUserInterestSnapshotRejectsBrokenState 校验快照自检：权重、维度与长度不一致时视为未命中。
func TestUserInterestSnapshotRejectsBrokenState(t *testing.T) {
	cases := []struct {
		name     string
		snapshot userInterestSnapshot
		wantNil  bool
	}{
		{"valid", userInterestSnapshot{Sum: []float64{2, 1}, Weight: 3, Dimension: 2}, false},
		{"zero weight", userInterestSnapshot{Sum: []float64{2, 1}, Weight: 0, Dimension: 2}, true},
		{"empty sum", userInterestSnapshot{Sum: nil, Weight: 1, Dimension: 0}, true},
		{"dimension mismatch", userInterestSnapshot{Sum: []float64{2, 1}, Weight: 3, Dimension: 3}, true},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			vector := item.snapshot.vector()
			if item.wantNil && vector != nil {
				t.Fatalf("expected nil vector, got %v", vector)
			}
			if !item.wantNil && len(vector) != len(item.snapshot.Sum) {
				t.Fatalf("unexpected vector %v", vector)
			}
		})
	}
}

func assertVector(t *testing.T, got []float64, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("vector length = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("vector[%d] = %v, want %v (got %v)", i, got[i], want[i], got)
		}
	}
}
