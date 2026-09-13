package contract

import domainmessage "GCFeed/internal/domain/message"

// UnreadCountUnknown 表示未读数获取失败，客户端应回退到 REST 查询。
const UnreadCountUnknown = -1

// 消息实时通知类型。
const (
	NotificationTypeMessage = "message"
	NotificationTypeUnread  = "unread"
)

// Notification 描述一次需要实时下发的消息变更。
type Notification struct {
	UserID      int64
	Type        string
	Message     *domainmessage.Message
	UnreadCount int
}
