package applicationfeed

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	domainfeed "GCFeed/internal/domain/feed"
)

// normalizeLimit 统一默认页大小和最大页大小。
func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultFeedLimit
	}
	if limit > domainfeed.MaxLimit {
		return domainfeed.MaxLimit
	}
	return limit
}

// clientContextValue 读取客户端透传的上下文字段，缺失时返回空串。
func clientContextValue(context map[string]string, key string) string {
	if context == nil {
		return ""
	}
	return context[key]
}

// parseTimelineCursor 将客户端传回的字符串游标解析成领域游标。
func parseTimelineCursor(raw string) (*domainfeed.TimelineCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainfeed.ErrInvalidCursor
		}
	}

	var payload timelineCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil || payload.VideoID <= 0 {
		return nil, domainfeed.ErrInvalidCursor
	}

	return &domainfeed.TimelineCursor{
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}, nil
}

// parseHotCursor 将客户端传回的热榜游标解析成领域游标。
func parseHotCursor(raw string) (*domainfeed.HotCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainfeed.ErrInvalidCursor
		}
	}

	var payload hotCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainfeed.ErrInvalidCursor
	}

	if strings.TrimSpace(payload.WindowEnd) != "" {
		windowEnd, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.WindowEnd))
		if err != nil || payload.Offset < 0 {
			return nil, domainfeed.ErrInvalidCursor
		}
		return &domainfeed.HotCursor{
			WindowEnd: windowEnd,
			Offset:    payload.Offset,
		}, nil
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil || payload.VideoID <= 0 {
		return nil, domainfeed.ErrInvalidCursor
	}

	return &domainfeed.HotCursor{
		HotScore:    payload.HotScore,
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}, nil
}

// encodeTimelineCursor 把排序字段编码成 URL 安全的游标字符串。
func encodeTimelineCursor(cursor *domainfeed.TimelineCursor) string {
	if cursor == nil || cursor.VideoID <= 0 || cursor.PublishedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(timelineCursorPayload{
		PublishedAt: cursor.PublishedAt.UTC().Format(time.RFC3339Nano),
		VideoID:     cursor.VideoID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// encodeHotWindowCursor 把热榜滑动窗口位置编码成 URL 安全的游标字符串。
func encodeHotWindowCursor(cursor *domainfeed.HotCursor) string {
	if cursor == nil || cursor.WindowEnd.IsZero() || cursor.Offset < 0 {
		return ""
	}

	content, err := json.Marshal(hotCursorPayload{
		WindowEnd: cursor.WindowEnd.UTC().Format(time.RFC3339Nano),
		Offset:    cursor.Offset,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// encodeHotCursor 把热榜排序字段编码成 URL 安全的游标字符串。
func encodeHotCursor(cursor *domainfeed.HotCursor) string {
	if cursor == nil || cursor.VideoID <= 0 || cursor.PublishedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(hotCursorPayload{
		HotScore:    cursor.HotScore,
		PublishedAt: cursor.PublishedAt.UTC().Format(time.RFC3339Nano),
		VideoID:     cursor.VideoID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}
