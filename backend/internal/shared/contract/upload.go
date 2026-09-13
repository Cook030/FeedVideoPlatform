package contract

import "errors"

// 上传校验与媒体处理错误。
// 定义在中立契约包，使 application 编排层与 infra 适配器都能判定同一组错误，
// 且 infra 不需要反向依赖 application。
var (
	ErrInvalidUploadExtension = errors.New("unsupported upload extension")
	ErrInvalidUploadMIME      = errors.New("unsupported upload content type")
	ErrUploadTooLarge         = errors.New("upload file is too large")
	ErrVideoTooLong           = errors.New("video duration is too long")
	ErrVideoTooLargeDimension = errors.New("video resolution is too large")
	ErrUnsupportedVideoCodec  = errors.New("video codec is unsupported")
	ErrInvalidVideoMetadata   = errors.New("video metadata is invalid")
	ErrVideoToolUnavailable   = errors.New("video tool is unavailable")
	ErrFaststartFailed        = errors.New("video faststart failed")
)

// ProbeResult 是媒体探测结果，由 infra 适配器产出、application 规则层消费。
type ProbeResult struct {
	DurationSeconds float64
	HasVideo        bool
	Width           int
	Height          int
	VideoCodec      string
	AudioCodec      string
}

// SavedFile 描述一次成功落盘的上传文件。
// 放在契约包是为了让 application 端口与 infra 存储实现共用同一类型。
type SavedFile struct {
	// URL 是相对路径，例如 video/1700000000-abcd.mp4，用于拼接访问地址。
	URL string
	// Path 是物理路径，供媒体处理直接操作文件。
	Path string
	// Size 是文件字节数。
	Size int64
}
