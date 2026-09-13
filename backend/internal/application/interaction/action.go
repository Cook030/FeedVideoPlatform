package applicationinteraction

import (
	"context"
	"errors"
	"strings"
	"time"

	domaininteraction "GCFeed/internal/domain/interaction"
)

// Like 设置用户对视频的点赞状态为有效。
func (s *Service) Like(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeLike, true, idempotencyKey)
}

// Unlike 设置用户对视频的点赞状态为取消。
func (s *Service) Unlike(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeLike, false, idempotencyKey)
}

// Favorite 设置用户对视频的收藏状态为有效。
func (s *Service) Favorite(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeFavorite, true, idempotencyKey)
}

// Unfavorite 设置用户对视频的收藏状态为取消。
func (s *Service) Unfavorite(ctx context.Context, userID int64, videoID int64, idempotencyKey string) (*ActionResult, error) {
	return s.setAction(ctx, userID, videoID, domaininteraction.ActionTypeFavorite, false, idempotencyKey)
}

// setAction 统一处理点赞和收藏状态变更，actionType 区分点赞或收藏，active 表示目标状态。
func (s *Service) setAction(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*ActionResult, error) {
	if userID <= 0 {
		return nil, domaininteraction.ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, domaininteraction.ErrInvalidVideoID
	}
	if len(strings.TrimSpace(idempotencyKey)) > domaininteraction.MaxIdempotencyKeyLength {
		return nil, domaininteraction.ErrIdempotencyKeyTooLong
	}

	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, err
	}
	if s.actionStateStore != nil && s.actionPublisher != nil {
		return s.setActionAsync(ctx, userID, videoID, actionType, active, idempotencyKey)
	}

	return s.setActionSync(ctx, userID, videoID, actionType, active, idempotencyKey)
}

func (s *Service) setActionAsync(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*ActionResult, error) {
	initialStat, err := s.repo.GetVideoStat(ctx, videoID)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrVideoNotFound) {
			return nil, domaininteraction.ErrVideoNotFound
		}
		return nil, ErrUpdateInteractionFailed
	}

	state, err := s.actionStateStore.SetActionState(ctx, userID, videoID, actionType, active, idempotencyKey, initialStat)
	if err != nil {
		return nil, ErrUpdateInteractionFailed
	}

	if state.Delta != 0 {
		event := NewActionChangedEvent(userID, videoID, actionType, active, idempotencyKey)
		if err := s.actionPublisher.PublishActionChanged(ctx, event); err != nil {
			return s.setActionSync(ctx, userID, videoID, actionType, active, idempotencyKey)
		}
		s.recordActionHotScore(ctx, state.VideoID, state.ActionType, state.Delta)
		if state.ActionType == domaininteraction.ActionTypeLike && state.Active && state.Delta > 0 {
			s.notifyLike(ctx, &domaininteraction.Action{
				UserID:     userID,
				VideoID:    state.VideoID,
				ActionType: state.ActionType,
				Status:     domaininteraction.ActionStatusActive,
			})
		}
	}

	return &ActionResult{
		VideoID:       state.VideoID,
		ActionType:    state.ActionType,
		Active:        state.Active,
		LikeCount:     state.LikeCount,
		FavoriteCount: state.FavoriteCount,
	}, nil
}

func (s *Service) setActionSync(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string) (*ActionResult, error) {
	action, count, delta, err := s.repo.SetAction(ctx, userID, videoID, actionType, active, idempotencyKey)
	if err != nil {
		if errors.Is(err, domaininteraction.ErrVideoNotFound) {
			return nil, domaininteraction.ErrVideoNotFound
		}
		return nil, ErrUpdateInteractionFailed
	}

	result := &ActionResult{
		VideoID:    action.VideoID,
		ActionType: action.ActionType,
		Active:     action.Active(),
	}
	if action.ActionType == domaininteraction.ActionTypeLike {
		result.LikeCount = count
	} else {
		result.FavoriteCount = count
	}
	s.recordActionHotScore(ctx, action.VideoID, action.ActionType, delta)
	if action.ActionType == domaininteraction.ActionTypeLike && action.Active() && delta > 0 {
		s.notifyLike(ctx, action)
	}
	return result, nil
}

func (s *Service) recordActionHotScore(ctx context.Context, videoID int64, actionType string, delta int) {
	recordActionHotScore(ctx, s.hotScoreRecorder, videoID, actionType, delta)
}

func (s *Service) recordHotScore(ctx context.Context, videoID int64, scoreDelta int) {
	recordHotScore(ctx, s.hotScoreRecorder, videoID, scoreDelta)
}

func recordActionHotScore(ctx context.Context, recorder HotScoreRecorder, videoID int64, actionType string, delta int) {
	if actionType == domaininteraction.ActionTypeLike {
		recordHotScore(ctx, recorder, videoID, delta*hotScoreLikeWeight)
		return
	}
	recordHotScore(ctx, recorder, videoID, delta*hotScoreFavoriteWeight)
}

func recordHotScore(ctx context.Context, recorder HotScoreRecorder, videoID int64, scoreDelta int) {
	if recorder == nil || scoreDelta == 0 {
		return
	}
	_ = recorder.AddHotScore(ctx, videoID, scoreDelta, time.Now())
}
