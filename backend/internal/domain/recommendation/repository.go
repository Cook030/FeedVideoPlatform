package domainrecommendation

import (
	"context"
	"time"
)

type Repository interface {
	ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*Candidate, error)
	LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error)
	LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error)
	ListRecentExposures(ctx context.Context, userID int64, videoIDs []int64, since time.Time) ([]*Exposure, error)
	SaveExposures(ctx context.Context, exposures []*ExposureWrite) ([]*Exposure, error)
	// ApplyUserInterest 用单个视频的向量按权重增量修正用户兴趣向量。
	// 没有可用缓存时应当安全返回 nil，由回源聚合继续提供兴趣向量。
	ApplyUserInterest(ctx context.Context, userID int64, videoID int64, weight float64) error
}
