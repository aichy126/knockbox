package service

import (
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
)

// 过期会话和过期配对码必须真的被清掉。
//
// 这套清理一直存在，但从来没有任何地方调用它：接入页每被打开一次就在
// pair_code 里留一行，登录一次就留一行 session，两张表只增不减。
func TestSessionGCRemovesExpiredRows(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	now := time.Now().Unix()

	rows := []struct {
		id      string
		expires int64
	}{
		{"expired", now - 3600},
		{"live", now + 3600},
	}
	for _, r := range rows {
		if _, err := d.Engine().Exec(
			`INSERT INTO session(id,user_id,user_agent,ip,expires_at,created_at,last_seen_at)
			 VALUES(?,?,'ua','127.0.0.1',?,?,?)`, r.id, uid, r.expires, now, now); err != nil {
			t.Fatal(err)
		}
	}
	// 早就过期的码、刚用掉的码、还在有效期里的码
	codes := []struct {
		code    string
		expires int64
		used    int64
	}{
		{"OLDCODE1", now - 2*86400, 0},
		{"USEDCODE", now + 3600, now - 2*86400},
		{"LIVECODE", now + 3600, 0},
	}
	for _, c := range codes {
		if _, err := d.Engine().Exec(
			`INSERT INTO pair_code(code,user_id,issued_by,expires_at,used_at,created_at)
			 VALUES(?,?,'test',?,?,?)`, c.code, uid, c.expires, c.used, now); err != nil {
			t.Fatal(err)
		}
	}

	if err := NewSession(d).GC(); err != nil {
		t.Fatal(err)
	}

	var n int64
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM session WHERE id='expired'").Get(&n)
	if n != 0 {
		t.Error("过期会话没被清掉")
	}
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM session WHERE id='live'").Get(&n)
	if n != 1 {
		t.Error("有效会话被误删了")
	}
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM pair_code WHERE code='OLDCODE1'").Get(&n)
	if n != 0 {
		t.Error("过期配对码没被清掉")
	}
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM pair_code WHERE code='USEDCODE'").Get(&n)
	if n != 0 {
		t.Error("早已用掉的配对码没被清掉")
	}
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM pair_code WHERE code='LIVECODE'").Get(&n)
	if n != 1 {
		t.Error("还在有效期里的配对码被误删了")
	}
}

// GC 循环必须把会话清理带上——自建模式下它不做保留策略，
// 如果顺手把这一步也跳过，两张表就永远没人清。
func TestGCRoundCleansSessionsEvenInSelfHostMode(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	now := time.Now().Unix()
	if _, err := d.Engine().Exec(
		`INSERT INTO pair_code(code,user_id,issued_by,expires_at,used_at,created_at)
		 VALUES('STALECOD',?,'public',?,0,?)`, uid, now-2*86400, now-2*86400); err != nil {
		t.Fatal(err)
	}

	// PublicEnabled=false：自建模式，消息一条不动，但过期配对码照样要清
	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: false}), 0).Once()

	var n int64
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM pair_code").Get(&n)
	if n != 0 {
		t.Errorf("GC 跑完后过期配对码还剩 %d 条", n)
	}
}
