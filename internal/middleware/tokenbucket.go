package middleware

import (
	"sync"
	"time"
)

// TokenBucket 按 key 的令牌桶：长期速率 rate 个/秒，最多攒 burst 个。
//
// 和 RateLimit 的固定窗口分开，因为这两件事的形状不同。固定窗口挡的是
// 「每小时最多注册几次」这类低频动作；而发送必须允许瞬时成片到达
// （一次构建失败连发几十条是正常的），同时把长期速率压住。
// 用固定窗口做发送限流，要么窗口太小挡住正常的成片推送，要么窗口太大等于没挡。
//
// rate <= 0 表示不限，Allow 恒为 true。
type TokenBucket struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	rate    float64
	burst   float64
	// now 可替换，测试里注入一个假时钟，不必靠 sleep 去等令牌回填。
	now       func() time.Time
	lastSweep time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// sweepEvery 多久清理一次闲置的桶。
// 桶是按 key 建的，而 key 是频道 token——频道会被删、token 会被轮换，
// 不清理的话这张表只增不减。
const sweepEvery = time.Minute

// NewTokenBucket burst <= 0 时取 rate，也就是「不额外允许突发」。
func NewTokenBucket(rate, burst int) *TokenBucket {
	if burst <= 0 {
		burst = rate
	}
	return &TokenBucket{
		buckets: map[string]*bucket{},
		rate:    float64(rate),
		burst:   float64(burst),
		now:     time.Now,
	}
}

// Allow 取一个令牌，返回是否放行。
func (t *TokenBucket) Allow(key string) bool {
	if t == nil || t.rate <= 0 {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	t.sweepLocked(now)

	b, ok := t.buckets[key]
	if !ok {
		// 新来的 key 从满桶开始，扣掉这一次。
		t.buckets[key] = &bucket{tokens: t.burst - 1, last: now}
		return true
	}
	if elapsed := now.Sub(b.last).Seconds(); elapsed > 0 {
		b.tokens += elapsed * t.rate
		if b.tokens > t.burst {
			b.tokens = t.burst
		}
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// sweepLocked 丢掉已经攒满、说明这一阵没人用的桶。
// 在 Allow 里顺带做，省掉一个常驻 goroutine。
func (t *TokenBucket) sweepLocked(now time.Time) {
	if now.Sub(t.lastSweep) < sweepEvery {
		return
	}
	t.lastSweep = now
	for k, b := range t.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*t.rate >= t.burst {
			delete(t.buckets, k)
		}
	}
}
