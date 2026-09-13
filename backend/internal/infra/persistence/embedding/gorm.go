package infraembedding

import (
	domainembedding "GCFeed/internal/domain/embedding"
	domainvideo "GCFeed/internal/domain/video"
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type publishedVideoTextModel struct {
	VideoID     int64
	Title       string
	Description string
}

type Repository struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// SaveVideoEmbedding 使用 video_id + model upsert，重复发布事件会覆盖同模型向量。
func (r *Repository) SaveVideoEmbedding(ctx context.Context, embedding *domainembedding.VideoEmbedding) error {
	model := VideoEmbeddingModel{
		VideoID:       embedding.VideoID,
		Model:         embedding.Model,
		Dimension:     embedding.Dimension,
		EmbeddingJSON: embedding.EmbeddingJSON,
		TextHash:      embedding.TextHash,
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "video_id"},
			{Name: "model"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"dimension",
			"embedding_json",
			"text_hash",
			"updated_at",
		}),
	}).Create(&model).Error
}

// FindVideoEmbedding 按 video_id + model 查询视频向量。
func (r *Repository) FindVideoEmbedding(ctx context.Context, videoID int64, model string) (*domainembedding.VideoEmbedding, error) {
	var item VideoEmbeddingModel
	err := r.db.WithContext(ctx).
		Where("video_id = ? AND model = ?", videoID, model).
		Take(&item).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainembedding.ErrVideoEmbeddingNotFound
		}
		return nil, err
	}
	return domainembedding.RestoreVideoEmbedding(
		item.VideoID,
		item.Model,
		item.Dimension,
		item.EmbeddingJSON,
		item.TextHash,
		item.CreatedAt,
		item.UpdatedAt,
	), nil
}

// ListPublishedVideoTexts 按 video_id 游标读取重算所需内容，并批量附加标签。
func (r *Repository) ListPublishedVideoTexts(ctx context.Context, afterVideoID int64, limit int) ([]*domainembedding.VideoText, error) {
	if limit <= 0 {
		return []*domainembedding.VideoText{}, nil
	}
	var models []publishedVideoTextModel
	if err := r.db.WithContext(ctx).
		Table("video").
		Select("id AS video_id, title, description").
		Where("id > ? AND status = ?", afterVideoID, domainvideo.StatusPublished).
		Order("id ASC").
		Limit(limit).
		Scan(&models).Error; err != nil {
		return nil, err
	}
	if len(models) == 0 {
		return []*domainembedding.VideoText{}, nil
	}

	items := make([]*domainembedding.VideoText, 0, len(models))
	itemByID := make(map[int64]*domainembedding.VideoText, len(models))
	videoIDs := make([]int64, 0, len(models))
	for _, model := range models {
		item := &domainembedding.VideoText{
			VideoID:     model.VideoID,
			Title:       model.Title,
			Description: model.Description,
			Tags:        []string{},
		}
		items = append(items, item)
		itemByID[item.VideoID] = item
		videoIDs = append(videoIDs, item.VideoID)
	}

	var tags []struct {
		VideoID int64
		Tag     string
	}
	if err := r.db.WithContext(ctx).
		Table("video_tag").
		Select("video_id, tag").
		Where("video_id IN ?", videoIDs).
		Order("video_id ASC").
		Order("tag ASC").
		Scan(&tags).Error; err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if item := itemByID[tag.VideoID]; item != nil {
			item.Tags = append(item.Tags, tag.Tag)
		}
	}
	return items, nil
}
