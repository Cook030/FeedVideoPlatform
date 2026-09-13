package applicationupload

import (
	"errors"

	contract "GCFeed/internal/shared/contract"
)

// ErrStorageFailed 表示落盘阶段失败（磁盘、权限等内部错误），对外应返回 500。
var ErrStorageFailed = errors.New("failed to save upload")

// 上传相关错误统一来自 contract，这里保留包内别名便于调用方用 errors.Is 判定。
var (
	ErrInvalidUploadExtension = contract.ErrInvalidUploadExtension
	ErrInvalidUploadMIME      = contract.ErrInvalidUploadMIME
	ErrUploadTooLarge         = contract.ErrUploadTooLarge
	ErrVideoTooLong           = contract.ErrVideoTooLong
	ErrVideoTooLargeDimension = contract.ErrVideoTooLargeDimension
	ErrUnsupportedVideoCodec  = contract.ErrUnsupportedVideoCodec
	ErrInvalidVideoMetadata   = contract.ErrInvalidVideoMetadata
	ErrVideoToolUnavailable   = contract.ErrVideoToolUnavailable
	ErrFaststartFailed        = contract.ErrFaststartFailed
)
