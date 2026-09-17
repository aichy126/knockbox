package middleware

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitFixedCeiling(t *testing.T) {
	r := NewRateLimit(2, time.Hour)
	for i := 0; i < 2; i++ {
		if !r.Allow("a") {
			t.Fatalf("第 %d 次请求在上限以内却被挡了", i+1)
		}
	}
	if r.Allow("a") {
		t.Error("超过上限仍被放行")
	}
	if !r.Allow("b") {
		t.Error("限流是按 key 分开的，另一个 key 不该受影响")
	}
}

// 后台里能编辑的上限必须每次现取。
// 上限在构造时定死的话，用户在界面上改完没有任何反应，还得重启才生效——
// 而界面上看不出这一点。
func TestRateLimitFuncReadsCeilingEachTime(t *testing.T) {
	var limit atomic.Int64
	limit.Store(1)
	r := NewRateLimitFunc(func() int { return int(limit.Load()) }, time.Hour)

	if !r.Allow("ip") {
		t.Fatal("第一次就被挡了")
	}
	if r.Allow("ip") {
		t.Fatal("上限是 1，第二次应当被挡")
	}

	limit.Store(5) // 相当于在后台把上限调大
	if !r.Allow("ip") {
		t.Error("上限调大之后应当立刻放行，不该等到重启")
	}
}

// 窗口过了要重新计数，否则一次超额会把这个 IP 永久挡住。
func TestRateLimitWindowResets(t *testing.T) {
	r := NewRateLimit(1, 30*time.Millisecond)
	if !r.Allow("ip") {
		t.Fatal("第一次就被挡了")
	}
	if r.Allow("ip") {
		t.Fatal("窗口内第二次应当被挡")
	}
	time.Sleep(50 * time.Millisecond)
	if !r.Allow("ip") {
		t.Error("窗口过了之后应当重新放行")
	}
}
