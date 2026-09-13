package domainembedding

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"strings"
	"time"
)

// Hash n-gram 模型的标识与维度。
// 常量留在领域层，供基础设施实现与测试共同引用。
const HashNgramModel = "hash-ngram-v1"
const HashNgramDimension = 128

// VideoEmbedding 保存一个视频文本内容对应的向量。
type VideoEmbedding struct {
	VideoID       int64
	Model         string
	Dimension     int
	Embedding     []float64
	TextHash      string
	EmbeddingJSON string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// BuildVideoText 把视频标题和简介拼成稳定向量输入。
func BuildVideoText(title string, description string) string {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if description == "" {
		return title
	}
	if title == "" {
		return description
	}
	return title + "\n" + description
}

// TextHash 计算文本哈希，方便重复发布事件判断内容是否变化。
func TextHash(text string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(text)))
	return hex.EncodeToString(sum[:])
}

// CosineSimilarity 计算两个向量的余弦相似度，长度不一致时返回 ErrDimensionMismatch。
func CosineSimilarity(left []float64, right []float64) (float64, error) {
	if len(left) != len(right) {
		return 0, ErrDimensionMismatch
	}
	var dot float64
	var leftNorm float64
	var rightNorm float64
	for i := range left {
		dot += left[i] * right[i]
		leftNorm += left[i] * left[i]
		rightNorm += right[i] * right[i]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), nil
}

// NewVideoEmbedding 创建视频向量领域对象。
func NewVideoEmbedding(videoID int64, model string, embedding []float64, textHash string, embeddingJSON string) *VideoEmbedding {
	return &VideoEmbedding{
		VideoID:       videoID,
		Model:         strings.TrimSpace(model),
		Dimension:     len(embedding),
		Embedding:     cloneVector(embedding),
		TextHash:      strings.TrimSpace(textHash),
		EmbeddingJSON: strings.TrimSpace(embeddingJSON),
	}
}

// RestoreVideoEmbedding 从数据库记录恢复领域对象。
func RestoreVideoEmbedding(videoID int64, model string, dimension int, embeddingJSON string, textHash string, createdAt time.Time, updatedAt time.Time) *VideoEmbedding {
	return &VideoEmbedding{
		VideoID:       videoID,
		Model:         strings.TrimSpace(model),
		Dimension:     dimension,
		EmbeddingJSON: strings.TrimSpace(embeddingJSON),
		TextHash:      strings.TrimSpace(textHash),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
	}
}

func cloneVector(vector []float64) []float64 {
	cloned := make([]float64, len(vector))
	copy(cloned, vector)
	return cloned
}
