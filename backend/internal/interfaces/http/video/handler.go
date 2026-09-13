package interfaceshttpvideo

import (
	applicationvideo "GCFeed/internal/application/video"
	domainvideo "GCFeed/internal/domain/video"
	sharedhttputil "GCFeed/internal/shared/httputil"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

const defaultListLimit = 20

type Handler struct {
	service *applicationvideo.Service
}

// New 注入视频应用服务。
func New(service *applicationvideo.Service) *Handler {
	return &Handler{service: service}
}

// Create 处理发布视频请求，用户身份来自 JWT，上行数据来自 JSON 请求体。
func (h *Handler) Create(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req CreateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Idempotency-Key 来自请求头，用于客户端重试时获得同一个视频结果。
	result, err := h.service.CreatePublished(
		c.Request.Context(),
		userID,
		req.Title,
		req.Description,
		req.Tags,
		req.MediaURL,
		req.CoverURL,
		c.GetHeader("Idempotency-Key"),
	)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	status := http.StatusCreated
	if !result.Created {
		// 幂等重放返回已有资源，使用 200 表示本次没有新建记录。
		status = http.StatusOK
	}
	c.JSON(status, videoResponseFromDomain(result.Video))
}

// Get 查询公开视频详情，videoId 来自 RESTful 路径参数。
func (h *Handler) Get(c *gin.Context) {
	videoID, err := sharedhttputil.ParsePositiveInt64(c.Param("videoId"), domainvideo.ErrInvalidVideoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
		return
	}

	video, err := h.service.Get(c.Request.Context(), videoID)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	c.JSON(http.StatusOK, videoResponseFromDomain(video))
}

// Delete 删除当前用户自己的视频，删除操作在领域层做作者权限校验。
func (h *Handler) Delete(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	videoID, err := sharedhttputil.ParsePositiveInt64(c.Param("videoId"), domainvideo.ErrInvalidVideoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), userID, videoID); err != nil {
		writeVideoError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// ListByAuthor 查询指定用户的公开作品列表。
func (h *Handler) ListByAuthor(c *gin.Context) {
	authorID, err := sharedhttputil.ParsePositiveInt64(c.Param("userId"), domainvideo.ErrInvalidVideoID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	limit, offset, err := sharedhttputil.ParsePagination(c, defaultListLimit, domainvideo.ErrInvalidLimit, domainvideo.ErrInvalidOffset)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	videos, err := h.service.ListByAuthor(c.Request.Context(), authorID, limit, offset)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	c.JSON(http.StatusOK, videoListResponseFromDomain(videos, limit, offset))
}

// ListMine 查询当前登录用户自己的作品列表。
func (h *Handler) ListMine(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, offset, err := sharedhttputil.ParsePagination(c, defaultListLimit, domainvideo.ErrInvalidLimit, domainvideo.ErrInvalidOffset)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	videos, err := h.service.ListByAuthor(c.Request.Context(), userID, limit, offset)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	c.JSON(http.StatusOK, videoListResponseFromDomain(videos, limit, offset))
}

// writeVideoError 统一视频接口错误到 HTTP 状态码的映射。
func writeVideoError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainvideo.ErrVideoNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}
	if errors.Is(err, domainvideo.ErrVideoPermissionDenied) {
		c.JSON(http.StatusForbidden, gin.H{"error": "video permission denied"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断哪些视频领域错误属于客户端请求问题。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainvideo.ErrInvalidVideoID) ||
		errors.Is(err, domainvideo.ErrInvalidAuthorID) ||
		errors.Is(err, domainvideo.ErrEmptyTitle) ||
		errors.Is(err, domainvideo.ErrTitleTooLong) ||
		errors.Is(err, domainvideo.ErrDescriptionTooLong) ||
		errors.Is(err, domainvideo.ErrTooManyTags) ||
		errors.Is(err, domainvideo.ErrTagTooLong) ||
		errors.Is(err, domainvideo.ErrEmptyMediaURL) ||
		errors.Is(err, domainvideo.ErrEmptyCoverURL) ||
		errors.Is(err, domainvideo.ErrIdempotencyKeyTooLong) ||
		errors.Is(err, domainvideo.ErrInvalidLimit) ||
		errors.Is(err, domainvideo.ErrInvalidOffset)
}
