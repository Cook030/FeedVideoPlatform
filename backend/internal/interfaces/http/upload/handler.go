package interfaceshttpupload

import (
	"errors"
	"net/http"

	applicationupload "GCFeed/internal/application/upload"

	"github.com/gin-gonic/gin"
)

// Handler 处理上传请求，业务逻辑全部委托给 application/upload.Service。
type Handler struct {
	service *applicationupload.Service
}

// uploadResponse 是上传成功后返回给前端的文件访问地址和元信息。
type uploadResponse struct {
	URL      string `json:"url"`
	Kind     string `json:"kind"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
}

// New 创建上传 Handler，service 由装配层注入。
func New(service *applicationupload.Service) *Handler {
	return &Handler{service: service}
}

// Create 接收 multipart/form-data 文件，并按 kind 保存到不同子目录。
func (h *Handler) Create(c *gin.Context) {
	// MaxBytesReader 在读取请求体前限制上传大小，避免大文件撑爆内存或磁盘。
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, applicationupload.MaxUploadBytes)

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}

	kind, ok := applicationupload.NormalizeKind(c.PostForm("kind"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid upload kind"})
		return
	}

	result, err := h.service.Save(c.Request.Context(), kind, file)
	if err != nil {
		writeUploadError(c, err)
		return
	}

	c.JSON(http.StatusCreated, uploadResponse{
		URL:      "/uploads/" + result.URL,
		Kind:     result.Kind,
		Filename: result.Filename,
		Size:     result.Size,
	})
}

// writeUploadError 统一上传失败的错误响应：校验类错误返回 400，内部错误返回 500。
func writeUploadError(c *gin.Context, err error) {
	if errors.Is(err, applicationupload.ErrVideoToolUnavailable) ||
		errors.Is(err, applicationupload.ErrFaststartFailed) ||
		errors.Is(err, applicationupload.ErrStorageFailed) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
