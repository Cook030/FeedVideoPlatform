// Package infraembedding 提供文本向量化的基础设施实现。
package infraembedding

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"

	domainembedding "GCFeed/internal/domain/embedding"
)

// HashNgramVectorizer 是零依赖的本地伪 embedding，适合先跑通推荐链路。
type HashNgramVectorizer struct {
	dimension int
}

// NewHashNgramVectorizer 创建当前版本的默认 128 维 hash n-gram 向量器。
func NewHashNgramVectorizer() *HashNgramVectorizer {
	return &HashNgramVectorizer{dimension: domainembedding.HashNgramDimension}
}

// Model 返回模型标识。
func (v *HashNgramVectorizer) Model() string {
	return domainembedding.HashNgramModel
}

// Dimension 返回向量维度。
func (v *HashNgramVectorizer) Dimension() int {
	if v == nil || v.dimension <= 0 {
		return domainembedding.HashNgramDimension
	}
	return v.dimension
}

// Vectorize 使用字符 n-gram 和 token 特征生成稳定向量，并做 L2 归一化。
func (v *HashNgramVectorizer) Vectorize(text string) []float64 {
	dimension := v.Dimension()
	vector := make([]float64, dimension)
	normalized := normalizeText(text)
	if normalized == "" {
		return vector
	}

	tokens := strings.Fields(normalized)
	for _, token := range tokens {
		addFeature(vector, "tok:"+token, 1.0)
	}

	runes := []rune(strings.ReplaceAll(normalized, " ", ""))
	for n := 2; n <= 3; n++ {
		if len(runes) < n {
			continue
		}
		for i := 0; i+n <= len(runes); i++ {
			addFeature(vector, string(runes[i:i+n]), 1.0)
		}
	}

	normalizeVector(vector)
	return vector
}

func normalizeText(text string) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}

	var builder strings.Builder
	previousSpace := true
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			previousSpace = false
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if !previousSpace {
				builder.WriteRune(' ')
				previousSpace = true
			}
		}
	}
	return strings.TrimSpace(builder.String())
}

func addFeature(vector []float64, feature string, weight float64) {
	if len(vector) == 0 || feature == "" {
		return
	}
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(feature))
	hash := hasher.Sum64()
	index := int(hash % uint64(len(vector)))
	sign := 1.0
	if (hash>>63)&1 == 1 {
		sign = -1.0
	}
	vector[index] += sign * weight
}

func normalizeVector(vector []float64) {
	var sum float64
	for _, value := range vector {
		sum += value * value
	}
	if sum == 0 {
		return
	}
	norm := math.Sqrt(sum)
	for i := range vector {
		vector[i] = vector[i] / norm
	}
}
