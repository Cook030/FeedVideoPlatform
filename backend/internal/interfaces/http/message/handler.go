package interfaceshttpmessage

import (
	applicationmessage "GCFeed/internal/application/message"
	domainmessage "GCFeed/internal/domain/message"
	sharedhttputil "GCFeed/internal/shared/httputil"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationmessage.Service
}

// New 注入消息应用服务。
func New(service *applicationmessage.Service) *Handler {
	return &Handler{service: service}
}

// List 查询当前登录用户的消息列表。
func (h *Handler) List(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, err := sharedhttputil.ParseLimit(c.Query("limit"), domainmessage.ErrInvalidLimit)
	if err != nil {
		writeMessageError(c, err)
		return
	}

	result, err := h.service.List(c.Request.Context(), userID, c.Query("cursor"), limit)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponseFromResult(result))
}

// CountUnread 查询当前登录用户未读消息数。
func (h *Handler) CountUnread(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	stat, err := h.service.CountUnread(c.Request.Context(), userID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, unreadStatResponse{UnreadCount: stat.UnreadCount})
}

// MarkRead 将当前登录用户的指定消息标记为已读。
func (h *Handler) MarkRead(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req markReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.MarkRead(c.Request.Context(), userID, req.MessageIDs)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, markReadResponse{UpdatedCount: result.UpdatedCount})
}

// Create 供内部事件链路生成用户消息。
func (h *Handler) Create(c *gin.Context) {
	var req createMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.CreateFromActorEvent(
		c.Request.Context(),
		req.UserID,
		req.Type,
		req.Title,
		req.Content,
		req.EventID,
		c.GetHeader("Idempotency-Key"),
		req.ActorID,
		req.ActorNickname,
		req.ActorAvatarURL,
	)
	if err != nil {
		writeMessageError(c, err)
		return
	}

	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	c.JSON(status, responseFromDomain(result.Message))
}

func writeMessageError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isBadRequestError(err error) bool {
	return errors.Is(err, domainmessage.ErrInvalidUserID) ||
		errors.Is(err, domainmessage.ErrInvalidMessageID) ||
		errors.Is(err, domainmessage.ErrInvalidLimit) ||
		errors.Is(err, domainmessage.ErrInvalidCursor) ||
		errors.Is(err, domainmessage.ErrInvalidMessageType) ||
		errors.Is(err, domainmessage.ErrEmptyTitle) ||
		errors.Is(err, domainmessage.ErrTitleTooLong) ||
		errors.Is(err, domainmessage.ErrEmptyContent) ||
		errors.Is(err, domainmessage.ErrContentTooLong) ||
		errors.Is(err, domainmessage.ErrEventIDTooLong) ||
		errors.Is(err, domainmessage.ErrIdempotencyKeyTooLong)
}
