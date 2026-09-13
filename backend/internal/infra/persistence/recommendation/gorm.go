package infrarecommendation

import (
	domainembedding "GCFeed/internal/domain/embedding"
	domainexposure "GCFeed/internal/domain/exposure"
	domainfeed "GCFeed/internal/domain/feed"
	domainrecommendation "GCFeed/internal/domain/recommendation"
	domainvideo "GCFeed/internal/domain/video"
	infraexposure "GCFeed/internal/infra/persistence/exposure"
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// hotScoreExpression 与 Feed 仓储共用 domain 的权威口径。
var hotScoreExpression = domainfeed.HotScoreSQLExpression("vs.like_count", "vs.comment_count", "vs.favorite_count")

// UserInterestCache 缓存用户兴趣向量，未命中时由本仓储回源聚合后回填。
type UserInterestCache interface {
	Load(ctx context.Context, userID int64) ([]float64, bool, error)
	Store(ctx context.Context, userID int64, vector []float64, weight float64) error
	Apply(ctx context.Context, userID int64, videoVector []float64, weight float64) error
}

type Repository struct {
	db                 *gorm.DB
	interestCache      UserInterestCache
	embeddingModel     string
	embeddingDimension int
}

type Option func(*Repository)

// WithUserInterestCache 启用用户兴趣向量缓存，未设置时每次推荐都回源聚合。
func WithUserInterestCache(cache UserInterestCache) Option {
	return func(r *Repository) {
		r.interestCache = cache
	}
}

// WithEmbeddingSpec 指定推荐链路唯一可读取的视频向量版本与维度。
func WithEmbeddingSpec(model string, dimension int) Option {
	return func(r *Repository) {
		if model != "" && dimension > 0 {
			r.embeddingModel = model
			r.embeddingDimension = dimension
		}
	}
}

type candidateModel struct {
	VideoID     int64
	AuthorID    int64
	HotScore    int
	PublishedAt time.Time
}

type videoVectorModel struct {
	VideoID       int64
	Dimension     int
	EmbeddingJSON string
}

func New(db *gorm.DB, options ...Option) *Repository {
	repository := &Repository{
		db:                 db,
		embeddingModel:     domainembedding.HashNgramModel,
		embeddingDimension: domainembedding.HashNgramDimension,
	}
	for _, option := range options {
		if option != nil {
			option(repository)
		}
	}
	return repository
}

func (r *Repository) ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*domainrecommendation.Candidate, error) {
	if limit <= 0 {
		return []*domainrecommendation.Candidate{}, nil
	}

	var models []candidateModel
	err := r.db.WithContext(ctx).
		Table("video AS v").
		Select("v.id AS video_id, v.author_id, ("+hotScoreExpression+") AS hot_score, v.published_at").
		Joins("LEFT JOIN video_stat AS vs ON vs.video_id = v.id").
		Joins(
			"LEFT JOIN exposures AS e ON e.user_id = ? AND e.video_id = v.id AND e.last_exposed_at >= ?",
			userID,
			time.Now().Add(-domainrecommendation.RecentExposureWindow),
		).
		Where("v.status = ? AND v.published_at IS NOT NULL AND e.video_id IS NULL", domainvideo.StatusPublished).
		Order("hot_score DESC").
		Order("v.published_at DESC").
		Order("v.id DESC").
		Limit(limit).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}

	candidates := make([]*domainrecommendation.Candidate, 0, len(models))
	for _, model := range models {
		candidates = append(candidates, domainrecommendation.RestoreCandidate(
			model.VideoID,
			model.AuthorID,
			0,
			0,
			model.HotScore,
			0,
			"",
			model.PublishedAt,
		))
	}
	return candidates, nil
}

