package interfaceshttpmessage

import (
	"net/http"
	"net/http/httptest"
	"testing"

	infrajwt "GCFeed/internal/infra/jwt"
	interfaceshttpmiddleware "GCFeed/internal/interfaces/http/middleware"

	"github.com/gin-gonic/gin"
)

// TestSSEAuthFromQueryToken 只覆盖 HTTP 层的 query token 鉴权；
// Hub 的连接注册、背压驱逐与订阅抽象由 infra/realtime 的同包测试覆盖。
func TestSSEAuthFromQueryToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager, err := infrajwt.NewManager("test-secret", "15m")
	if err != nil {
		t.Fatalf("new jwt manager: %v", err)
	}
	token, err := manager.SignAccessToken(42, "user")
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	var gotUserID int64
	router := gin.New()
	router.GET("/api/messages/stream", interfaceshttpmiddleware.NewSSEAuth(manager), func(c *gin.Context) {
		value, _ := c.Get(interfaceshttpmiddleware.ContextUserIDKey)
		gotUserID, _ = value.(int64)
		c.Status(http.StatusNoContent)
	})

	// 缺少 token 时拒绝建连。
	requireStatus(t, httptest.NewRecorder(), router, "/api/messages/stream", http.StatusUnauthorized)

	// query token 合法时放行并写入用户上下文。
	recorder := httptest.NewRecorder()
	requireStatus(t, recorder, router, "/api/messages/stream?token="+token, http.StatusNoContent)
	if gotUserID != 42 {
		t.Fatalf("expected user id 42, got %d", gotUserID)
	}
}

func requireStatus(t *testing.T, recorder *httptest.ResponseRecorder, router *gin.Engine, path string, want int) {
	t.Helper()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != want {
		t.Fatalf("path %s: expected status %d, got %d", path, want, recorder.Code)
	}
}
