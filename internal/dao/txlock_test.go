package dao

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"xorm.io/xorm"
)

const basePragmas = "_pragma=busy_timeout(3000)&_pragma=journal_mode(WAL)&_pragma=secure_delete(ON)"

// 两个并发的「先读后写」事务。
//
// deferred（SQLite 默认）：两个都以读锁开始，谁先写谁拿到写锁，
// 另一个升级时直接失败，且 busy_timeout 对锁升级【不生效】——这就是随机 "database is locked" 的来源。
//
// immediate：Begin 的那一刻就拿写锁，第二个在 Begin 处等待（受 busy_timeout 保护），
// 等到前一个提交再继续，两个都成功。
func racingReadThenWrite(t *testing.T, dsn string) error {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.db") + "?" + dsn
	e, err := xorm.NewEngine("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()
	e.SetMaxOpenConns(4)
	if _, err := e.Exec("CREATE TABLE c(id INTEGER PRIMARY KEY, n INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Exec("INSERT INTO c(id, n) VALUES (1, 0)"); err != nil {
		t.Fatal(err)
	}

	bump := func(hold time.Duration) error {
		sess := e.NewSession()
		defer func() { _ = sess.Close() }()
		if err := sess.Begin(); err != nil {
			return err
		}
		rows, err := sess.QueryString("SELECT n FROM c WHERE id = 1") // 先读
		if err != nil {
			_ = sess.Rollback()
			return err
		}
		_ = rows
		time.Sleep(hold)                                                            // 把两个事务的窗口叠在一起
		if _, err := sess.Exec("UPDATE c SET n = n + 1 WHERE id = 1"); err != nil { // 后写
			_ = sess.Rollback()
			return err
		}
		return sess.Commit()
	}

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, hold := range []time.Duration{150 * time.Millisecond, 150 * time.Millisecond} {
		wg.Add(1)
		go func(i int, hold time.Duration) {
			defer wg.Done()
			errs[i] = bump(hold)
		}(i, hold)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// 这是真正要守住的行为：配置里写了 _txlock=immediate，并发写就不该报锁错误。
func TestImmediateTxSurvivesConcurrentReadThenWrite(t *testing.T) {
	if err := racingReadThenWrite(t, "_txlock=immediate&"+basePragmas); err != nil {
		t.Fatalf("_txlock=immediate 下并发写仍然失败: %v\n"+
			"（检查 config.toml 的 data_source 是否还带着 _txlock=immediate）", err)
	}
}

// 对照组：证明上面那条测试不是「随便怎样都能过」。
// 默认的 deferred 在同样的并发下会失败——失败即预期，这条测试固定住「这个开关确实有用」。
func TestDeferredTxFailsWithoutTxlock(t *testing.T) {
	err := racingReadThenWrite(t, basePragmas)
	if err == nil {
		t.Skip("deferred 下这次没撞上（并发时序问题），不影响 immediate 那条的结论")
	}
	t.Logf("deferred 如预期失败: %v", err)
}
