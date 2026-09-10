package interfaceshttpmessage

import (
	applicationmessage "GCFeed/internal/application/message"
	inframetrics "GCFeed/internal/infra/metrics"
	interfaceshttpmiddleware "GCFeed/internal/interfaces/http/middleware"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	// sseHeartbeatInterval 是心跳间隔，用于保活并探测已断开的连接。
	sseHeartbeatInterval = 25 * time.Second
	// sseClientBuffer 是单连接的发送缓冲；写满说明客户端消费过慢，直接断开。
	sseClientBuffer = 16
	// sseMaxConnectionsPerUser 限制单用户并发连接数，避免恶意占用。
	sseMaxConnectionsPerUser = 5
)

// streamEvent 是待写入 SSE 连接的事件。
type streamEvent struct {
	Type string
	Data []byte
}

// streamClient 表示一条在途的 SSE 连接。channel 的关闭权归 Hub，Handler 只负责读取。
type streamClient struct {
	userID int64
	id     uint64
	ch     chan streamEvent
	mu     sync.Mutex
	closed bool
}

func (c *streamClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.closed {
		c.closed = true
		close(c.ch)
	}

}

func (c *streamClient) trySend(event streamEvent) (sent bool, open bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false, false
	}
	select {
	case c.ch <- event:
		return true, true
	default:
		return false, true
	}
}

// Hub 保存当前实例持有的 SSE 连接，把事件投递给本机连接。
// 多实例之间由 Redis Pub/Sub 广播，每个实例只投递自己持有的连接。
type Hub struct {
	mu      sync.RWMutex
	members map[int64]map[*streamClient]struct{}
	nextID  uint64
}

// NewHub 创建连接注册表。
func NewHub() *Hub {
	return &Hub{members: make(map[int64]map[*streamClient]struct{})}
}

func (h *Hub) register(userID int64) *streamClient {
	kicked := 0
	h.mu.Lock()
	h.nextID++
	client := &streamClient{
		userID: userID,
		id:     h.nextID,
		ch:     make(chan streamEvent, sseClientBuffer),
	}
	set := h.members[userID]
	if set == nil {
		set = make(map[*streamClient]struct{})
		h.members[userID] = set
	}
	// 超出单用户连接上限时踢掉最早建立的连接，保证新连接可用。
	for len(set) >= sseMaxConnectionsPerUser {
		var victim *streamClient
		for candidate := range set {
			if victim == nil || candidate.id < victim.id {
				victim = candidate
			}
		}
		delete(set, victim)
		victim.close()
		kicked++
	}
	set[client] = struct{}{}
	h.mu.Unlock()

	// 被踢连接的 gauge 在此扣减，其 handler 退出时不会重复扣减。
	if kicked > 0 {
		inframetrics.ObserveSSEConnection(-kicked)
	}
	inframetrics.ObserveSSEConnection(1)
	return client
}

func (h *Hub) unregister(client *streamClient) {
	if client == nil {
		return
	}
	removed := false
	h.mu.Lock()
	if set := h.members[client.userID]; set != nil {
		if _, exists := set[client]; exists {
			delete(set, client)
			removed = true
		}
		if len(set) == 0 {
			delete(h.members, client.userID)
		}
	}
	h.mu.Unlock()

	client.close()
	if removed {
		inframetrics.ObserveSSEConnection(-1)
	}
}

// Close 关闭本实例所有在途连接，用于进程优雅退出时让客户端尽快重连。
func (h *Hub) Close() {
	if h == nil {
		return
	}
	h.mu.Lock()
	clients := make([]*streamClient, 0, len(h.members))
	for _, set := range h.members {
		for client := range set {
			clients = append(clients, client)
		}
	}
	h.members = make(map[int64]map[*streamClient]struct{})
	h.mu.Unlock()

	// 先清空注册表再关闭 channel，后续 handler 的 unregister 不会重复扣减指标。
	for _, client := range clients {
		client.close()
	}
	if len(clients) > 0 {
		inframetrics.ObserveSSEConnection(-len(clients))
	}
}

// OnEvent 由 Redis 订阅协程调用，只投递给本实例持有的连接。
func (h *Hub) OnEvent(userID int64, eventType string, data []byte) {
	if h == nil || userID <= 0 || len(data) == 0 {
		return
	}

	h.mu.RLock()
	set := h.members[userID]
	targets := make([]*streamClient, 0, len(set))
	for client := range set {
		targets = append(targets, client)
	}
	h.mu.RUnlock()

	event := streamEvent{Type: eventType, Data: data}
	for _, client := range targets {
		if sent, open := client.trySend(event); !sent && open {
			// 缓冲写满说明客户端消费过慢，断开让它重连后通过 REST 补齐。
			inframetrics.ObserveSSEDispatchDropped()
			h.unregister(client)
		}
	}
}

// StreamHandler 负责单条 SSE 连接的生命周期。
type StreamHandler struct {
	service *applicationmessage.Service
	hub     *Hub
	ready   func() bool
}

// NewStreamHandler 注入消息服务与连接注册表。
func NewStreamHandler(service *applicationmessage.Service, hub *Hub, ready func() bool) *StreamHandler {
	return &StreamHandler{service: service, hub: hub, ready: ready}
}

// Stream 建立 SSE 长连接，推送新消息和未读数变更事件。
func (h *StreamHandler) Stream(c *gin.Context) {
	if h.ready == nil || !h.ready() {
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"message": "message stream is unavailable"})
		return
	}
	userID, ok := userIDFromContext(c)
	if !ok {
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

	client := h.hub.register(userID)
	defer h.hub.unregister(client)

	// 首帧 ready 携带初始未读数，前端同时用它确认鉴权与链路已就绪。
	if !writeSSE(c, "ready", h.readyPayload(c, userID)) {
		return
	}

	ticker := time.NewTicker(sseHeartbeatInterval)
	defer ticker.Stop()

	ctx := c.Request.Context()
	expiresAt, ok := c.Get(interfaceshttpmiddleware.ContextTokenExpiresAtKey)
	expiresAtUnix, ok := expiresAt.(int64)
	if !ok || expiresAtUnix <= 0 {
		return
	}
	expiry := time.NewTimer(time.Until(time.Unix(expiresAtUnix, 0)))
	defer expiry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-expiry.C:
			return
		case event, open := <-client.ch:
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
