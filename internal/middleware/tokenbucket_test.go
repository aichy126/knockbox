package middleware

import (
	"sync"
	"testing"
	"time"
)

// fakeClock 令牌回填是按时间算的，用真实时间测就只能 sleep，
// 既慢又不稳。时钟自己拿在手里，测试就是确定的。
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func newTestBucket(rate, burst int) (*TokenBucket, *fakeClock) {
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	tb := NewTokenBucket(rate, burst)
	tb.now = clk.Now
	tb.lastSweep = clk.t
	return tb, clk
}

// 允许一次性把突发额度用光——一次构建失败连发几十条是正常的。
func TestTokenBucketAllowsFullBurstAtOnce(t *testing.T) {
	tb, _ := newTestBucket(2, 10)
	for i := 0; i < 10; i++ {
		if !tb.Allow("ch") {
			t.Fatalf("突发额度是 10，第 %d 次不该被挡", i+1)
		}
	}
	if tb.Allow("ch") {
		t.Error("额度用光之后应当被挡")
	}
}

// 长期速率要压住：额度用光后，令牌按 rate 回填。
func TestTokenBucketRefillsAtRate(t *testing.T) {
	tb, clk := newTestBucket(2, 4) // 每秒 2 个
	for i := 0; i < 4; i++ {
		if !tb.Allow("ch") {
			t.Fatalf("第 %d 次不该被挡", i+1)
		}
	}
	if tb.Allow("ch") {
		t.Fatal("额度已用光")
	}

	clk.advance(500 * time.Millisecond) // 回填 1 个
	if !tb.Allow("ch") {
		t.Error("半秒后应当回填出 1 个令牌")
	}
	if tb.Allow("ch") {
		t.Error("只回填了 1 个，第二次仍该被挡")
	}

	clk.advance(time.Second) // 回填 2 个
	for i := 0; i < 2; i++ {
		if !tb.Allow("ch") {
			t.Errorf("一秒后应当回填出 2 个，第 %d 次被挡了", i+1)
		}
	}
	if tb.Allow("ch") {
		t.Error("回填只有 2 个")
	}
}

// 回填不能超过突发上限，否则长时间不发之后能一次性灌进任意多条。
func TestTokenBucketDoesNotAccumulateBeyondBurst(t *testing.T) {
	tb, clk := newTestBucket(5, 5)
	if !tb.Allow("ch") {
		t.Fatal("第一次就被挡了")
	}
	clk.advance(time.Hour) // 攒一个小时

	for i := 0; i < 5; i++ {
		if !tb.Allow("ch") {
			t.Fatalf("第 %d 次不该被挡", i+1)
		}
	}
	if tb.Allow("ch") {
		t.Error("攒再久也不能超过突发上限")
	}
}

// 桶按 key 分开：一个频道被刷爆不能连累别的频道。
func TestTokenBucketIsolatesKeys(t *testing.T) {
	tb, _ := newTestBucket(1, 2)
	for i := 0; i < 2; i++ {
		tb.Allow("noisy")
	}
	if tb.Allow("noisy") {
		t.Fatal("noisy 的额度该用光了")
	}
	if !tb.Allow("quiet") {
		t.Error("另一个频道不该受影响")
	}
}

// rate <= 0 表示关闭限流，给需要完全放开的自建实例留的口子。
func TestTokenBucketDisabledWhenRateNotPositive(t *testing.T) {
	tb, _ := newTestBucket(0, 0)
	for i := 0; i < 1000; i++ {
		if !tb.Allow("ch") {
			t.Fatal("rate=0 应当表示不限")
		}
	}
	var nilBucket *TokenBucket
	if !nilBucket.Allow("ch") {
		t.Error("没有配置限流器时应当放行")
	}
}

// key 是频道 token，频道会被删、token 会被轮换，闲置的桶必须回收。
func TestTokenBucketSweepsIdleBuckets(t *testing.T) {
	tb, clk := newTestBucket(10, 10)
	for i := 0; i < 50; i++ {
		tb.Allow(string(rune('a' + i%26)))
	}
	if len(tb.buckets) == 0 {
		t.Fatal("应当建出一批桶")
	}
	clk.advance(2 * sweepEvery)
	tb.Allow("trigger") // 顺带触发清理
	if len(tb.buckets) != 1 {
		t.Errorf("闲置的桶应当被回收，只剩刚用过的那个，实际还有 %d 个", len(tb.buckets))
	}
}

// burst 留空时等于 rate，不额外允许突发。
func TestTokenBucketBurstDefaultsToRate(t *testing.T) {
	tb := NewTokenBucket(3, 0)
	if tb.burst != 3 {
		t.Errorf("burst 应当兜底成 rate，得到 %v", tb.burst)
	}
}
