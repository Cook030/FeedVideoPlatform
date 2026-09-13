package interfaceshttpmessage

import (
	"time"

	applicationmessage "GCFeed/internal/application/message"
	domainmessage "GCFeed/internal/domain/message"
)

type createMessageRequest struct {
	UserID         int64  `json:"user_id"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	Content        string `json:"content"`
	EventID        string `json:"event_id"`
	ActorID        int64  `json:"actor_id"`
	ActorNickname  string `json:"actor_nickname"`
	ActorAvatarURL string `json:"actor_avatar_url"`
}

type markReadRequest struct {
	MessageIDs []int64 `json:"message_ids"`
}

type messageResponse struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	Type           string     `json:"type"`
	Title          string     `json:"title"`
	Content        string     `json:"content"`
	EventID        string     `json:"event_id,omitempty"`
	ActorID        int64      `json:"actor_id,omitempty"`
	ActorNickname  string     `json:"actor_nickname,omitempty"`
	ActorAvatarURL string     `json:"actor_avatar_url,omitempty"`
	IsRead         bool       `json:"is_read"`
	CreatedAt      time.Time  `json:"created_at"`
	ReadAt         *time.Time `json:"read_at,omitempty"`
}

type messageListResponse struct {
	Items      []messageResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

type unreadStatResponse struct {
	UnreadCount int `json:"unread_count"`
}

type markReadResponse struct {
	UpdatedCount int `json:"updated_count"`
}

// listResponseFromResult 把应用层消息列表转换为 HTTP 响应。
func listResponseFromResult(result *applicationmessage.ListResult) messageListResponse {
	items := make([]messageResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, responseFromDomain(item))
	}
	return messageListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

// responseFromDomain 把领域消息转换为 HTTP 响应。
func responseFromDomain(message *domainmessage.Message) messageResponse {
	return messageResponse{
		ID:             message.ID,
		UserID:         message.UserID,
		Type:           message.Type,
		Title:          message.Title,
		Content:        message.Content,
		EventID:        message.EventID,
		ActorID:        message.ActorID,
		ActorNickname:  message.ActorNickname,
		ActorAvatarURL: message.ActorAvatarURL,
		IsRead:         message.IsRead,
		CreatedAt:      message.CreatedAt,
		ReadAt:         message.ReadAt,
	}
}
