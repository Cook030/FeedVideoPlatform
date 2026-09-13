package domainvideo

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeTagsReturnsStableSet(t *testing.T) {
	tags, err := NormalizeTags([]string{" 篮球 ", "教学", "篮球", ""})
	if err != nil {
		t.Fatalf("normalize tags: %v", err)
	}
	if !reflect.DeepEqual(tags, []string{"教学", "篮球"}) {
		t.Fatalf("unexpected normalized tags: %+v", tags)
	}
}

func TestNormalizeTagsEnforcesLimits(t *testing.T) {
	tooMany := make([]string, MaxTagCount+1)
	for i := range tooMany {
		tooMany[i] = string(rune('a' + i))
	}
	if _, err := NormalizeTags(tooMany); !errors.Is(err, ErrTooManyTags) {
		t.Fatalf("expected too many tags, got %v", err)
	}
	if _, err := NormalizeTags([]string{strings.Repeat("标", MaxTagLength+1)}); !errors.Is(err, ErrTagTooLong) {
		t.Fatalf("expected tag too long, got %v", err)
	}
}
