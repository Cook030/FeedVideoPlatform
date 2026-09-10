package interfaceshttpmiddleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// visitorIdleTTL 是空转访客的回收时间，避免按 IP 的令牌桶无限增长。
const visitorIdleTTL = 10 * time.Minute

// 登录和注册是匿名可调用且成本不对称的接口：前者每次校验都要跑 bcrypt，
// 后者会写库。限流的目的是让单来源的爆破和灌水变得不划算。
const (
	loginLimitInterval    = time.Minute / 10 // 平均每 6 秒放行一次
	loginLimitBurst       = 10
	registerLimitInterval = 12 * time.Minute // 平均每小时放行 5 次
	registerLimitBurst    = 5
)

// NewLoginRateLimit 返回登录接口的按 IP 限流中间件。
func NewLoginRateLimit() gin.HandlerFunc {
	return NewIPRateLimit(loginLimitInterval, loginLimitBurst)
}

// NewRegisterRateLimit 返回注册接口的按 IP 限流中间件。
func NewRegisterRateLimit() gin.HandlerFunc {
	return NewIPRateLimit(registerLimitInterval, registerLimitBurst)
}

// NewIPRateLimit 返回按客户端 IP 限流的中间件。
// 使用进程内令牌桶而非 Redis，是因为当前部署是单体单实例，最小改动即可生效。
func NewIPRateLimit(interval time.Duration, burst int) gin.HandlerFunc {
	limiter := newVisitorLimiter(rate.Every(interval), burst)
	return func(c *gin.Context) {
		if !limiter.allow(c.ClientIP()) {
			c.Header("Retry-After", "60")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"message": "too many requests",
			})
			return
		}
		c.Next()
	}
}

// visitorLimiter 为每个 key（这里是客户端 IP）维护一个独立的令牌桶。
type visitorLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	limit    rate.Limit
	burst    int
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newVisitorLimiter(limit rate.Limit, burst int) *visitorLimiter {
	l := &visitorLimiter{
		visitors: make(map[string]*visitor),
		limit:    limit,
		burst:    burst,
	}
	go l.cleanupLoop()
	return l
}

func (l *visitorLimiter) allow(key string) bool {
	l.mu.Lock()
	v, ok := l.visitors[key]
	if !ok {
		v = &visitor{limiter: rate.NewLimiter(l.limit, l.burst)}
		l.visitors[key] = v
	}
	v.lastSeen = time.Now()
	l.mu.Unlock()
	return v.limiter.Allow()
}

func (l *visitorLimiter) cleanupLoop() {
	ticker := time.NewTicker(visitorIdleTTL)
	defer ticker.Stop()
	for range ticker.C {
		l.cleanupIdle()
	}
}

func (l *visitorLimiter) cleanupIdle() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, v := range l.visitors {
		if time.Since(v.lastSeen) > visitorIdleTTL {
			delete(l.visitors, key)
		}
	}
}
