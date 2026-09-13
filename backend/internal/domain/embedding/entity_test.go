package domainembedding

import (
	"errors"
	"testing"
)

func TestBuildVideoTextIncludesStableTags(t *testing.T) {
	first := BuildVideoText(" 篮球训练 ", " 投篮技巧 ", []string{"篮球", "教学", "篮球", " "})
	second := BuildVideoText("篮球训练", "投篮技巧", []string{"教学", "篮球"})
	want := "篮球训练\n投篮技巧\n教学\n篮球"
	if first != want || second != want {
		t.Fatalf("unexpected video text: first=%q second=%q", first, second)
	}
}

func TestCosineSimilarityRejectsDimensionMismatch(t *testing.T) {
	_, err := CosineSimilarity([]float64{1, 0}, []float64{1, 0, 0})
	if !errors.Is(err, ErrDimensionMismatch) {
		t.Fatalf("expected dimension mismatch, got %v", err)
	}
}
