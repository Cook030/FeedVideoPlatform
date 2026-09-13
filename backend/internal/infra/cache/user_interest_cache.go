package infracache

import (
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const userInterestTTL = 30 * time.Minute
const userInterestKeyPrefix = "rec:user_interest:v2:"

type redisUserInterestClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	SetArgs(ctx context.Context, key string, value interface{}, a redis.SetArgs) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// userInterestSnapshot 保存兴趣向量的加权和与权重，而不是最终向量。
// 这样增量修正和全量回源聚合可以共用同一套加权平均语义。
type userInterestSnapshot struct {
	Sum       []float64 `json:"sum"`
	Weight    float64   `json:"weight"`
	Dimension int       `json:"dimension"`
}

// UserInterestCache 缓存推荐链路使用的用户兴趣向量。
//
// 回源聚合需要扫描最近若干条行为流水并逐条解析向量，放在推荐请求路径上开销明显。
// 这里采用 cache-aside：未命中时回源聚合并回填，之后由观看行为事件做增量修正，
// TTL 到期后自然重建，避免增量长期漂移。缓存只影响性能，缺失时行为与回源一致。
type UserInterestCache struct {
	client    redisUserInterestClient
	model     string
	dimension int
}

// NewUserInterestCache 创建用户兴趣向量缓存。
func NewUserInterestCache(client redisUserInterestClient, model string, dimension int) *UserInterestCache {
	return &UserInterestCache{client: client, model: model, dimension: dimension}
}

// Load 读取用户兴趣向量，第二个返回值表示是否命中缓存。
func (c *UserInterestCache) Load(ctx context.Context, userID int64) ([]float64, bool, error) {
	if c == nil || c.client == nil || userID <= 0 {
		return nil, false, nil
	}
	key := c.userInterestKey(userID)
	content, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		inframetrics.ObserveCacheRead("user_interest", 1, 0, nil)
		return nil, false, nil
	}
	if err != nil {
		inframetrics.ObserveCacheRead("user_interest", 1, 0, err)
		return nil, false, err
	}

	var snapshot userInterestSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		_ = c.client.Del(ctx, key).Err()
		inframetrics.ObserveCacheRead("user_interest", 1, 0, err)
		return nil, false, nil
	}
	vector := snapshot.vector(c.dimension)
	if len(vector) == 0 {
		_ = c.client.Del(ctx, key).Err()
		inframetrics.ObserveCacheRead("user_interest", 1, 0, nil)
		return nil, false, nil
	}
	inframetrics.ObserveCacheRead("user_interest", 1, 1, nil)
	return vector, true, nil
}

// Store 写入回源聚合得到的全量基线，vector 为归一化向量，weight 为其累计权重。
func (c *UserInterestCache) Store(ctx context.Context, userID int64, vector []float64, weight float64) error {
	if c == nil || c.client == nil || userID <= 0 || len(vector) != c.dimension || weight <= 0 {
		return nil
	}
	sum := make([]float64, len(vector))
	for i, value := range vector {
		sum[i] = value * weight
	}
	return c.write(ctx, userID, userInterestSnapshot{Sum: sum, Weight: weight, Dimension: len(vector)}, false)
}

// Apply 用单条观看行为对兴趣向量做增量修正。
//
// 缓存不存在时直接跳过：此时写入会让缓存只反映最近一两条行为，
// 与"最近 30 天加权平均"的回源语义不一致，交给下次回源重建更准确。
func (c *UserInterestCache) Apply(ctx context.Context, userID int64, videoVector []float64, weight float64) error {
	if c == nil || c.client == nil || userID <= 0 || len(videoVector) == 0 || weight <= 0 {
		return nil
	}
	if len(videoVector) != c.dimension {
		return c.Invalidate(ctx, userID)
	}
	key := c.userInterestKey(userID)
	content, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}

	var snapshot userInterestSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		_ = c.client.Del(ctx, key).Err()
		return nil
	}
	// 快照不属于当前维度时立即作废，下次推荐请求会回源重建。
	if snapshot.Dimension != c.dimension || snapshot.Dimension != len(videoVector) || snapshot.Weight <= 0 || len(snapshot.Sum) != c.dimension {
		return c.Invalidate(ctx, userID)
	}
	for i := range videoVector {
		snapshot.Sum[i] += videoVector[i] * weight
	}
	snapshot.Weight += weight
	// 用 XX 写入：若上面读到快照后 key 刚好过期，这次增量就没有基线可叠加，
	// 直接放弃写入，交给下次回源重建，避免缓存里只剩最近一条行为的错误结果。
	return c.write(ctx, userID, snapshot, true)
}

// Invalidate 删除用户兴趣向量，下次访问会重新回源聚合。
func (c *UserInterestCache) Invalidate(ctx context.Context, userID int64) error {
	if c == nil || c.client == nil || userID <= 0 {
		return nil
	}
	return c.client.Del(ctx, c.userInterestKey(userID)).Err()
}

func (c *UserInterestCache) write(ctx context.Context, userID int64, snapshot userInterestSnapshot, onlyIfExists bool) error {
	content, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	args := redis.SetArgs{TTL: userInterestTTL}
	if onlyIfExists {
		args.Mode = "XX"
	}
	return c.client.SetArgs(ctx, c.userInterestKey(userID), content, args).Err()
}

func (s userInterestSnapshot) vector(expectedDimension int) []float64 {
	if s.Weight <= 0 || expectedDimension <= 0 || s.Dimension != expectedDimension || len(s.Sum) != expectedDimension {
		return nil
	}
	vector := make([]float64, len(s.Sum))
	for i, value := range s.Sum {
		vector[i] = value / s.Weight
	}
	return vector
}

func (c *UserInterestCache) userInterestKey(userID int64) string {
	return fmt.Sprintf("%s%s:%d:%d", userInterestKeyPrefix, c.model, c.dimension, userID)
}
