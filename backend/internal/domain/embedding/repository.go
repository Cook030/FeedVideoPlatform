package domainembedding

import "context"

// Repository 定义 embedding 模块需要的持久化能力。
type Repository interface {
	SaveVideoEmbedding(ctx context.Context, embedding *VideoEmbedding) error
	FindVideoEmbedding(ctx context.Context, videoID int64, model string) (*VideoEmbedding, error)
}

// VideoText 是重算向量所需的最小视频内容快照。
type VideoText struct {
	VideoID     int64
	Title       string
	Description string
	Tags        []string
}

// VideoTextSource 分页提供已发布视频，供一次性向量重算使用。
type VideoTextSource interface {
	ListPublishedVideoTexts(ctx context.Context, afterVideoID int64, limit int) ([]*VideoText, error)
}

// Vectorizer 把文本转换为固定维度向量。
//
// 领域层只依赖该抽象；具体算法实现（hash n-gram）位于基础设施层，
// 使领域不再内嵌哈希与归一化等算法细节。
type Vectorizer interface {
	Model() string
	Dimension() int
	Vectorize(text string) []float64
}
