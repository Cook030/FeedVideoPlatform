package domainrecommendation

import (
	domainexposure "GCFeed/internal/domain/exposure"
	"time"
)

// PositiveEventWindow 是用户兴趣向量的行为回溯窗口。
const PositiveEventWindow = 30 * 24 * time.Hour

// IsPositiveEventType 判断某类行为是否计入用户兴趣向量。
// 曝光只说明"推给用户看过"，不代表用户感兴趣，因此不参与兴趣计算。
func IsPositiveEventType(eventType string) bool {
	switch eventType {
	case domainexposure.EventTypePlay, domainexposure.EventTypeComplete:
		return true
	default:
		return false
	}
}

// EventWeight 计算单条观看行为对兴趣向量的贡献权重：
// 看完最重，播放按观看时长线性加权，其余行为按基准权重处理。
func EventWeight(eventType string, watchMs int, completed bool) float64 {
	switch eventType {
	case domainexposure.EventTypeComplete:
		return 3
	case domainexposure.EventTypePlay:
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
