package applicationupload

import (
	"testing"

	contract "GCFeed/internal/shared/contract"
)

// TestValidateMetadata 迁移自原 interfaces/http/upload/handler_test.go，
// 断言与历史版本一致，仅改为消费中立的 ProbeResult。
func TestValidateMetadata(t *testing.T) {
	valid := &contract.ProbeResult{
		DurationSeconds: 60.5,
		HasVideo:        true,
		Width:           1920,
		Height:          1080,
		VideoCodec:      "h264",
		AudioCodec:      "aac",
	}
	if err := ValidateMetadata(valid); err != nil {
		t.Fatalf("expected valid metadata, got %v", err)
	}

	for _, codecName := range []string{"h264", "h265", "hevc", "vp8", "vp9", "av1"} {
		metadata := *valid
		metadata.VideoCodec = codecName
		if err := ValidateMetadata(&metadata); err != nil {
			t.Fatalf("expected %s metadata to pass, got %v", codecName, err)
		}
	}

	long := *valid
	long.DurationSeconds = 900
	if err := ValidateMetadata(&long); err == nil {
		t.Fatalf("expected long video metadata to fail")
	}

	large := *valid
	large.Width = 4096
	large.Height = 2160
	if err := ValidateMetadata(&large); err == nil {
		t.Fatalf("expected large video metadata to fail")
	}

	codec := *valid
	codec.VideoCodec = "mpeg2video"
	if err := ValidateMetadata(&codec); err == nil {
		t.Fatalf("expected unsupported codec metadata to fail")
	}
}
