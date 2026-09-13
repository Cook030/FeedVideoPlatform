package httputil

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// ParseLimit 解析用户显式传入的 limit；空值返回 (0,nil)，由应用层决定默认页大小。
// invalid 由调用方传入，以保证各端点的错误文案不变。
func ParseLimit(raw string, invalid error) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, invalid
	}
	return limit, nil
}

// ParsePositiveInt64 解析路径或查询参数中的正整数 ID；解析失败或非正数返回 invalid。
func ParsePositiveInt64(raw string, invalid error) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, invalid
	}
	return value, nil
}

// ParseOptionalInt64 解析可选的非负整数；空值返回 (0,nil)，负数或解析失败返回 invalid。
func ParseOptionalInt64(raw string, invalid error) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return 0, invalid
	}
	return value, nil
}

// ParsePagination 解析 offset 分页参数：limit 缺省使用 defLimit，offset 允许为 0。
func ParsePagination(c *gin.Context, defLimit int, invalidLimit error, invalidOffset error) (int, int, error) {
	limit := defLimit
	offset := 0

	rawLimit := strings.TrimSpace(c.Query("limit"))
	if rawLimit != "" {
		// limit 必须为正数，应用层会进一步限制最大值。
		value, err := strconv.Atoi(rawLimit)
		if err != nil || value <= 0 {
			return 0, 0, invalidLimit
		}
		limit = value
	}

	rawOffset := strings.TrimSpace(c.Query("offset"))
	if rawOffset != "" {
		// offset 允许为 0，表示从第一条开始。
		value, err := strconv.Atoi(rawOffset)
		if err != nil || value < 0 {
			return 0, 0, invalidOffset
		}
		offset = value
	}

	return limit, offset, nil
}
