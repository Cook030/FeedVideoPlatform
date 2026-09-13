package applicationinteraction

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	domaininteraction "GCFeed/internal/domain/interaction"
)

type commentCursorPayload struct {
	CreatedAt string `json:"created_at"`
	CommentID int64  `json:"comment_id"`
}

// normalizeCommentLimit 统一评论分页默认值和最大值。
func normalizeCommentLimit(limit int) int {
	if limit <= 0 {
		return defaultCommentLimit
	}
	if limit > domaininteraction.MaxLimit {
		return domaininteraction.MaxLimit
	}
	return limit
}

// parseCommentCursor 解析上一页返回的游标，游标内保存最后一条评论的排序字段。
func parseCommentCursor(raw string) (*domaininteraction.CommentCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domaininteraction.ErrInvalidCursor
		}
	}

	var payload commentCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domaininteraction.ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.CreatedAt))
	if err != nil || payload.CommentID <= 0 {
		return nil, domaininteraction.ErrInvalidCursor
	}

	return &domaininteraction.CommentCursor{
		CreatedAt: createdAt,
		CommentID: payload.CommentID,
	}, nil
}

// encodeCommentCursor 把当前页最后一条评论的排序字段编码成下一页游标。
func encodeCommentCursor(cursor *domaininteraction.CommentCursor) string {
	if cursor == nil || cursor.CommentID <= 0 || cursor.CreatedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(commentCursorPayload{
		CreatedAt: cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		CommentID: cursor.CommentID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}
