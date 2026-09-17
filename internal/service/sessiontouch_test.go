package service

import (
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
)

func lastSeen(t *testing.T, d *dao.DAO) int64 {
	t.Helper()
	var v int64
	if _, err := d.Engine().SQL("SELECT last_seen_at FROM session LIMIT 1").Get(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func loginOnce(t *testing.T, d *dao.DAO) string {
	t.Helper()
	if _, err := NewAccount(d).Create("admin", "hunter2hunter2", "admin"); err != nil {
		t.Fatal(err)
	}
	raw, _, err := NewSession(d).Login("admin", "hunter2hunter2", "ua", "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// 管理界面每个请求都要校验一次会话。原先每次都写一遍 last_seen_at，
// SQLite 下相当于给每个后台请求加一次写锁，而这个字段只用来显示「最近活动」。
func TestVerifyDoesNotWriteOnEveryRequest(t *testing.T) {
	d, _ := newTestDAO(t)
	raw := loginOnce(t, d)
	s := NewSession(d)

	// 刚记过活动
	now := time.Now().Unix()
	if _, err := d.Engine().Exec("UPDATE session SET last_seen_at = ?", now); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		if _, err := s.Verify(raw); err != nil {
			t.Fatalf("第 %d 次校验失败: %v", i+1, err)
		}
	}
	if got := lastSeen(t, d); got != now {
		t.Errorf("间隔内不该重写 last_seen_at：%d → %d", now, got)
	}
}

// 但也不能永远不写——「最近活动」显示成几天前就没用了。
func TestVerifyRefreshesStaleLastSeen(t *testing.T) {
	d, _ := newTestDAO(t)
	raw := loginOnce(t, d)

	stale := time.Now().Add(-time.Hour).Unix()
	if _, err := d.Engine().Exec("UPDATE session SET last_seen_at = ?", stale); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSession(d).Verify(raw); err != nil {
		t.Fatal(err)
	}
	if got := lastSeen(t, d); got <= stale {
		t.Errorf("超过间隔后应当刷新：还是 %d", got)
	}
}

// 节流只影响写入频率，不影响会话本身是否有效。
func TestVerifyStillRejectsExpiredSession(t *testing.T) {
	d, _ := newTestDAO(t)
	raw := loginOnce(t, d)
	if _, err := d.Engine().Exec(
		"UPDATE session SET expires_at = ?", time.Now().Add(-time.Minute).Unix()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSession(d).Verify(raw); err == nil {
		t.Error("过期会话仍被放行")
	}
}
