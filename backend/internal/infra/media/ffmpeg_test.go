package inframedia

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateFaststartTempFileUsesMP4Extension 迁移自原 interfaces/http/upload/handler_test.go。
func TestCreateFaststartTempFileUsesMP4Extension(t *testing.T) {
	dir := t.TempDir()
	tmp, err := createFaststartTempFile(filepath.Join(dir, "clip.mp4"))
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	if filepath.Dir(tmpPath) != dir {
		t.Fatalf("expected temp file in upload dir, got %s", tmpPath)
	}
	if !strings.HasSuffix(tmpPath, ".faststart.mp4") {
		t.Fatalf("expected mp4 temp extension, got %s", tmpPath)
	}
}
