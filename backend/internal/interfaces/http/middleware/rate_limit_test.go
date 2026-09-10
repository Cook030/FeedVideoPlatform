package interfaceshttpmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestIPRateLimitRejectsAfterBurst(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/login", NewIPRateLimit(time.Hour, 2), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	statuses := make([]int, 0, 3)
	for i := 0; i < 3; i++ {
		statuses = append(statuses, doRequest(engine, "10.0.0.1:1234"))
	}

	if statuses[0] != http.StatusNoContent || statuses[1] != http.StatusNoContent {
		t.Fatalf("expected first two requests to pass, got %v", statuses)
	}
	if statuses[2] != http.StatusTooManyRequests {
		t.Fatalf("expected third request to be limited, got %v", statuses)
	}
}

func TestIPRateLimitKeepsVisitorsIndependent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/login", NewIPRateLimit(time.Hour, 1), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	if status := doRequest(engine, "10.0.0.1:1234"); status != http.StatusNoContent {
		t.Fatalf("expected first visitor to pass, got %d", status)
	}
	if status := doRequest(engine, "10.0.0.1:1234"); status != http.StatusTooManyRequests {
		t.Fatalf("expected repeated visitor to be limited, got %d", status)
	}
	if status := doRequest(engine, "10.0.0.2:1234"); status != http.StatusNoContent {
		t.Fatalf("expected other visitor to pass, got %d", status)
	}
}

func doRequest(engine *gin.Engine, remoteAddr string) int {
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec.Code
}
