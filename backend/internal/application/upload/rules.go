package applicationupload

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"

	contract "GCFeed/internal/shared/contract"
)

// 上传大小、时长、分辨率与编解码白名单，全部属于业务校验规则。
const (
	MaxUploadBytes          = 1024 << 20
	maxVideoBytes           = 512 << 20
	maxImageBytes           = 20 << 20
	MaxVideoDurationSeconds = 10 * 60
	MaxVideoDimension       = 3840
	sniffBytes              = 512
)

var allowedVideoExt = map[string]struct{}{
	".mp4":  {},
	".mov":  {},
	".webm": {},
}

var allowedImageExt = map[string]struct{}{
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".webp": {},
}

var allowedVideoMIME = map[string]struct{}{
	"application/octet-stream": {},
	"video/mp4":                {},
	"video/quicktime":          {},
	"video/webm":               {},
}

var allowedImageMIME = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

var allowedVideoCodec = map[string]struct{}{
	"h264": {},
	"h265": {},
	"hevc": {},
	"vp8":  {},
	"vp9":  {},
	"av1":  {},
}

var allowedAudioCodec = map[string]struct{}{
	"aac":    {},
	"mp3":    {},
	"opus":   {},
	"vorbis": {},
}

// Validation 描述一次校验通过后的文件元信息。
type Validation struct {
	Ext  string
	MIME string
}

// NormalizeKind 规范化文件分类，分类会影响保存目录和访问 URL。
func NormalizeKind(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "video":
		return "video", true
	case "cover":
		return "cover", true
	case "avatar":
		return "avatar", true
	case "":
		return "file", true
	case "file":
		return "file", true
	default:
		return "", false
	}
}

// ValidateFile 校验上传文件的大小、扩展名与嗅探出的 MIME 类型。
func ValidateFile(file *multipart.FileHeader, kind string) (*Validation, error) {
	if err := validateUploadSize(file.Size, kind); err != nil {
		return nil, err
	}

	ext := strings.ToLower(filepath.Ext(filepath.Base(file.Filename)))
	if ext == "" {
		if kind == "file" {
			ext = ".bin"
		} else {
			return nil, ErrInvalidUploadExtension
		}
	}

	if err := validateUploadExtension(ext, kind); err != nil {
		return nil, err
	}
	if kind == "file" {
		return &Validation{Ext: ext}, nil
	}

	mimeType, err := sniffUploadMIME(file)
	if err != nil {
		return nil, err
	}
	if err := validateUploadMIME(mimeType, kind); err != nil {
		return nil, err
	}

	return &Validation{Ext: ext, MIME: mimeType}, nil
}

func validateUploadSize(size int64, kind string) error {
	if size <= 0 {
		return ErrInvalidUploadMIME
	}
	if kind == "video" && size > maxVideoBytes {
		return ErrUploadTooLarge
	}
	if (kind == "cover" || kind == "avatar") && size > maxImageBytes {
		return ErrUploadTooLarge
	}
	if size > MaxUploadBytes {
		return ErrUploadTooLarge
	}
	return nil
}

func validateUploadExtension(ext string, kind string) error {
	switch kind {
	case "video":
		if _, ok := allowedVideoExt[ext]; !ok {
			return ErrInvalidUploadExtension
		}
	case "cover", "avatar":
		if _, ok := allowedImageExt[ext]; !ok {
			return ErrInvalidUploadExtension
		}
	}
	return nil
}

func sniffUploadMIME(fileHeader *multipart.FileHeader) (string, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return "", ErrInvalidUploadMIME
	}
	defer file.Close()

	header := make([]byte, sniffBytes)
	n, err := io.ReadFull(file, header)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "", ErrInvalidUploadMIME
	}
	if n == 0 {
		return "", ErrInvalidUploadMIME
	}
	return http.DetectContentType(header[:n]), nil
}

func validateUploadMIME(mimeType string, kind string) error {
	switch kind {
	case "video":
		if _, ok := allowedVideoMIME[mimeType]; !ok {
			return ErrInvalidUploadMIME
		}
	case "cover", "avatar":
		if _, ok := allowedImageMIME[mimeType]; !ok {
			return ErrInvalidUploadMIME
		}
	}
	return nil
}

// ShouldFaststart 判断该扩展名是否需要 faststart 处理。
func ShouldFaststart(ext string) bool {
	ext = strings.ToLower(strings.TrimSpace(ext))
	return ext == ".mp4" || ext == ".mov"
}

// ValidateMetadata 用业务规则校验媒体探测结果。
func ValidateMetadata(metadata *contract.ProbeResult) error {
	if metadata == nil {
		return ErrInvalidVideoMetadata
	}
	if metadata.DurationSeconds <= 0 {
		return ErrInvalidVideoMetadata
	}
	if metadata.DurationSeconds > MaxVideoDurationSeconds {
		return ErrVideoTooLong
	}
	if !metadata.HasVideo {
		return ErrInvalidVideoMetadata
	}
	if metadata.Width <= 0 || metadata.Height <= 0 {
		return ErrInvalidVideoMetadata
	}
	if metadata.Width > MaxVideoDimension || metadata.Height > MaxVideoDimension {
		return ErrVideoTooLargeDimension
	}
	if _, ok := allowedVideoCodec[metadata.VideoCodec]; !ok {
		return ErrUnsupportedVideoCodec
	}
	if metadata.AudioCodec != "" {
		if _, ok := allowedAudioCodec[metadata.AudioCodec]; !ok {
			return ErrUnsupportedVideoCodec
		}
	}
	return nil
}
