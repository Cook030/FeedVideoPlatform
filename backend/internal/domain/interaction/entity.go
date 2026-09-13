package domaininteraction

import (
	domainkernel "GCFeed/internal/domain/kernel"
	"strings"
	"time"
)

const (
	ActionTypeLike     = "LIKE"
	ActionTypeFavorite = "FAVORITE"

	// ActionStatusUnset 表示存储中还没有该行为的记录。
	ActionStatusUnset    = 0
	ActionStatusActive   = 1
	ActionStatusCanceled = 2

	CommentStatusNormal  = 1
	CommentStatusDeleted = 2

	MaxCommentContentLength = 1000
	MaxIdempotencyKeyLength = 128
	MaxLimit                = 100
)

// Action 表示用户对视频的一类互动状态，例如点赞或收藏。
type Action struct {
	ID             int64
	UserID         int64
	VideoID        int64
	ActionType     string
	Status         int
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Comment 表示视频评论，包含评论者展示信息和软删除状态。
type Comment struct {
	ID             int64
	VideoID        int64
	UserID         int64
	UserNickname   string
	UserAvatarURL  string
	Content        string
	Status         int
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CommentCursor 保存评论列表分页的排序字段。
type CommentCursor struct {
	CreatedAt time.Time
	CommentID int64
}

// VideoStat 保存互动模块需要的视频统计快照。
type VideoStat struct {
	VideoID       int64
	LikeCount     int
	CommentCount  int
	FavoriteCount int
}

// UserProfile 保存互动消息需要展示的用户资料。
type UserProfile struct {
	ID        int64
	Nickname  string
	AvatarURL string
}

// NormalizeActionType 统一行为类型大小写，避免外层传入 like、LIKE 等不同写法。
func NormalizeActionType(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value != ActionTypeLike && value != ActionTypeFavorite {
		return "", ErrInvalidActionType
	}
	return value, nil
}

// StatusFromActive 把接口目标状态转换为存储状态枚举。
func StatusFromActive(active bool) int {
	if active {
		return ActionStatusActive
	}
	return ActionStatusCanceled
}

// ResolveActionDelta 计算点赞/收藏状态迁移引起的计数增量。
//
// current 为已存状态（ActionStatusUnset 表示尚无记录），targetActive 为目标激活状态。
// 数据库与 Redis 两条写入路径都必须复用本函数，保证计数口径完全一致。
func ResolveActionDelta(current int, targetActive bool) int {
	if current == ActionStatusUnset {
		if targetActive {
			return 1
		}
		return 0
	}
	if current == StatusFromActive(targetActive) {
		return 0
	}
	if targetActive {
		return 1
	}
	return -1
}

// CanDeleteComment 判断操作者是否有权删除评论：评论作者、视频作者或管理员。
func CanDeleteComment(actorID int64, commentOwnerID int64, videoAuthorID int64, role string) bool {
	return actorID == commentOwnerID || actorID == videoAuthorID || role == domainkernel.RoleAdmin
}

// NewComment 创建评论领域对象，负责校验视频、用户、内容和幂等键。
func NewComment(videoID int64, userID int64, content string, idempotencyKey string) (*Comment, error) {
	if videoID <= 0 {
		return nil, ErrInvalidVideoID
	}
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}

	content = strings.TrimSpace(content)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if content == "" {
		return nil, ErrEmptyCommentContent
	}
	if len(content) > MaxCommentContentLength {
		return nil, ErrCommentContentTooLong
	}
	if len(idempotencyKey) > MaxIdempotencyKeyLength {
		return nil, ErrIdempotencyKeyTooLong
	}

	// 新评论默认处于正常状态，删除时再切换为 CommentStatusDeleted。
	return &Comment{
		VideoID:        videoID,
		UserID:         userID,
		Content:        content,
		Status:         CommentStatusNormal,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// RestoreAction 从数据库记录恢复互动行为，供仓储层返回领域对象。
func RestoreAction(id int64, userID int64, videoID int64, actionType string, status int, idempotencyKey string, createdAt time.Time, updatedAt time.Time) *Action {
	actionType, _ = NormalizeActionType(actionType)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = ActionStatusActive
	}

	return &Action{
		ID:             id,
		UserID:         userID,
		VideoID:        videoID,
		ActionType:     actionType,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

// RestoreComment 从数据库查询结果恢复评论对象，并清洗展示字段。
func RestoreComment(id int64, videoID int64, userID int64, userNickname string, userAvatarURL string, content string, status int, idempotencyKey string, createdAt time.Time, updatedAt time.Time) *Comment {
	content = strings.TrimSpace(content)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if status == 0 {
		status = CommentStatusNormal
	}

	return &Comment{
		ID:             id,
		VideoID:        videoID,
		UserID:         userID,
		UserNickname:   strings.TrimSpace(userNickname),
		UserAvatarURL:  strings.TrimSpace(userAvatarURL),
		Content:        content,
		Status:         status,
		IdempotencyKey: idempotencyKey,
		CreatedAt:      createdAt,
		UpdatedAt:      updatedAt,
	}
}

// Active 判断点赞或收藏当前是否处于有效状态。
func (a *Action) Active() bool {
	return a.Status == ActionStatusActive
}

// Deleted 判断评论是否已经被软删除。
func (c *Comment) Deleted() bool {
	return c.Status == CommentStatusDeleted
}
