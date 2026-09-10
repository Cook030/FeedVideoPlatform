package interfaceshttpmessage

import (
	infrajwt "GCFeed/internal/infra/jwt"
	interfaceshttpmiddleware "GCFeed/internal/interfaces/http/middleware"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

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

func TestHubDispatchToLocalClient(t *testing.T) {
	hub := NewHub()
	client := hub.register(7)
	defer hub.unregister(client)

	hub.OnEvent(7, "message", []byte(`{"id":1}`))

	select {
	case event := <-client.ch:
		if event.Type != "message" {
			t.Fatalf("expected message type, got %q", event.Type)
		}
		if string(event.Data) != `{"id":1}` {
			t.Fatalf("unexpected data: %s", event.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("expected event to be delivered")
	}

	// 其它用户的连接不应收到事件。
	other := hub.register(8)
	defer hub.unregister(other)
	hub.OnEvent(7, "message", []byte(`{"id":2}`))
	select {
	case <-other.ch:
		t.Fatal("unexpected event delivered to another user")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestHubDropsSlowClient(t *testing.T) {
	hub := NewHub()
	client := hub.register(9)
	defer hub.unregister(client)

	// 填满该连接的缓冲，模拟消费过慢的客户端。
	for i := 0; i < sseClientBuffer; i++ {
		hub.OnEvent(9, "message", []byte(`{"id":1}`))
	}
	// 再投递一次应触发断开，channel 被关闭。
	hub.OnEvent(9, "message", []byte(`{"id":2}`))

	deadline := time.After(time.Second)
	for {
		select {
		case _, open := <-client.ch:
			if !open {
				return
			}
		case <-deadline:
			t.Fatal("expected slow client to be dropped")
		}
	}
}

func TestHubCloseClosesAllClients(t *testing.T) {
	hub := NewHub()
	first := hub.register(1)
	second := hub.register(2)

	hub.Close()

	requireChannelClosed(t, first)
	requireChannelClosed(t, second)

	// 重复 Close 应安全无副作用。
	hub.Close()
}

func requireChannelClosed(t *testing.T, client *streamClient) {
	t.Helper()
	select {
	case _, open := <-client.ch:
		if open {
			t.Fatal("expected client channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("expected client channel to be closed")
	}
}

func requireStatus(t *testing.T, recorder *httptest.ResponseRecorder, router *gin.Engine, path string, want int) {
	t.Helper()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
	if recorder.Code != want {
		t.Fatalf("path %s: expected status %d, got %d", path, want, recorder.Code)
	}
}
