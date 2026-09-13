package domainrecommendation

import (
	domainkernel "GCFeed/internal/domain/kernel"
	"math"
	"strings"
	"time"
)

const MaxLimit = 100
const MaxSceneLength = 32
const MaxRequestIDLength = 64
const RecentExposureWindow = 7 * 24 * time.Hour

// PositiveEventWindow 是用户兴趣向量的行为回溯窗口。
const PositiveEventWindow = 30 * 24 * time.Hour

const ExposureDecisionReasonFresh = "fresh"
const ExposureDecisionReasonRecentlyExposed = "recently_exposed"

// 推荐排序权重：有用户向量时综合「相似度 / 热度 / 新鲜度」，
// 冷启动（无用户向量）时只用热度与新鲜度。
const (
	RankSimilarityWeight    = 0.70
	RankHotWeight           = 0.20
	RankFreshnessWeight     = 0.10
	RankHotOnlyWeight       = 0.65
	RankFreshnessOnlyWeight = 0.35
)

// rankHotNormalizeDivisor 是热度归一化除数。
const rankHotNormalizeDivisor = 10

// freshnessHalfLifeHours 是新鲜度衰减的基准小时数。
const freshnessHalfLifeHours = 72

// 推荐理由标签，属于对外可见的排序说明。
const (
	ReasonInterestMatch = "interest_match"
	ReasonHot           = "hot"
	ReasonFresh         = "fresh"
)

type CandidateRequest struct {
	UserID    int64
	Scene     string
	RequestID string
	Cursor    *Cursor
	Limit     int
}

type Cursor struct {
	RankScore   float64
	PublishedAt time.Time
	VideoID     int64
}

type Candidate struct {
	VideoID        int64
	AuthorID       int64
	RankScore      float64
	Similarity     float64
	HotScore       int
	FreshnessScore float64
	Reason         string
	PublishedAt    time.Time
}

type ExposureWrite struct {
	UserID    int64
	VideoID   int64
	Scene     string
	RequestID string
}

type ExposureDecisionRequest struct {
	UserID    int64
	Scene     string
	RequestID string
	VideoIDs  []int64
}

type Exposure struct {
	ID             int64
	UserID         int64
	VideoID        int64
	FirstExposedAt time.Time
	LastExposedAt  time.Time
	ExposureCount  int
	LastScene      string
}

type ExposureDecision struct {
	VideoID       int64
	Allowed       bool
	Reason        string
	LastExposedAt *time.Time
}

func NewCandidateRequest(userID int64, scene string, requestID string, cursor *Cursor, limit int) (*CandidateRequest, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	scene = strings.TrimSpace(strings.ToLower(scene))
	requestID = strings.TrimSpace(requestID)
	if scene == "" {
		return nil, ErrEmptyScene
	}
	if len(scene) > MaxSceneLength {
		return nil, ErrSceneTooLong
	}
	if len(requestID) > MaxRequestIDLength {
		return nil, ErrRequestIDTooLong
	}
	if limit <= 0 || limit > MaxLimit {
		return nil, ErrInvalidLimit
	}
	if cursor != nil && !cursor.Valid() {
		return nil, ErrInvalidCursor
	}
	return &CandidateRequest{
		UserID:    userID,
		Scene:     scene,
		RequestID: requestID,
		Cursor:    cursor,
		Limit:     limit,
	}, nil
}

func NewExposureDecisionRequest(userID int64, scene string, requestID string, videoIDs []int64) (*ExposureDecisionRequest, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	scene = strings.TrimSpace(strings.ToLower(scene))
	requestID = strings.TrimSpace(requestID)
	if scene == "" {
		return nil, ErrEmptyScene
	}
	if len(scene) > MaxSceneLength {
		return nil, ErrSceneTooLong
	}
	if len(requestID) > MaxRequestIDLength {
		return nil, ErrRequestIDTooLong
	}
	deduped := make([]int64, 0, len(videoIDs))
	seen := map[int64]struct{}{}
	for _, videoID := range videoIDs {
		if videoID <= 0 {
			return nil, ErrInvalidVideoID
		}
		if _, exists := seen[videoID]; exists {
			continue
		}
		seen[videoID] = struct{}{}
		deduped = append(deduped, videoID)
	}
	return &ExposureDecisionRequest{
		UserID:    userID,
		Scene:     scene,
		RequestID: requestID,
		VideoIDs:  deduped,
	}, nil
}

