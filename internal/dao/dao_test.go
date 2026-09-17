package dao

import (
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/migrate"
	"xorm.io/xorm"
)

func newDAO(t *testing.T) *DAO {
	t.Helper()
	e, err := xorm.NewEngine("sqlite3", filepath.Join(t.TempDir(), "t.db")+"?"+migrate.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	return New(e)
}

// rev 是增量同步的唯一游标，必须严格单调递增。
// 出现重复或空洞时，越过那一段的客户端就再也拉不回来，而且不会有任何报错。
func TestNextRevIsStrictlyIncreasing(t *testing.T) {
	d := newDAO(t)
	var prev int64
	for i := 0; i < 20; i++ {
		var rev int64
		if err := d.Tx(func(sess *xorm.Session) error {
			var err error
			rev, err = NextRev(sess)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if rev <= prev {
			t.Fatalf("第 %d 次取到 %d，不大于上一次的 %d", i+1, rev, prev)
		}
		prev = rev
	}
	cur, err := d.CurrentRev()
	if err != nil {
		t.Fatal(err)
	}
	if cur != prev {
		t.Fatalf("CurrentRev 应当等于最后分配的 %d，得到 %d", prev, cur)
	}
}

// rev 的分配和用它写的那一行必须同生共死。
// 事务回滚了而 rev 已经耗掉的话，同步流里就留下一个没有对应数据的空洞。
func TestRolledBackTxConsumesNoRev(t *testing.T) {
	d := newDAO(t)
	before, err := d.CurrentRev()
	if err != nil {
		t.Fatal(err)
	}

	boom := errors.New("有意失败")
	err = d.Tx(func(sess *xorm.Session) error {
		if _, err := NextRev(sess); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Tx 应当把 fn 的错误原样返回，得到 %v", err)
	}

	after, err := d.CurrentRev()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("事务回滚了 rev 却往前走了：%d → %d", before, after)
	}
}

func TestTxCommitsOnSuccess(t *testing.T) {
	d := newDAO(t)
	if err := d.Tx(func(sess *xorm.Session) error {
		_, err := sess.Exec("INSERT INTO kv(k, v, updated_at) VALUES('tx', 'ok', 1)")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	v, ok, err := d.KVGet("tx")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || v != "ok" {
		t.Fatalf("提交后应当读得到，得到 ok=%v v=%q", ok, v)
	}
}

// KVSet 要能新建也能覆盖：它是设置、签名密钥、GC 水位线的共同落脚点。
func TestKVSetInsertsThenUpdates(t *testing.T) {
	d := newDAO(t)
	if _, ok, _ := d.KVGet("missing"); ok {
		t.Fatal("没写过的键不该存在")
	}
	if err := d.KVSet("k", "v1", 100); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := d.KVGet("k"); !ok || v != "v1" {
		t.Fatalf("首次写入读回 ok=%v v=%q", ok, v)
	}
	if err := d.KVSet("k", "v2", 200); err != nil {
		t.Fatal(err)
	}
	if v, ok, _ := d.KVGet("k"); !ok || v != "v2" {
		t.Fatalf("覆盖之后读回 ok=%v v=%q", ok, v)
	}
	var n int64
	if _, err := d.Engine().SQL("SELECT COUNT(*) FROM kv WHERE k='k'").Get(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("覆盖写不该新增行，kv 里有 %d 行 k", n)
	}
}
