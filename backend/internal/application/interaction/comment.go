package applicationinteraction

import (
	"context"
	"errors"
	"strings"

	domaininteraction "GCFeed/internal/domain/interaction"
)

// CreateComment 创建评论，并通过幂等键防止客户端重试生成重复评论。
func (s *Service) CreateComment(ctx context.Context, userID int64, videoID int64, content string, idempotencyKey string) (*CreateCommentResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if len(idempotencyKey) > domaininteraction.MaxIdempotencyKeyLength {
		return nil, domaininteraction.ErrIdempotencyKeyTooLong
	}

	if idempotencyKey != "" {
		// 幂等键命中时返回已创建的评论，客户端重试可以拿到同一结果。
		comment, count, err := s.repo.FindCommentByUserAndIdempotencyKey(ctx, userID, idempotencyKey)
		if err == nil {
			return &CreateCommentResult{Comment: comment, CommentCount: count}, nil
		}
		if !errors.Is(err, domaininteraction.ErrCommentNotFound) {
			return nil, ErrLoadInteractionFailed
		}
	}

	comment, err := domaininteraction.NewComment(videoID, userID, content, idempotencyKey)
	if err != nil {
		return nil, err
	}

	created, count, delta, err := s.repo.CreateComment(ctx, comment)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrVideoNotFound) {
			return nil, domaininteraction.ErrVideoNotFound
		}
		return nil, ErrSaveInteractionFailed
	}
	s.recordHotScore(ctx, created.VideoID, delta*hotScoreCommentWeight)
	s.syncCommentCount(ctx, created.VideoID, count)
	if delta > 0 {
		s.notifyComment(ctx, created)
	}

	return &CreateCommentResult{Comment: created, CommentCount: count}, nil
}

// ListComments 使用游标分页查询评论，返回下一页游标和 has_more。
func (s *Service) ListComments(ctx context.Context, videoID int64, cursor string, limit int) (*CommentListResult, error) {
	if videoID <= 0 {
		return nil, domaininteraction.ErrInvalidVideoID
	}

	parsedCursor, err := parseCommentCursor(cursor)
	if err != nil {
		return nil, err
	}
	limit = normalizeCommentLimit(limit)

	// 多查 1 条用于判断是否还有下一页，返回给客户端时再裁掉。
	items, err := s.repo.ListComments(ctx, videoID, parsedCursor, limit+1)
	if err != nil {
		return nil, ErrLoadInteractionFailed
	}

	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		nextCursor = encodeCommentCursor(&domaininteraction.CommentCursor{
			CreatedAt: items[len(items)-1].CreatedAt,
			CommentID: items[len(items)-1].ID,
		})
	}

	return &CommentListResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// DeleteComment 删除评论并返回删除后的评论状态和视频评论数。
func (s *Service) DeleteComment(ctx context.Context, commentID int64, userID int64, role string) (*DeleteCommentResult, error) {
	if commentID <= 0 {
		return nil, domaininteraction.ErrInvalidCommentID
	}
	if userID <= 0 {
		return nil, domaininteraction.ErrInvalidUserID
	}

	comment, count, delta, err := s.repo.DeleteComment(ctx, commentID, userID, role)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrCommentNotFound) ||
			errors.Is(err, domaininteraction.ErrCommentPermissionDenied) {
			return nil, err
		}
		return nil, ErrUpdateInteractionFailed
	}
	s.recordHotScore(ctx, comment.VideoID, delta*hotScoreCommentWeight)
	s.syncCommentCount(ctx, comment.VideoID, count)

	return &DeleteCommentResult{
		CommentID:    comment.ID,
		Status:       comment.Status,
		CommentCount: count,
	}, nil
}

// syncCommentCount 把评论数写入 Feed 计数缓存，失败不影响主流程。
func (s *Service) syncCommentCount(ctx context.Context, videoID int64, commentCount int) {
	if s.statCache == nil || videoID <= 0 {
		return
	}
	stat, err := s.repo.GetVideoStat(ctx, videoID)
	if err != nil {
		stat = &domaininteraction.VideoStat{VideoID: videoID}
	}
	stat.CommentCount = commentCount
	_ = s.statCache.SetVideoStat(ctx, stat)
}
