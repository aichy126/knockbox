package service

import (
	"fmt"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
)

func seedOldMessage(t *testing.T, d *dao.DAO, uid int64, ch, uidStr string, daysAgo int) {
	t.Helper()
	at := time.Now().AddDate(0, 0, -daysAgo).Unix()
	if _, err := d.Engine().Exec(
		`INSERT INTO message(uid,user_id,channel_id,type,title,summary,body,rev,created_at)
		 VALUES(?,?,?,'text','t','s','b',?,?)`, uidStr, uid, ch, at, at); err != nil {
		t.Fatal(err)
	}
}

func msgCount(t *testing.T, d *dao.DAO) int64 {
	t.Helper()
	var n int64
	if _, err := d.Engine().SQL("SELECT COUNT(*) FROM message").Get(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// retention.days 是【实例级】保留期，自建模式下也必须生效。
//
// 原来 GC 只在公共模式下清理，而它读的还是 public.retention_days ——
// 自建用户根本没有任何设置保留期的办法，库只会一直涨。
func TestInstanceRetentionAppliesInSelfHostMode(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	seedOldMessage(t, d, uid, ch, "old", 60)
	seedOldMessage(t, d, uid, ch, "recent", 3)

	// PublicEnabled=false，只有实例级保留期
	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: false}), 30).Once()

	if got := msgCount(t, d); got != 1 {
		t.Fatalf("应当只剩最近那条，实际剩 %d 条", got)
	}
	var left string
	_, _ = d.Engine().SQL("SELECT uid FROM message").Get(&left)
	if left != "recent" {
		t.Errorf("删错了，剩下的是 %q", left)
	}
	if v, ok, _ := d.KVGet("gc_watermark"); !ok || v == "" {
		t.Error("硬删之后必须写 gc_watermark，否则离线设备察觉不到数据缺口")
	}
}

// 实例级保留期是运营者对自己那块磁盘的决定，被豁免配额的成员也不例外。
// unlimited 豁免的是公共实例的配额，不是「这台机器上永远不删」。
func TestInstanceRetentionAppliesToUnlimitedUsers(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	if _, err := d.Engine().Exec("UPDATE user SET unlimited=1 WHERE id=?", uid); err != nil {
		t.Fatal(err)
	}
	ch := mustChannel(t, d, uid)
	seedOldMessage(t, d, uid, ch, "old", 60)

	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: false}), 30).Once()

	if got := msgCount(t, d); got != 0 {
		t.Fatalf("实例级保留期对豁免成员同样生效，却还剩 %d 条", got)
	}
}

// 两个保留期都设时按更严的算。
func TestStricterRetentionWins(t *testing.T) {
	for _, tc := range []struct {
		name          string
		instance      int
		public        int
		ageDays       int
		wantRemaining int64
	}{
		{"实例级更严，消息落在它之外", 10, 30, 20, 0},
		{"公共更严，消息落在它之外", 30, 10, 20, 0},
		{"两个都覆盖不到", 30, 30, 5, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, _ := newTestDAO(t)
			uid := mustUser(t, d)
			ch := mustChannel(t, d, uid)
			seedOldMessage(t, d, uid, ch, "m", tc.ageDays)

			NewGC(d, nil, NewSettings(d, Defaults{
				PublicEnabled: true, RetentionDays: tc.public,
			}), tc.instance).Once()

			if got := msgCount(t, d); got != tc.wantRemaining {
				t.Errorf("剩 %d 条，期望 %d", got, tc.wantRemaining)
			}
		})
	}
}

// 公共实例的每用户上限仍然只约束没被豁免的成员。
func TestPublicRetentionStillSkipsUnlimitedUsers(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	if _, err := d.Engine().Exec("UPDATE user SET unlimited=1 WHERE id=?", uid); err != nil {
		t.Fatal(err)
	}
	ch := mustChannel(t, d, uid)
	seedOldMessage(t, d, uid, ch, "old", 60)

	// 只有 public.retention_days，没有实例级
	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: true, RetentionDays: 30}), 0).Once()

	if got := msgCount(t, d); got != 1 {
		t.Fatalf("豁免成员不受公共配额约束，却被清了，剩 %d 条", got)
	}
}

// 两个都是 0 就一条都不动——完整归档是默认。
func TestNoRetentionKeepsEverything(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	seedOldMessage(t, d, uid, ch, "ancient", 400)

	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: true, RetentionDays: 0}), 0).Once()

	if got := msgCount(t, d); got != 1 {
		t.Fatalf("没设保留期时不该动任何消息，剩 %d 条", got)
	}
}

// 保留期的取值规则单独测一遍，覆盖各种组合。
func TestRetentionFor(t *testing.T) {
	cases := []struct {
		name      string
		instance  int
		public    int
		unlimited int
		want      int
	}{
		{"都没设", 0, 0, 0, 0},
		{"只有实例级", 30, 0, 0, 30},
		{"只有公共", 0, 30, 0, 30},
		{"只有公共，成员被豁免", 0, 30, 1, 0},
		{"实例级更严", 10, 30, 0, 10},
		{"公共更严", 30, 10, 0, 10},
		{"公共更严但成员被豁免，回到实例级", 30, 10, 1, 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := &GC{retentionDays: tc.instance}
			if got := g.retentionFor(tc.unlimited, tc.public); got != tc.want {
				t.Errorf("retentionFor(unlimited=%d, public=%d) 在实例级 %d 下 = %d，期望 %d",
					tc.unlimited, tc.public, tc.instance, got, tc.want)
			}
		})
	}
}

// 投递记录跟着消息走：消息没了，它的 push_log 留着没有意义。
func TestRetentionAlsoClearsPushLog(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	seedOldMessage(t, d, uid, ch, "old", 60)

	var mid int64
	if _, err := d.Engine().SQL("SELECT id FROM message WHERE uid='old'").Get(&mid); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Engine().Exec(
		fmt.Sprintf(`INSERT INTO push_log(message_id,device_id,status,created_at,updated_at)
		 VALUES(%d,1,1,0,0)`, mid)); err != nil {
		t.Fatal(err)
	}

	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: false}), 30).Once()

	var n int64
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM push_log").Get(&n)
	if n != 0 {
		t.Errorf("消息被清之后 push_log 应当一并清掉，还剩 %d 条", n)
	}
}
