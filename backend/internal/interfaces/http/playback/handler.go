package interfaceshttpplayback

import (
	applicationplayback "GCFeed/internal/application/playback"
	domainplayback "GCFeed/internal/domain/playback"
	sharedhttputil "GCFeed/internal/shared/httputil"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationplayback.Service
}

// New 注入播放优化应用服务。
func New(service *applicationplayback.Service) *Handler {
	return &Handler{service: service}
}

// GetConfig 查询当前客户端播放配置。
func (h *Handler) GetConfig(c *gin.Context) {
	result, err := h.service.GetConfig(c.Request.Context(), c.Query("platform"), c.Query("network_type"))
	if err != nil {
		writePlaybackError(c, err)
		return
	}
	c.JSON(http.StatusOK, configResponseFromResult(result))
}

// ListPreloadVideos 查询 Feed 当前视频之后的预加载资源。
func (h *Handler) ListPreloadVideos(c *gin.Context) {
	currentVideoID, err := sharedhttputil.ParseOptionalInt64(c.Query("current_video_id"), domainplayback.ErrInvalidVideoID)
	if err != nil {
		writePlaybackError(c, domainplayback.ErrInvalidVideoID)
		return
	}
	limit, err := sharedhttputil.ParseLimit(c.Query("limit"), domainplayback.ErrInvalidLimit)
	if err != nil {
		writePlaybackError(c, err)
		return
	}

	result, err := h.service.ListPreloadVideos(c.Request.Context(), currentVideoID, limit)
	if err != nil {
		writePlaybackError(c, err)
		return
	}
	c.JSON(http.StatusOK, preloadResponseFromResult(result))
}

// CreateQoSReport 处理 Web 客户端播放质量上报。
func (h *Handler) CreateQoSReport(c *gin.Context) {
	userID, ok := sharedhttputil.UserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	h.createQoSReport(c, userID)
}

// CreateInternalQoSReport 处理服务间播放质量上报。
func (h *Handler) CreateInternalQoSReport(c *gin.Context) {
	var req createQoSReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	h.createQoSReportWithRequest(c, req.UserID, req)
}

func (h *Handler) createQoSReport(c *gin.Context, userID int64) {
	var req createQoSReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	h.createQoSReportWithRequest(c, userID, req)
}

func (h *Handler) createQoSReportWithRequest(c *gin.Context, userID int64, req createQoSReportRequest) {
	result, err := h.service.CreateQoSReport(
		c.Request.Context(),
		userID,
		req.VideoID,
		req.FirstFrameMs,
		req.StutterCount,
		req.WatchMs,
		c.GetHeader("Idempotency-Key"),
	)
	if err != nil {
		writePlaybackError(c, err)
		return
	}

	status := http.StatusCreated
	if !result.Created {
		status = http.StatusOK
	}
	c.JSON(status, qosResponseFromResult(result))
}

func writePlaybackError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isBadRequestError(err error) bool {
	return errors.Is(err, domainplayback.ErrInvalidUserID) ||
		errors.Is(err, domainplayback.ErrInvalidVideoID) ||
		errors.Is(err, domainplayback.ErrInvalidPlatform) ||
		errors.Is(err, domainplayback.ErrInvalidNetworkType) ||
		errors.Is(err, domainplayback.ErrInvalidLimit) ||
		errors.Is(err, domainplayback.ErrInvalidFirstFrameMs) ||
		errors.Is(err, domainplayback.ErrInvalidStutterCount) ||
		errors.Is(err, domainplayback.ErrInvalidWatchMs) ||
		errors.Is(err, domainplayback.ErrIdempotencyKeyTooLong)
}
