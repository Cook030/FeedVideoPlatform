package interfaceshttprecommendation

import (
	"time"

	applicationrecommendation "GCFeed/internal/application/recommendation"
)

type candidateRequest struct {
	UserID    int64  `json:"user_id"`
	Scene     string `json:"scene"`
	RequestID string `json:"request_id"`
	Cursor    string `json:"cursor"`
	Limit     *int   `json:"limit"`
}

type candidateResponse struct {
	UserID     int64                   `json:"user_id"`
	Scene      string                  `json:"scene"`
	RequestID  string                  `json:"request_id,omitempty"`
	Candidates []candidateItemResponse `json:"candidates"`
	NextCursor string                  `json:"next_cursor"`
	HasMore    bool                    `json:"has_more"`
}

type candidateItemResponse struct {
	VideoID        int64     `json:"video_id"`
	AuthorID       int64     `json:"author_id"`
	RankScore      float64   `json:"rank_score"`
	Similarity     float64   `json:"similarity"`
	HotScore       int       `json:"hot_score"`
	FreshnessScore float64   `json:"freshness_score"`
	Reason         string    `json:"reason"`
	PublishedAt    time.Time `json:"published_at"`
}

type exposuresRequest struct {
	UserID    int64   `json:"user_id"`
	Scene     string  `json:"scene"`
	RequestID string  `json:"request_id"`
	VideoIDs  []int64 `json:"video_ids"`
}

type exposureDecisionsRequest struct {
	UserID    int64   `json:"user_id"`
	Scene     string  `json:"scene"`
	RequestID string  `json:"request_id"`
	VideoIDs  []int64 `json:"video_ids"`
}

type exposureDecisionsResponse struct {
	UserID    int64                          `json:"user_id"`
	Scene     string                         `json:"scene"`
	RequestID string                         `json:"request_id,omitempty"`
	Decisions []exposureDecisionItemResponse `json:"decisions"`
}

type exposureDecisionItemResponse struct {
	VideoID       int64      `json:"video_id"`
	Allowed       bool       `json:"allowed"`
	Reason        string     `json:"reason"`
	LastExposedAt *time.Time `json:"last_exposed_at,omitempty"`
}

type exposuresResponse struct {
	Exposures []exposureItemResponse `json:"exposures"`
}

type exposureItemResponse struct {
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	FirstExposedAt time.Time `json:"first_exposed_at"`
	LastExposedAt  time.Time `json:"last_exposed_at"`
	ExposureCount  int       `json:"exposure_count"`
	LastScene      string    `json:"last_scene"`
}

// candidateResponseFromResult 把推荐候选结果转换为 HTTP 响应。
func candidateResponseFromResult(result *applicationrecommendation.CandidateResult) candidateResponse {
	items := make([]candidateItemResponse, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		items = append(items, candidateItemResponse{
			VideoID:        candidate.VideoID,
			AuthorID:       candidate.AuthorID,
			RankScore:      candidate.RankScore,
			Similarity:     candidate.Similarity,
			HotScore:       candidate.HotScore,
			FreshnessScore: candidate.FreshnessScore,
			Reason:         candidate.Reason,
			PublishedAt:    candidate.PublishedAt,
		})
	}
	return candidateResponse{
		UserID:     result.UserID,
		Scene:      result.Scene,
		RequestID:  result.RequestID,
		Candidates: items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// exposuresResponseFromResult 把曝光写入结果转换为 HTTP 响应。
func exposuresResponseFromResult(result *applicationrecommendation.ExposureResult) exposuresResponse {
	items := make([]exposureItemResponse, 0, len(result.Exposures))
	for _, exposure := range result.Exposures {
		items = append(items, exposureItemResponse{
			UserID:         exposure.UserID,
			VideoID:        exposure.VideoID,
			FirstExposedAt: exposure.FirstExposedAt,
			LastExposedAt:  exposure.LastExposedAt,
			ExposureCount:  exposure.ExposureCount,
			LastScene:      exposure.LastScene,
		})
	}
	return exposuresResponse{Exposures: items}
}

// exposureDecisionsResponseFromResult 把曝光决策结果转换为 HTTP 响应。
func exposureDecisionsResponseFromResult(result *applicationrecommendation.ExposureDecisionResult) exposureDecisionsResponse {
	items := make([]exposureDecisionItemResponse, 0, len(result.Decisions))
	for _, decision := range result.Decisions {
		items = append(items, exposureDecisionItemResponse{
			VideoID:       decision.VideoID,
			Allowed:       decision.Allowed,
			Reason:        decision.Reason,
			LastExposedAt: decision.LastExposedAt,
		})
	}
	return exposureDecisionsResponse{
		UserID:    result.UserID,
		Scene:     result.Scene,
		RequestID: result.RequestID,
		Decisions: items,
	}
}
