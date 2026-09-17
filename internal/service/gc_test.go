package service

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestGCPhysicallyDeletes 保留策略必须是【物理删除】。
//
// SQLite 的 DELETE 只把页标记为空闲，内容仍留在文件里直到被新数据覆盖——
// 所以光看 SELECT 查不到是不够的，必须去原始文件里 grep。
// 这个测试的价值全在最后那几行：它会在有人不小心去掉 secure_delete
// 或者漏掉 wal_checkpoint 时立刻失败。
func TestGCPhysicallyDeletes(t *testing.T) {
	d, dir := newTestDAO(t)
	const canary = "CANARY-7f3a9c-GC"

	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	old := time.Now().AddDate(0, 0, -60).Unix()
	for i := 0; i < 3; i++ {
		if _, err := d.Engine().Exec(
			`INSERT INTO message(uid,user_id,channel_id,type,title,summary,body,rev,created_at)
			 VALUES(?,?,?,'text',?,?,?,?,?)`,
			fmt.Sprintf("m%d", i), uid, ch, "t", canary, canary, int64(i+1), old); err != nil {
			t.Fatal(err)
		}
	}

	st := NewSettings(d, Defaults{PublicEnabled: true, RetentionDays: 30})
	NewGC(d, nil, st, 0).Once()

	var left int64
	if _, err := d.Engine().SQL("SELECT COUNT(*) FROM message").Get(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("超期消息没被删掉，还剩 %d 条", left)
	}

	// gc_watermark 必须写上：没有它，离线很久的设备永远学不到这些消息没了
	if v, ok, _ := d.KVGet("gc_watermark"); !ok || v == "" {
		t.Fatal("gc_watermark 没写，离线设备将无法察觉数据缺口")
	}

	// 真正的检查：原始文件里不能再有那个字符串
	for _, name := range []string{"test.db", "test.db-wal"} {
		p := filepath.Join(dir, name)
		b, err := os.ReadFile(p)
		if err != nil {
			continue // -wal 可能已经被 checkpoint 掉了
		}
		if bytes.Contains(b, []byte(canary)) {
			t.Fatalf("%s 里还能搜到已删除的内容——secure_delete 或 wal_checkpoint 失效了", name)
		}
	}
}

// TestGCSkipsUnlimitedUsers 豁免的用户不该被清。
func TestGCSkipsUnlimitedUsers(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	if _, err := d.Engine().Exec("UPDATE user SET unlimited=1 WHERE id=?", uid); err != nil {
		t.Fatal(err)
	}
	ch := mustChannel(t, d, uid)
	old := time.Now().AddDate(0, 0, -60).Unix()
	if _, err := d.Engine().Exec(
		`INSERT INTO message(uid,user_id,channel_id,type,title,summary,body,rev,created_at)
		 VALUES('keep',?,?,'text','t','s','b',1,?)`, uid, ch, old); err != nil {
		t.Fatal(err)
	}
	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: true, RetentionDays: 30}), 0).Once()

	var left int64
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM message").Get(&left)
	if left != 1 {
		t.Fatalf("豁免用户的消息被清掉了，剩 %d 条", left)
	}
}

// TestGCNoRetentionKeepsEverything 自建模式下一条都不能动。
func TestGCNoRetentionKeepsEverything(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	old := time.Now().AddDate(0, -6, 0).Unix()
	if _, err := d.Engine().Exec(
		`INSERT INTO message(uid,user_id,channel_id,type,title,summary,body,rev,created_at)
		 VALUES('ancient',?,?,'text','t','s','b',1,?)`, uid, ch, old); err != nil {
		t.Fatal(err)
	}
	// PublicEnabled=false：自建模式，完整归档是刻意的取舍
	NewGC(d, nil, NewSettings(d, Defaults{PublicEnabled: false, RetentionDays: 30}), 0).Once()

	var left int64
	_, _ = d.Engine().SQL("SELECT COUNT(*) FROM message").Get(&left)
	if left != 1 {
		t.Fatal("自建模式下不该清理任何消息")
	}
}
