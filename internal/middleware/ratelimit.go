package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimit 按 IP 的固定窗口限流。
//
// 用内存计数而不是 Redis：这是个单机自建服务，进程重启丢掉计数没有影响——
// 攻击者拿不到「让服务重启」这个能力，而运维重启时清零反而是想要的。
type RateLimit struct {
	mu     sync.Mutex
	hits   map[string]*window
	limit  func() int
	period time.Duration
}

type window struct {
	count int
	until time.Time
}

// NewRateLimit 上限固定。
func NewRateLimit(limit int, period time.Duration) *RateLimit {
	return NewRateLimitFunc(func() int { return limit }, period)
}

// NewRateLimitFunc 上限每次现取。
// 后台里能编辑的设置必须走这个：上限在构造时定死的话，改完要重启才生效，
// 而界面上看不出这一点。
func NewRateLimitFunc(limit func() int, period time.Duration) *RateLimit {
	r := &RateLimit{hits: map[string]*window{}, limit: limit, period: period}
	go r.reap()
	return r
}

// Allow 记一次并返回是否放行。
func (r *RateLimit) Allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	w, ok := r.hits[key]
	if !ok || now.After(w.until) {
		r.hits[key] = &window{count: 1, until: now.Add(r.period)}
		return true
	}
	w.count++
	return w.count <= r.limit()
}

func (r *RateLimit) reap() {
	for range time.Tick(time.Minute) {
		r.mu.Lock()
		now := time.Now()
		for k, w := range r.hits {
			if now.After(w.until) {
				delete(r.hits, k)
			}
		}
		r.mu.Unlock()
	}
}

// Gin 返回一个按客户端 IP 限流的中间件。
// Gin 限流中间件。收的是 uierr 的 code 而不是一句现成的话：
// 被限的是一个人（配对、接入都发生在他面前），该用他的语言说这句。
// render 由 api 层给进来——middleware 不该认识语料。
func (r *RateLimit) Gin(code string, render func(*gin.Context, string, ...any) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !r.Allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code": 1,
				"msg":  render(c, code),
				"data": gin.H{"error_code": code},
			})
			return
		}
		c.Next()
	}
}
