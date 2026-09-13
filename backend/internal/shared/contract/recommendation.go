package contract

import (
	"errors"

	domainrecommendation "GCFeed/internal/domain/recommendation"
)

// ErrRecommendationUnavailable 表示推荐候选加载失败。
//
// 由 application/recommendation 产生、application/feed 消费。定义在中立契约包
// 是为了避免两个 application 业务子包互相 import；错误文案与既有实现保持一致。
var ErrRecommendationUnavailable = errors.New("failed to load recommendations")

// CandidateRequest 是推荐候选查询参数。
type CandidateRequest struct {
	UserID    int64
	Scene     string
	RequestID string
	Cursor    string
	Limit     int
}

// CandidateResult 是推荐候选查询结果。
type CandidateResult struct {
	UserID     int64
	Scene      string
	RequestID  string
	Candidates []*domainrecommendation.Candidate
	NextCursor string
	HasMore    bool
}
