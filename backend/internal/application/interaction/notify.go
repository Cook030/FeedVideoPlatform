package applicationinteraction

import (
	"context"
	"fmt"

	domaininteraction "GCFeed/internal/domain/interaction"
	domainmessage "GCFeed/internal/domain/message"
)

// notifyLike 给视频作者发送点赞通知，作者与操作者相同或写入器缺失时跳过。
func (s *Service) notifyLike(ctx context.Context, action *domaininteraction.Action) {
	if s.messageWriter == nil || action == nil {
		return
	}
	authorID, err := s.repo.GetVideoAuthorID(ctx, action.VideoID)
	if err != nil || authorID == action.UserID {
		return
	}
	eventID := fmt.Sprintf("interaction:like:%d:%d", action.VideoID, action.UserID)
	s.createInteractionMessage(ctx, authorID, domainmessage.TypeLike, "收到点赞", "点赞了你的视频", eventID, action.UserID)
}

// notifyComment 给视频作者发送评论通知。
func (s *Service) notifyComment(ctx context.Context, comment *domaininteraction.Comment) {
	if s.messageWriter == nil || comment == nil {
		return
	}
	authorID, err := s.repo.GetVideoAuthorID(ctx, comment.VideoID)
	if err != nil || authorID == comment.UserID {
		return
	}
	eventID := fmt.Sprintf("interaction:comment:%d", comment.ID)
	s.createInteractionMessage(ctx, authorID, domainmessage.TypeComment, "收到评论", comment.Content, eventID, comment.UserID)
}

// createInteractionMessage 组装并写入站内消息，优先携带互动者的资料。
func (s *Service) createInteractionMessage(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, actorID int64) {
	actor, _ := s.repo.GetUserProfile(ctx, actorID)
	if writer, ok := s.messageWriter.(ActorMessageWriter); ok {
		actorNickname := ""
		actorAvatarURL := ""
		if actor != nil {
			actorNickname = actor.Nickname
			actorAvatarURL = actor.AvatarURL
		}
		_, _ = writer.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, eventID, actorID, actorNickname, actorAvatarURL)
		return
	}
	actorName := fmt.Sprintf("用户 %d", actorID)
	if actor != nil && actor.Nickname != "" {
		actorName = actor.Nickname
	}
	_, _ = s.messageWriter.CreateFromEvent(ctx, userID, messageType, title, fmt.Sprintf("%s %s", actorName, content), eventID, eventID)
}
