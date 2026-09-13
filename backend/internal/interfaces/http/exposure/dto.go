package interfaceshttpexposure

import (
	"time"

	applicationexposure "GCFeed/internal/application/exposure"
)

// createViewEventRequest 是观看行为上报请求体。
type createViewEventRequest struct {
	VideoID   int64  `json:"video_id"`
	Scene     string `json:"scene"`
	RequestID string `json:"request_id"`
	EventType string `json:"event_type"`
	WatchMs   int    `json:"watch_ms"`
	Completed bool   `json:"completed"`
}

// viewEventResponse 是观看行为流水响应。
type viewEventResponse struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VideoID   int64     `json:"video_id"`
	Scene     string    `json:"scene"`
	RequestID string    `json:"request_id,omitempty"`
	EventType string    `json:"event_type"`
	WatchMs   int       `json:"watch_ms"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

// exposureResponse 是曝光聚合响应，exposed 事件会返回该对象。
type exposureResponse struct {
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	FirstExposedAt time.Time `json:"first_exposed_at"`
	LastExposedAt  time.Time `json:"last_exposed_at"`
	ExposureCount  int       `json:"exposure_count"`
	LastScene      string    `json:"last_scene"`
}

// createViewEventResponse 是上报后的完整响应。
type createViewEventResponse struct {
	Event    viewEventResponse `json:"event"`
	Exposure *exposureResponse `json:"exposure,omitempty"`
}

// responseFromResult 把观看行为上报结果转换为 HTTP 响应。
func responseFromResult(result *applicationexposure.RecordViewEventResult) createViewEventResponse {
	response := createViewEventResponse{
		Event: viewEventResponse{
			ID:        result.Event.ID,
			UserID:    result.Event.UserID,
			VideoID:   result.Event.VideoID,
			Scene:     result.Event.Scene,
			RequestID: result.Event.RequestID,
			EventType: result.Event.EventType,
			WatchMs:   result.Event.WatchMs,
			Completed: result.Event.Completed,
			CreatedAt: result.Event.CreatedAt,
		},
	}
	if result.Exposure != nil {
		response.Exposure = &exposureResponse{
			UserID:         result.Exposure.UserID,
			VideoID:        result.Exposure.VideoID,
			FirstExposedAt: result.Exposure.FirstExposedAt,
			LastExposedAt:  result.Exposure.LastExposedAt,
			ExposureCount:  result.Exposure.ExposureCount,
			LastScene:      result.Exposure.LastScene,
		}
	}
	return response
}
