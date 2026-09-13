package domainembedding

import "context"

// Repository 定义 embedding 模块需要的持久化能力。
type Repository interface {
	SaveVideoEmbedding(ctx context.Context, embedding *VideoEmbedding) error
	FindVideoEmbedding(ctx context.Context, videoID int64, model string) (*VideoEmbedding, error)
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
