package interfaceshttpfeed

import (
	applicationfeed "GCFeed/internal/application/feed"
	domainfeed "GCFeed/internal/domain/feed"
	sharedhttputil "GCFeed/internal/shared/httputil"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationfeed.Service
}

// New 注入 Feed 应用服务。
func New(service *applicationfeed.Service) *Handler {
	return &Handler{service: service}
}

// ListFeedItems 读取指定 scene 的 Feed，cursor 和 limit 来自 query 参数。
func (h *Handler) ListFeedItems(c *gin.Context) {
	limit, err := sharedhttputil.ParseLimit(c.Query("limit"), domainfeed.ErrInvalidLimit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	viewerID, _ := sharedhttputil.ViewerIDFromContext(c)
	result, err := h.service.GetFeed(c.Request.Context(), applicationfeed.FeedRequest{
		Scene:    domainfeed.Scene(c.Query("scene")),
		Cursor:   c.Query("cursor"),
		Limit:    limit,
		ViewerID: viewerID,
	})
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, feedItemsResponseFromResult(result))
}

// Query 通过请求体接收复杂 Feed 查询参数，适合推荐上下文逐步扩展。
func (h *Handler) Query(c *gin.Context) {
	var req feedQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	limit, err := parseBodyLimit(req.Limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	viewerID, _ := sharedhttputil.ViewerIDFromContext(c)
	result, err := h.service.GetFeed(c.Request.Context(), applicationfeed.FeedRequest{
		Scene:         domainfeed.Scene(req.Scene),
		Cursor:        req.Cursor,
		Limit:         limit,
		ViewerID:      viewerID,
		ClientContext: req.ClientContext,
	})
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, feedItemsResponseFromResult(result))
}

// parseBodyLimit 校验 JSON 请求体中的 limit，空值交给应用服务使用默认页大小。
func parseBodyLimit(value *int) (int, error) {
	if value == nil {
		return 0, nil
	}
	if *value <= 0 {
		return 0, domainfeed.ErrInvalidLimit
	}
	return *value, nil
}

// writeFeedError 统一 Feed 接口错误响应。
func writeFeedError(c *gin.Context, err error) {
	if errors.Is(err, domainfeed.ErrViewerRequired) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断 Feed 参数错误。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainfeed.ErrInvalidLimit) ||
		errors.Is(err, domainfeed.ErrInvalidCursor) ||
		errors.Is(err, domainfeed.ErrUnsupportedScene)
}
