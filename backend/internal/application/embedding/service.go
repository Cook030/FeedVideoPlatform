package applicationembedding

import (
	domainembedding "GCFeed/internal/domain/embedding"
	contract "GCFeed/internal/shared/contract"
	"context"
	"encoding/json"
	"errors"
)

var ErrSaveVideoEmbeddingFailed = errors.New("failed to save video embedding")
var ErrMarshalEmbeddingFailed = errors.New("failed to marshal embedding")
var ErrVectorizerUnavailable = errors.New("embedding vectorizer is unavailable")
var ErrLoadVideoTextsFailed = errors.New("failed to load video texts")
var ErrInvalidVideoTextPage = errors.New("invalid video text page")

type Service struct {
	repo       domainembedding.Repository
	vectorizer domainembedding.Vectorizer
}

// RebuildPublishedVideos 分页重算全部已发布视频的当前模型向量。
// SaveVideoEmbedding 使用 video_id + model upsert，因此命令可安全重复执行。
func (s *Service) RebuildPublishedVideos(ctx context.Context, source domainembedding.VideoTextSource, batchSize int) (int, error) {
	if source == nil {
		return 0, ErrLoadVideoTextsFailed
	}
	if batchSize <= 0 {
		batchSize = 200
	}

	afterVideoID := int64(0)
	rebuilt := 0
	for {
		items, err := source.ListPublishedVideoTexts(ctx, afterVideoID, batchSize)
		if err != nil {
			return rebuilt, ErrLoadVideoTextsFailed
		}
		if len(items) == 0 {
			return rebuilt, nil
		}

		lastVideoID := afterVideoID
		for _, item := range items {
			if item == nil || item.VideoID <= afterVideoID {
				continue
			}
			if _, err := s.GenerateForPublishedVideo(ctx, &contract.PublishedEvent{
				VideoID:     item.VideoID,
				Title:       item.Title,
				Description: item.Description,
				Tags:        item.Tags,
			}); err != nil {
				return rebuilt, err
			}
			rebuilt++
			if item.VideoID > lastVideoID {
				lastVideoID = item.VideoID
			}
		}
		if lastVideoID == afterVideoID {
			return rebuilt, ErrInvalidVideoTextPage
		}
		afterVideoID = lastVideoID
		if len(items) < batchSize {
			return rebuilt, nil
		}
	}
}

type GenerateVideoEmbeddingResult struct {
	Embedding        *domainembedding.VideoEmbedding
	CreatedOrUpdated bool
}

// New 装配向量化服务；vectorizer 由基础设施层实现并在装配时注入。
func New(repo domainembedding.Repository, vectorizer domainembedding.Vectorizer) *Service {
	return &Service{
		repo:       repo,
		vectorizer: vectorizer,
	}
}

// GenerateForPublishedVideo 根据视频发布事件生成并保存视频内容向量。
func (s *Service) GenerateForPublishedVideo(ctx context.Context, event *contract.PublishedEvent) (*GenerateVideoEmbeddingResult, error) {
	if event == nil || event.VideoID <= 0 {
		return &GenerateVideoEmbeddingResult{}, nil
	}
	if s.vectorizer == nil {
		return nil, ErrVectorizerUnavailable
	}

	text := domainembedding.BuildVideoText(event.Title, event.Description, event.Tags)
	vector := s.vectorizer.Vectorize(text)
	if len(vector) != s.vectorizer.Dimension() {
		return nil, domainembedding.ErrDimensionMismatch
	}
	content, err := json.Marshal(vector)
	if err != nil {
		return nil, ErrMarshalEmbeddingFailed
	}

	embedding := domainembedding.NewVideoEmbedding(
		event.VideoID,
		s.vectorizer.Model(),
		vector,
		domainembedding.TextHash(text),
		string(content),
	)
	if err := s.repo.SaveVideoEmbedding(ctx, embedding); err != nil {
		return nil, ErrSaveVideoEmbeddingFailed
	}

	return &GenerateVideoEmbeddingResult{
		Embedding:        embedding,
		CreatedOrUpdated: true,
	}, nil
}
