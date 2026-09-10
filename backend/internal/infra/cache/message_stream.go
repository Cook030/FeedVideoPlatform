package infracache

import (
	applicationmessage "GCFeed/internal/application/message"
	infraconfig "GCFeed/internal/infra/config"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// messageStreamChannelPrefix 是接收用户专属的 Pub/Sub 频道前缀，后缀为 userID。
const messageStreamChannelPrefix = "gcfeed:msg:user:"

// streamEnvelope 是频道内传输的统一信封，Type 决定客户端如何处理 Data。
type streamEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// MessageStream 基于 Redis Pub/Sub 在多个 API 实例之间扇出消息事件。
// 发送方把事件发布到接收用户频道，每个实例订阅全部频道后只投递本机持有的连接。
type MessageStream struct {
	client *redis.Client
	ready  atomic.Bool
}

// Ready 表示 Redis 订阅当前已经建立。
func (s *MessageStream) Ready() bool {
	return s != nil && s.ready.Load()
}

// NewMessageStream 创建消息实时通道。这里使用独立的 Redis 客户端，
// 避免复用 Feed 缓存客户端上偏短的读写超时影响 Pub/Sub 长连接。
func NewMessageStream(cfg infraconfig.RedisConfig) *MessageStream {
	return &MessageStream{
		client: redis.NewClient(&redis.Options{
			Addr:         cfg.Addr,
			Password:     cfg.Password,
			DB:           cfg.DB,
			DialTimeout:  2 * time.Second,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 5 * time.Second,
		}),
	}
}

// Close 释放底层 Redis 连接。
func (s *MessageStream) Close() error {
	if s == nil || s.client == nil {
		return nil
	}
	return s.client.Close()
}

// Notify 实现 applicationmessage.Notifier，把消息变更发布到接收用户频道。
func (s *MessageStream) Notify(ctx context.Context, notification applicationmessage.Notification) error {
	if s == nil || s.client == nil || notification.UserID <= 0 {
		return nil
	}
	data := buildNotificationData(notification)
	if len(data) == 0 {
		return nil
	}
	payload, err := json.Marshal(streamEnvelope{Type: notification.Type, Data: data})
	if err != nil {
		return err
	}
	return s.client.Publish(ctx, messageStreamChannel(notification.UserID), payload).Err()
}

// Run 订阅所有用户频道并把事件交给 onEvent 处理，应在独立 goroutine 中运行。
func (s *MessageStream) Run(ctx context.Context, onEvent func(userID int64, eventType string, data []byte)) error {
	if s == nil || s.client == nil {
		return nil
	}
	for ctx.Err() == nil {
		_ = s.runOnce(ctx, onEvent)
		if ctx.Err() != nil {
			break
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
		case <-timer.C:
		}
	}
	return ctx.Err()
}

func (s *MessageStream) runOnce(ctx context.Context, onEvent func(userID int64, eventType string, data []byte)) error {
	pubsub := s.client.PSubscribe(ctx, messageStreamChannelPrefix+"*")
	defer func() {
		s.ready.Store(false)
		_ = pubsub.Close()
	}()

	// 等待订阅建立，避免启动初期的事件被丢弃。
	if _, err := pubsub.Receive(ctx); err != nil {
		return err
	}
	s.ready.Store(true)

	channel := pubsub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case message, ok := <-channel:
			if !ok {
				return nil
			}
			userID, userOK := parseMessageStreamUserID(message.Channel)
			if !userOK {
				continue
			}
			var envelope streamEnvelope
			if err := json.Unmarshal([]byte(message.Payload), &envelope); err != nil {
				continue
			}
			onEvent(userID, envelope.Type, envelope.Data)
		}
	}
}

func messageStreamChannel(userID int64) string {
	return messageStreamChannelPrefix + strconv.FormatInt(userID, 10)
}

func parseMessageStreamUserID(channel string) (int64, bool) {
	raw := strings.TrimPrefix(channel, messageStreamChannelPrefix)
	if raw == channel || raw == "" {
		return 0, false
	}
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return 0, false
	}
	return userID, true
}

// buildNotificationData 把应用层通知转换成 SSE 的 data 负载。
func buildNotificationData(notification applicationmessage.Notification) json.RawMessage {
	if notification.Type == applicationmessage.NotificationTypeMessage {
		if notification.Message == nil {
			return nil
		}
		message := notification.Message
		data, err := json.Marshal(map[string]any{
			"id":               message.ID,
			"user_id":          message.UserID,
			"type":             message.Type,
			"title":            message.Title,
			"content":          message.Content,
			"event_id":         message.EventID,
			"actor_id":         message.ActorID,
			"actor_nickname":   message.ActorNickname,
			"actor_avatar_url": message.ActorAvatarURL,
			"is_read":          message.IsRead,
			"created_at":       message.CreatedAt,
			"unread_count":     notification.UnreadCount,
		})
		if err != nil {
			return nil
		}
		return data
	}

	data, err := json.Marshal(map[string]int{"unread_count": notification.UnreadCount})
	if err != nil {
		return nil
	}
	return data
}
