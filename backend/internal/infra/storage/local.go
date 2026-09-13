// Package infrastorage 提供本地磁盘的上传文件存储实现。
package infrastorage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strconv"
	"time"

	contract "GCFeed/internal/shared/contract"
)

// LocalStorage 把上传文件保存到本地目录（默认 ./uploads）。
type LocalStorage struct {
	root string
}

// New 创建本地存储，root 是文件保存根目录。
func New(root string) *LocalStorage {
	return &LocalStorage{root: root}
}

// NewFilename 生成带时间戳与随机后缀的文件名，降低不同用户上传同名文件的冲突概率。
func (s *LocalStorage) NewFilename(ext string) string {
	return fmt.Sprintf("%d-%s%s", time.Now().UnixNano(), randomSuffix(), ext)
}

// Save 把上传文件写入 kind 子目录，返回相对 URL 与物理路径。
func (s *LocalStorage) Save(ctx context.Context, kind string, filename string, file *multipart.FileHeader) (contract.SavedFile, error) {
	targetDir := filepath.Join(s.root, kind)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return contract.SavedFile{}, err
	}
	targetPath := filepath.Join(targetDir, filename)

	src, err := file.Open()
	if err != nil {
		return contract.SavedFile{}, err
	}
	defer src.Close()

	dst, err := os.Create(targetPath)
	if err != nil {
		return contract.SavedFile{}, err
	}
	written, copyErr := io.Copy(dst, src)
	closeErr := dst.Close()
	if copyErr != nil {
		_ = os.Remove(targetPath)
		return contract.SavedFile{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(targetPath)
		return contract.SavedFile{}, closeErr
	}

	size := written
	if file.Size > 0 {
		size = file.Size
	}
	return contract.SavedFile{
		URL:  kind + "/" + filename,
		Path: targetPath,
		Size: size,
	}, nil
}

// Remove 删除已落盘文件，用于处理失败时的回滚。
func (s *LocalStorage) Remove(ctx context.Context, file contract.SavedFile) error {
	if file.Path == "" {
		return nil
	}
	return os.Remove(file.Path)
}

// randomSuffix 生成文件名随机后缀，随机失败时用时间戳兜底。
func randomSuffix() string {
	buf := make([]byte, 6)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}