func NewExposureWrite(userID int64, videoID int64, scene string, requestID string) (*ExposureWrite, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if videoID <= 0 {
		return nil, ErrInvalidVideoID
	}
	scene = strings.TrimSpace(strings.ToLower(scene))
	requestID = strings.TrimSpace(requestID)
	if scene == "" {
		return nil, ErrEmptyScene
	}
	if len(scene) > MaxSceneLength {
		return nil, ErrSceneTooLong
	}
	if len(requestID) > MaxRequestIDLength {
		return nil, ErrRequestIDTooLong
	}
	return &ExposureWrite{
		UserID:    userID,
		VideoID:   videoID,
		Scene:     scene,
		RequestID: requestID,
	}, nil
}

func RestoreCandidate(videoID int64, authorID int64, rankScore float64, similarity float64, hotScore int, freshnessScore float64, reason string, publishedAt time.Time) *Candidate {
	return &Candidate{
		VideoID:        videoID,
		AuthorID:       authorID,
		RankScore:      rankScore,
		Similarity:     similarity,
		HotScore:       hotScore,
		FreshnessScore: freshnessScore,
		Reason:         strings.TrimSpace(reason),
		PublishedAt:    publishedAt,
	}
}

func RestoreExposure(id int64, userID int64, videoID int64, firstExposedAt time.Time, lastExposedAt time.Time, exposureCount int, lastScene string) *Exposure {
	return &Exposure{
		ID:             id,
		UserID:         userID,
		VideoID:        videoID,
		FirstExposedAt: firstExposedAt,
		LastExposedAt:  lastExposedAt,
		ExposureCount:  exposureCount,
		LastScene:      strings.TrimSpace(lastScene),
	}
}

func RestoreExposureDecision(videoID int64, allowed bool, reason string, lastExposedAt *time.Time) *ExposureDecision {
	var exposedAt *time.Time
	if lastExposedAt != nil {
		value := *lastExposedAt
		exposedAt = &value
	}
	return &ExposureDecision{
		VideoID:       videoID,
		Allowed:       allowed,
		Reason:        strings.TrimSpace(reason),
		LastExposedAt: exposedAt,
	}
}

func (c *Cursor) Valid() bool {
	return c != nil && c.VideoID > 0 && !c.PublishedAt.IsZero() && !math.IsNaN(c.RankScore) && !math.IsInf(c.RankScore, 0)
}

// IsPositiveEventType 判断某类行为是否计入用户兴趣向量。
// 曝光只说明"推给用户看过"，不代表用户感兴趣，因此不参与兴趣计算。
func IsPositiveEventType(eventType string) bool {
	switch eventType {
	case domainkernel.EventTypePlay, domainkernel.EventTypeComplete:
		return true
	default:
		return false
	}
}

// EventWeight 计算单条观看行为对兴趣向量的贡献权重：
// 看完最重，播放按观看时长线性加权，其余行为按基准权重处理。
func EventWeight(eventType string, watchMs int, completed bool) float64 {
	switch eventType {
	case domainkernel.EventTypeComplete:
		return 3
	case domainkernel.EventTypePlay:
		weight := 1 + float64(watchMs)/30000
		if weight > 2 {
			weight = 2
		}
		if completed {
			weight += 1
		}
		return weight
	default:
		return 1
	}
}

// RankScore 计算候选排序分：热度先做 log1p 归一化，
// hasUserVector 区分个性化与冷启动两种口径。
func RankScore(similarity float64, hotScore int, freshness float64, hasUserVector bool) float64 {
	normalizedHot := hotScore
	if normalizedHot < 0 {
		normalizedHot = 0
	}
	hot := math.Log1p(float64(normalizedHot)) / rankHotNormalizeDivisor
	if hasUserVector {
		return similarity*RankSimilarityWeight + hot*RankHotWeight + freshness*RankFreshnessWeight
	}
	return hot*RankHotOnlyWeight + freshness*RankFreshnessOnlyWeight
}

// FreshnessScore 计算新鲜度衰减：1 / (1 + hours/72)。
func FreshnessScore(now time.Time, publishedAt time.Time) float64 {
	if publishedAt.IsZero() {
		return 0
	}
	hours := now.Sub(publishedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return 1 / (1 + hours/freshnessHalfLifeHours)
}

// Reason 返回推荐理由标签。
func Reason(hasUserVector bool, similarity float64, hotScore int) string {
	if hasUserVector && similarity > 0.05 {
		return ReasonInterestMatch
	}
	if hotScore > 0 {
		return ReasonHot
	}
	return ReasonFresh
}
