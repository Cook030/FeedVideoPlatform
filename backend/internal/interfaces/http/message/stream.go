package interfaceshttpmessage

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	applicationmessage "GCFeed/internal/application/message"
	inforealtime "GCFeed/internal/infra/realtime"
	sharedhttputil "GCFeed/internal/shared/httputil"

	"github.com/gin-gonic/gin"
)

// StreamHandler 负责单条 SSE 连接的生命周期：鉴权、首帧与事件写回。
// 连接注册表与跨实例扇出由 infra/realtime 提供。
type StreamHandler struct {
	service *applicationmessage.Service
	hub     *inforealtime.Hub
	stream  *inforealtime.MessageStream
}

// NewStreamHandler 注入消息服务、连接注册表与实时通道。
func NewStreamHandler(service *applicationmessage.Service, hub *inforealtime.Hub, stream *inforealtime.MessageStream) *StreamHandler {
	return &StreamHandler{service: service, hub: hub, stream: stream}
}

// Ticket 使用现有 Bearer JWT 换取一次性 SSE ticket。
func (h *StreamHandler) Ticket(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok || h.stream == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "message stream is unavailable"})
		return
	}
	ticket, err := h.stream.IssueTicket(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "message stream is unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ticket": ticket})
}

// Stream 建立 SSE 长连接，推送新消息和未读数变更事件。
func (h *StreamHandler) Stream(c *gin.Context) {
	if h.stream == nil || !h.stream.Ready() {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "message stream is unavailable"})
		return
	}
	userID, err := h.stream.ConsumeTicket(c.Request.Context(), c.Query("ticket"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "invalid access token"})
		return
	}

	// SSE 必须关闭反向代理缓冲并保持连接长活。
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	c.Writer.Flush()

	subscription := h.hub.Subscribe(userID)
	defer subscription.Close()

	// 首帧 ready 携带初始未读数，前端同时用它确认鉴权与链路已就绪。
	if !writeSSE(c, "ready", h.readyPayload(c, userID)) {
		return
	}

	ticker := time.NewTicker(inforealtime.HeartbeatInterval)
	defer ticker.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case event, open := <-subscription.Events():
			if !open {
				return
			}
			if !writeSSE(c, event.Type, event.Data) {
				return
			}
		case <-ticker.C:
			if _, err := fmt.Fprint(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
		}
	}
}

func (h *StreamHandler) readyPayload(c *gin.Context, userID int64) []byte {
	unreadCount := applicationmessage.UnreadCountUnknown
	if h.service != nil {
		if stat, err := h.service.CountUnread(c.Request.Context(), userID); err == nil && stat != nil {
			unreadCount = stat.UnreadCount
		}
	}
	data, err := json.Marshal(map[string]int{"unread_count": unreadCount})
	if err != nil {
		return []byte(`{"unread_count":-1}`)
	}
	return data
}

// writeSSE 写入一个完整的 SSE 事件并立即 flush，返回 false 表示连接已不可写。
func writeSSE(c *gin.Context, eventType string, data []byte) bool {
	if _, err := fmt.Fprintf(c.Writer, "event: %s\n", eventType); err != nil {
		return false
	}
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}
