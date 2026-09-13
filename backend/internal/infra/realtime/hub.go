package inforealtime

import (
	"sync"
	"time"

	inframetrics "GCFeed/internal/infra/metrics"
)

// SSE 调优参数：心跳间隔、单连接缓冲与单用户并发上限。
const (
	// HeartbeatInterval 是心跳间隔，用于保活并探测已断开的连接。
	HeartbeatInterval = 25 * time.Second
	// sseClientBuffer 是单连接的发送缓冲；写满说明客户端消费过慢，直接断开。
	sseClientBuffer = 16
	// sseMaxConnectionsPerUser 限制单用户并发连接数，避免恶意占用。
	sseMaxConnectionsPerUser = 5
)

// StreamEvent 是待写入 SSE 连接的事件。
type StreamEvent struct {
	Type string
	Data []byte
}

// streamClient 表示一条在途的 SSE 连接。channel 的关闭权归 Hub，订阅方只负责读取。
type streamClient struct {
	userID int64
	id     uint64
	ch     chan StreamEvent
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

func (c *streamClient) trySend(event StreamEvent) (sent bool, open bool) {
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
		ch:     make(chan StreamEvent, sseClientBuffer),
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

	event := StreamEvent{Type: eventType, Data: data}
	for _, client := range targets {
		if sent, open := client.trySend(event); !sent && open {
			// 缓冲写满说明客户端消费过慢，断开让它重连后通过 REST 补齐。
			inframetrics.ObserveSSEDispatchDropped()
			h.unregister(client)
		}
	}
}

// Subscription 表示一条已注册的 SSE 连接，由 Subscribe 创建、Close 注销。
type Subscription struct {
	hub    *Hub
	client *streamClient
}

// Subscribe 注册一条新连接；超出单用户上限时会踢掉最早的连接。
func (h *Hub) Subscribe(userID int64) *Subscription {
	return &Subscription{hub: h, client: h.register(userID)}
}

// Events 返回该连接的只读事件通道，通道关闭表示连接已被注销。
func (s *Subscription) Events() <-chan StreamEvent {
	return s.client.ch
}

// Close 注销连接，重复调用安全。
func (s *Subscription) Close() {
	if s == nil || s.hub == nil {
		return
	}
	s.hub.unregister(s.client)
}
