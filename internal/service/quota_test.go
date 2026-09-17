package service

import (
	"errors"
	"github.com/aichy126/knockbox/internal/uierr"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
)

func seedMessages(t *testing.T, d *dao.DAO, uid int64, ch string, n int, at int64) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := d.Engine().Exec(
			`INSERT INTO message(uid,user_id,channel_id,type,title,summary,body,rev,created_at)
			 VALUES(?,?,?,'text','t','s','b',?,?)`,
			"q"+string(rune('a'+i)), uid, ch, int64(i+1), at); err != nil {
			t.Fatal(err)
		}
	}
}

// 配额窗口是滚动的 24 小时，所以恢复时刻由【窗口里最早那条消息】决定。
//
// 原来这里硬编码 Reset = 1 小时，用户看到「1 小时后恢复」，等一小时回来照样被挡。
// 一个说错话的提示比不给提示更糟：它让人放弃排查。
func TestDailyQuotaReportsRealResetTime(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)

	// 两小时前发满额度：还要再等 22 小时最早那条才滑出窗口
	seedMessages(t, d, uid, ch, 3, time.Now().Add(-2*time.Hour).Unix())

	err := NewQuota(d, Limits{Enabled: true, MaxPerDay: 3}).CheckSend(uid)
	if err == nil {
		t.Fatal("已达上限却放行了")
	}
	var qe *QuotaError
	if !errors.As(err, &qe) {
		t.Fatalf("应当是 *QuotaError，得到 %T", err)
	}
	if qe.Reset < 21*time.Hour || qe.Reset > 22*time.Hour+time.Minute {
		t.Errorf("恢复时间应当在 22 小时上下，得到 %s", qe.Reset)
	}
	// 算得出恢复时刻就该承诺它——这是「用哪个 code」的区别，不是措辞的区别。
	// 句子本身（不说「今天」、说清是滚动 24 小时）由语料负责，
	// 断言在 internal/api/web 那边。
	if qe.Code() != uierr.QuotaDailyWithETA {
		t.Errorf("算得出恢复时刻时应当用 %s，得到 %s", uierr.QuotaDailyWithETA, qe.Code())
	}
	if len(qe.Args()) != 2 {
		t.Errorf("带恢复时刻的那句要两个插值，得到 %v", qe.Args())
	}
}

// 窗口里一条都没有却仍然判超额（MaxPerDay = 0 以外的边界），
// 算不出恢复时刻时就不要承诺一个时间。
func TestDailyQuotaOmitsResetWhenUnknown(t *testing.T) {
	e := &QuotaError{Kind: "daily", Limit: 5}
	if e.Code() != uierr.QuotaDaily {
		t.Errorf("算不出恢复时刻时不该用带时间的那句：%s", e.Code())
	}
}

// 豁免的用户不受配额限制。
func TestExemptUserBypassesQuota(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	seedMessages(t, d, uid, ch, 3, time.Now().Unix())
	if _, err := d.Engine().Exec("UPDATE user SET unlimited=1 WHERE id=?", uid); err != nil {
		t.Fatal(err)
	}
	if err := NewQuota(d, Limits{Enabled: true, MaxPerDay: 3}).CheckSend(uid); err != nil {
		t.Fatalf("豁免用户被限了: %v", err)
	}
}

// 自建模式（Enabled=false）下配额一律不生效。
func TestQuotaDisabledInSelfHostMode(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	seedMessages(t, d, uid, ch, 5, time.Now().Unix())
	q := NewQuota(d, Limits{Enabled: false, MaxPerDay: 1, MaxChannels: 1})
	if err := q.CheckSend(uid); err != nil {
		t.Errorf("自建模式不该限制发送: %v", err)
	}
	if err := q.CheckChannel(uid); err != nil {
		t.Errorf("自建模式不该限制建频道: %v", err)
	}
}
