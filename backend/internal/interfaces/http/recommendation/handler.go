package interfaceshttprecommendation

import (
	applicationrecommendation "GCFeed/internal/application/recommendation"
	domainrecommendation "GCFeed/internal/domain/recommendation"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationrecommendation.Service
}

func New(service *applicationrecommendation.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) ListCandidates(c *gin.Context) {
	var req candidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	limit := 0
	if req.Limit != nil {
		limit = *req.Limit
	}
	result, err := h.service.Recommend(c.Request.Context(), applicationrecommendation.CandidateRequest{
		UserID:    req.UserID,
		Scene:     req.Scene,
		RequestID: req.RequestID,
		Cursor:    req.Cursor,
		Limit:     limit,
	})
	if err != nil {
		writeRecommendationError(c, err)
		return
	}

	c.JSON(http.StatusOK, candidateResponseFromResult(result))
}

func (h *Handler) SaveExposures(c *gin.Context) {
	var req exposuresRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	inputs := make([]applicationrecommendation.ExposureInput, 0, len(req.VideoIDs))
	for _, videoID := range req.VideoIDs {
		inputs = append(inputs, applicationrecommendation.ExposureInput{
			UserID:    req.UserID,
			VideoID:   videoID,
			Scene:     req.Scene,
			RequestID: req.RequestID,
		})
	}
	result, err := h.service.SaveExposures(c.Request.Context(), inputs)
	if err != nil {
		writeRecommendationError(c, err)
		return
	}

	c.JSON(http.StatusCreated, exposuresResponseFromResult(result))
}

func (h *Handler) DecideExposures(c *gin.Context) {
	var req exposureDecisionsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.DecideExposures(c.Request.Context(), applicationrecommendation.ExposureDecisionInput{
		UserID:    req.UserID,
		Scene:     req.Scene,
		RequestID: req.RequestID,
		VideoIDs:  req.VideoIDs,
	})
	if err != nil {
		writeRecommendationError(c, err)
		return
	}

	c.JSON(http.StatusOK, exposureDecisionsResponseFromResult(result))
}

func writeRecommendationError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainrecommendation.ErrVideoNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isBadRequestError(err error) bool {
	return errors.Is(err, domainrecommendation.ErrInvalidUserID) ||
		errors.Is(err, domainrecommendation.ErrInvalidVideoID) ||
		errors.Is(err, domainrecommendation.ErrInvalidLimit) ||
		errors.Is(err, domainrecommendation.ErrEmptyScene) ||
		errors.Is(err, domainrecommendation.ErrSceneTooLong) ||
		errors.Is(err, domainrecommendation.ErrRequestIDTooLong) ||
		errors.Is(err, domainrecommendation.ErrInvalidCursor)
}