// LoadUserInterestVector 读取用户兴趣向量，优先命中缓存，未命中时回源聚合。
func (r *Repository) LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error) {
	if r.interestCache != nil {
		if vector, ok, err := r.interestCache.Load(ctx, userID); err == nil && ok {
			return vector, true, nil
		}
	}

	vector, totalWeight, err := r.aggregateUserInterestVector(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if len(vector) == 0 || totalWeight == 0 {
		return nil, false, nil
	}
	if r.interestCache != nil {
		_ = r.interestCache.Store(ctx, userID, vector, totalWeight)
	}
	return vector, true, nil
}

// aggregateUserInterestVector 扫描最近的正向观看行为，返回归一化向量和累计权重。
func (r *Repository) aggregateUserInterestVector(ctx context.Context, userID int64) ([]float64, float64, error) {
	rows, err := r.db.WithContext(ctx).
		Table("video_view_events AS ev").
		Select("ve.dimension, ve.embedding_json, ev.event_type, ev.watch_ms, ev.completed").
		Joins("JOIN video_embedding AS ve ON ve.video_id = ev.video_id AND ve.model = ? AND ve.dimension = ?", r.embeddingModel, r.embeddingDimension).
		Where("ev.user_id = ? AND ev.created_at >= ? AND ev.event_type IN ?", userID, time.Now().Add(-domainrecommendation.PositiveEventWindow), positiveEventTypes()).
		Order("ev.created_at DESC").
		Limit(200).
		Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var sum []float64
	var totalWeight float64
	for rows.Next() {
		var dimension int
		var embeddingJSON string
		var eventType string
		var watchMs int
		var completed bool
		if err := rows.Scan(&dimension, &embeddingJSON, &eventType, &watchMs, &completed); err != nil {
			return nil, 0, err
		}
		vector, err := decodeVector(embeddingJSON)
		if err != nil || dimension != r.embeddingDimension || len(vector) != dimension {
			continue
		}
		if len(sum) == 0 {
			sum = make([]float64, len(vector))
		}
		if len(vector) != len(sum) {
			continue
		}
		weight := domainrecommendation.EventWeight(eventType, watchMs, completed)
		for i := range vector {
			sum[i] += vector[i] * weight
		}
		totalWeight += weight
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(sum) == 0 || totalWeight == 0 {
		return nil, 0, nil
	}
	for i := range sum {
		sum[i] = sum[i] / totalWeight
	}
	return sum, totalWeight, nil
}

// ApplyUserInterest 用单个视频的向量增量修正缓存中的用户兴趣向量。
// 没有配置缓存时无需写入，回源聚合本身就是实时的。
func (r *Repository) ApplyUserInterest(ctx context.Context, userID int64, videoID int64, weight float64) error {
	if r.interestCache == nil || userID <= 0 || videoID <= 0 || weight <= 0 {
		return nil
	}
	vectors, err := r.LoadVideoVectors(ctx, []int64{videoID})
	if err != nil {
		return err
	}
	vector := vectors[videoID]
	if len(vector) == 0 {
		return nil
	}
	return r.interestCache.Apply(ctx, userID, vector, weight)
}

func (r *Repository) LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error) {
	vectors := map[int64][]float64{}
	if len(videoIDs) == 0 {
		return vectors, nil
	}

	var models []videoVectorModel
	err := r.db.WithContext(ctx).
		Table("video_embedding").
		Select("video_id, dimension, embedding_json").
		Where("video_id IN ? AND model = ? AND dimension = ?", videoIDs, r.embeddingModel, r.embeddingDimension).
		Scan(&models).
		Error
	if err != nil {
		return nil, err
	}
	for _, model := range models {
		vector, err := decodeVector(model.EmbeddingJSON)
		if err != nil || model.Dimension != r.embeddingDimension || len(vector) != model.Dimension {
			continue
		}
		vectors[model.VideoID] = vector
	}
	return vectors, nil
}

func (r *Repository) ListRecentExposures(ctx context.Context, userID int64, videoIDs []int64, since time.Time) ([]*domainrecommendation.Exposure, error) {
	if len(videoIDs) == 0 {
		return []*domainrecommendation.Exposure{}, nil
	}

	var models []infraexposure.ExposureModel
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND video_id IN ? AND last_exposed_at >= ?", userID, videoIDs, since).
		Find(&models).
		Error
	if err != nil {
		return nil, err
	}
	exposures := make([]*domainrecommendation.Exposure, 0, len(models))
	for _, model := range models {
		exposures = append(exposures, restoreExposure(model))
	}
	return exposures, nil
}

func (r *Repository) SaveExposures(ctx context.Context, writes []*domainrecommendation.ExposureWrite) ([]*domainrecommendation.Exposure, error) {
	if len(writes) == 0 {
		return []*domainrecommendation.Exposure{}, nil
	}

	exposures := make([]*domainrecommendation.Exposure, 0, len(writes))
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, write := range writes {
			if write == nil {
				continue
			}
			if err := ensurePublishedVideo(tx, write.VideoID); err != nil {
				return err
			}
			event := infraexposure.ViewEventModel{
				UserID:    write.UserID,
				VideoID:   write.VideoID,
				Scene:     write.Scene,
				RequestID: stringPtr(write.RequestID),
				EventType: domainexposure.EventTypeExposed,
				WatchMs:   0,
				Completed: false,
			}
			if err := tx.Create(&event).Error; err != nil {
				return err
			}
			model := infraexposure.ExposureModel{
				UserID:         write.UserID,
				VideoID:        write.VideoID,
				FirstExposedAt: event.CreatedAt,
				LastExposedAt:  event.CreatedAt,
				ExposureCount:  1,
				LastScene:      write.Scene,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "user_id"},
					{Name: "video_id"},
				},
				DoUpdates: clause.Assignments(map[string]any{
					"last_exposed_at": gorm.Expr("VALUES(last_exposed_at)"),
					"exposure_count":  gorm.Expr("exposure_count + 1"),
					"last_scene":      gorm.Expr("VALUES(last_scene)"),
					"updated_at":      gorm.Expr("VALUES(updated_at)"),
				}),
			}).Create(&model).Error; err != nil {
				return err
			}

			var saved infraexposure.ExposureModel
			if err := tx.Where("user_id = ? AND video_id = ?", write.UserID, write.VideoID).Take(&saved).Error; err != nil {
				return err
			}
			exposures = append(exposures, restoreExposure(saved))
		}
		return nil
	})
	if err != nil {
		return nil, mapRecommendationError(err)
	}
	return exposures, nil
}

func decodeVector(content string) ([]float64, error) {
	var vector []float64
	if err := json.Unmarshal([]byte(content), &vector); err != nil {
		return nil, err
	}
	return vector, nil
}

func positiveEventTypes() []string {
	return []string{
		domainexposure.EventTypePlay,
		domainexposure.EventTypeComplete,
	}
}

func ensurePublishedVideo(tx *gorm.DB, videoID int64) error {
	var item struct {
		ID int64
	}
	err := tx.Table("video").
		Select("id").
		Where("id = ? AND status = ?", videoID, domainvideo.StatusPublished).
		Take(&item).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domainrecommendation.ErrVideoNotFound
		}
		return err
	}
	return nil
}

func restoreExposure(model infraexposure.ExposureModel) *domainrecommendation.Exposure {
	return domainrecommendation.RestoreExposure(
		model.ID,
		model.UserID,
		model.VideoID,
		model.FirstExposedAt,
		model.LastExposedAt,
		model.ExposureCount,
		model.LastScene,
	)
}

func mapRecommendationError(err error) error {
	if errors.Is(err, domainrecommendation.ErrVideoNotFound) {
		return err
	}
	return err
}

func stringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
