package infracache

import (
	"context"
	"strconv"
	"strings"
	"time"

	domaininteraction "GCFeed/internal/domain/interaction"
	contract "GCFeed/internal/shared/contract"

	"github.com/redis/go-redis/v9"
)

// actionStateCache 负责点赞/收藏的快速状态写入与实时计数更新。
type actionStateCache struct {
	client redisWatchCmdable
}

// SetActionState 写入 Redis 行为状态和实时计数，供点赞收藏接口快速返回。
func (c *actionStateCache) SetActionState(ctx context.Context, userID int64, videoID int64, actionType string, active bool, idempotencyKey string, initialStat *domaininteraction.VideoStat) (*contract.ActionStateResult, error) {
	actionType, err := domaininteraction.NormalizeActionType(actionType)
	if err != nil {
		return nil, err
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	actionKey := interactionActionKey(userID, videoID, actionType)
	counterBaseKey := interactionStatCounterBaseKey(videoID)
	counterShardKey := interactionStatCounterShardKey(videoID, interactionStatCounterShardIndex(userID))
	jsonKey := feedStatKey(videoID)
	targetStatus := domaininteraction.ActionStatusCanceled
	if active {
		targetStatus = domaininteraction.ActionStatusActive
	}

	var result *contract.ActionStateResult
	err = c.client.Watch(ctx, func(tx *redis.Tx) error {
		values, err := tx.HGetAll(ctx, actionKey).Result()
		if err != nil {
			return err
		}

		storedStatus, _ := strconv.Atoi(values["status"])
		storedIDKey := values["idempotency_key"]
		effectiveActive := active
		effectiveStatus := targetStatus
		delta := 0
		if storedIDKey == idempotencyKey && idempotencyKey != "" {
			effectiveActive = storedStatus == domaininteraction.ActionStatusActive
			effectiveStatus = storedStatus
			delta = 0
		} else {
			delta = domaininteraction.ResolveActionDelta(storedStatus, active)
		}

		baseStat := actionStatBaseInit(videoID, initialStat)

		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.HSet(ctx, actionKey, map[string]any{
				"status":          effectiveStatus,
				"idempotency_key": idempotencyKey,
				"updated_at":      time.Now().UTC().Format(time.RFC3339Nano),
			})
			pipe.Expire(ctx, actionKey, actionStateTTL)
			queueActionStatBaseInit(ctx, pipe, counterBaseKey, baseStat)
			pipe.Expire(ctx, counterBaseKey, actionStatTTL)
			if delta != 0 {
				pipe.HIncrBy(ctx, counterShardKey, interactionStatField(actionType), int64(delta))
			}
			pipe.Expire(ctx, counterShardKey, actionStatTTL)
			return nil
		})
		if err != nil {
			return err
		}

		result = &contract.ActionStateResult{
			VideoID:        videoID,
			ActionType:     actionType,
			Active:         effectiveActive,
			Delta:          delta,
			IdempotencyKey: idempotencyKey,
		}
		return nil
	}, actionKey)
	if err != nil {
		return nil, err
	}

	stat, err := actionStat(ctx, c.client, counterBaseKey, interactionStatCounterShardKeys(videoID), jsonKey, videoID, initialStat)
	if err != nil {
		return nil, err
	}
	result.LikeCount = stat.LikeCount
	result.FavoriteCount = stat.FavoriteCount
	_ = setActionStatJSON(ctx, c.client, jsonKey, stat)
	return result, nil
}
