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

type Service struct {
	repo       domainembedding.Repository
	vectorizer domainembedding.Vectorizer
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

	text := domainembedding.BuildVideoText(event.Title, event.Description)
	vector := s.vectorizer.Vectorize(text)
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
