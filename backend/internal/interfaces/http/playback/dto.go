package interfaceshttpplayback

import (
	"time"

	applicationplayback "GCFeed/internal/application/playback"
)

type playbackConfigResponse struct {
	ID           int64     `json:"id"`
	Platform     string    `json:"platform"`
	NetworkType  string    `json:"network_type"`
	PreloadCount int       `json:"preload_count"`
	BufferMs     int       `json:"buffer_ms"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type preloadVideoResponse struct {
	VideoID  int64  `json:"video_id"`
	MediaURL string `json:"media_url"`
	CoverURL string `json:"cover_url"`
}

type preloadVideosResponse struct {
	Items []preloadVideoResponse `json:"items"`
}

type createQoSReportRequest struct {
	UserID       int64 `json:"user_id,omitempty"`
	VideoID      int64 `json:"video_id"`
	FirstFrameMs *int  `json:"first_frame_ms,omitempty"`
	StutterCount int   `json:"stutter_count"`
	WatchMs      int   `json:"watch_ms"`
}

type qosReportResponse struct {
	ID           int64     `json:"id"`
	UserID       int64     `json:"user_id"`
	VideoID      int64     `json:"video_id"`
	FirstFrameMs *int      `json:"first_frame_ms,omitempty"`
	StutterCount int       `json:"stutter_count"`
	WatchMs      int       `json:"watch_ms"`
	CreatedAt    time.Time `json:"created_at"`
}

// configResponseFromResult 把播放配置结果转换为 HTTP 响应。
func configResponseFromResult(result *applicationplayback.ConfigResult) playbackConfigResponse {
	config := result.Config
	return playbackConfigResponse{
		ID:           config.ID,
		Platform:     config.Platform,
		NetworkType:  config.NetworkType,
		PreloadCount: config.PreloadCount,
		BufferMs:     config.BufferMs,
		UpdatedAt:    config.UpdatedAt,
	}
}

// preloadResponseFromResult 把预加载列表结果转换为 HTTP 响应。
func preloadResponseFromResult(result *applicationplayback.PreloadResult) preloadVideosResponse {
	items := make([]preloadVideoResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, preloadVideoResponse{
			VideoID:  item.VideoID,
			MediaURL: item.MediaURL,
			CoverURL: item.CoverURL,
		})
	}
	return preloadVideosResponse{Items: items}
}

// qosResponseFromResult 把 QoS 上报结果转换为 HTTP 响应。
func qosResponseFromResult(result *applicationplayback.QoSReportResult) qosReportResponse {
	report := result.Report
	return qosReportResponse{
		ID:           report.ID,
		UserID:       report.UserID,
		VideoID:      report.VideoID,
		FirstFrameMs: report.FirstFrameMs,
		StutterCount: report.StutterCount,
		WatchMs:      report.WatchMs,
		CreatedAt:    report.CreatedAt,
	}
}
