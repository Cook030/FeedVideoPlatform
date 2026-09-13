package infraembedding

import (
	"math"
	"testing"

	domainembedding "GCFeed/internal/domain/embedding"
)

// 以下用例迁移自 domain/embedding/hash_ngram_test.go，断言保持不变。

func TestHashNgramVectorizerDeterministic(t *testing.T) {
	vectorizer := NewHashNgramVectorizer()

	left := vectorizer.Vectorize("篮球训练\n投篮技巧")
	right := vectorizer.Vectorize("篮球训练\n投篮技巧")

	if vectorizer.Model() != domainembedding.HashNgramModel {
		t.Fatalf("unexpected model: %s", vectorizer.Model())
	}
	if len(left) != domainembedding.HashNgramDimension || len(right) != domainembedding.HashNgramDimension {
		t.Fatalf("unexpected dimension: left=%d right=%d", len(left), len(right))
	}
	for i := range left {
		if left[i] != right[i] {
			t.Fatalf("vector changed at %d: %f != %f", i, left[i], right[i])
		}
	}
	if norm(left) < 0.999 || norm(left) > 1.001 {
		t.Fatalf("unexpected normalized vector norm: %f", norm(left))
	}
}

func TestHashNgramVectorizerSimilarity(t *testing.T) {
	vectorizer := NewHashNgramVectorizer()

	base := vectorizer.Vectorize("篮球训练 投篮技巧")
	similar := vectorizer.Vectorize("篮球投篮训练 技巧")
	different := vectorizer.Vectorize("美食探店 火锅推荐")

	similarScore, err := domainembedding.CosineSimilarity(base, similar)
	if err != nil {
		t.Fatalf("similar cosine: %v", err)
	}
	differentScore, err := domainembedding.CosineSimilarity(base, different)
	if err != nil {
		t.Fatalf("different cosine: %v", err)
	}
	if similarScore <= differentScore {
		t.Fatalf("expected similar text score to be higher: similar=%f different=%f", similarScore, differentScore)
	}
	if sameVector(base, different) {
		t.Fatalf("different text produced same vector")
	}
}

func TestHashNgramVectorizerEmptyText(t *testing.T) {
	vectorizer := NewHashNgramVectorizer()

	vector := vectorizer.Vectorize("   \n\t")
	if len(vector) != domainembedding.HashNgramDimension {
		t.Fatalf("unexpected dimension: %d", len(vector))
	}
	if norm(vector) != 0 {
		t.Fatalf("expected zero vector for empty text: %f", norm(vector))
	}
}

func norm(vector []float64) float64 {
	var sum float64
	for _, value := range vector {
		sum += value * value
	}
	return math.Sqrt(sum)
}

func sameVector(left []float64, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
