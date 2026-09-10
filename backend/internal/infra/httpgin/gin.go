package infrahttpgin

import (
	infraconfig "GCFeed/internal/infra/config"
	inframetrics "GCFeed/internal/infra/metrics"
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// shutdownTimeout 是优雅退出时等待在途请求结束的最长时间。
const shutdownTimeout = 10 * time.Second

// sseLogSkipPaths 这些路由使用 query token 鉴权，跳过访问日志避免凭据泄露。
var sseLogSkipPaths = []string{"/api/messages/stream"}

// Init 创建 Gin 引擎，并使用默认日志和恢复中间件。
func Init() *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	g := gin.New()
	g.Use(gin.LoggerWithConfig(gin.LoggerConfig{SkipPaths: sseLogSkipPaths}), gin.Recovery())
	g.Use(inframetrics.HTTPMiddleware())
	return g
}

// Run 启动 HTTP 服务，并在 ctx 取消时优雅退出：先停止接收新连接，再等待在途请求结束。
func Run(ctx context.Context, cfg *infraconfig.Config, g *gin.Engine) error {
	server := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Port),
		Handler: g,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return nil
}
