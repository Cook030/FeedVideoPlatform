package applicationupload

import (
	"context"
	"mime/multipart"

	contract "GCFeed/internal/shared/contract"
)

// Storage 负责上传文件的命名、落盘与回滚删除。
type Storage interface {
	// NewFilename 生成不会与既有文件冲突的文件名（含扩展名）。
	NewFilename(ext string) string
	// Save 把上传文件写入 kind 对应目录。
	Save(ctx context.Context, kind string, filename string, file *multipart.FileHeader) (contract.SavedFile, error)
	// Remove 删除已落盘文件，用于后续处理失败时的回滚。
	Remove(ctx context.Context, file contract.SavedFile) error
}

// MediaProcessor 负责媒体探测与 MP4 faststart 处理。
type MediaProcessor interface {
	Probe(ctx context.Context, path string) (*contract.ProbeResult, error)
	Faststart(ctx context.Context, path string) error
}
