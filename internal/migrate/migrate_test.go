package migrate

import (
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	_ "github.com/aichy126/igo/db" // 注册 modernc 的 sqlite3 驱动
	"xorm.io/xorm"
)

// 用 DSN 本身，不要手抄：抄漏一条时报出来的错会指向「配置写错了」，
// 而实际上只是测试没抄全。
const pragmas = DSN

func open(t *testing.T, extra string) *xorm.Engine {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "test.db") + "?" + extra
	e, err := xorm.NewEngine("sqlite3", dsn)
	if err != nil {
		t.Fatalf("打开数据库失败: %v", err)
	}
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestRunCreatesSchema(t *testing.T) {
	e := open(t, pragmas)
	if err := Run(e); err != nil {
		t.Fatalf("迁移失败: %v", err)
	}

	for _, table := range []string{
		"user", "device", "channel", "message", "file", "pair_code",
		"push_log", "purge_log", "session", "seq", "kv", "schema_migration",
	} {
		rows, err := e.QueryString(
			"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?", table)
		if err != nil || len(rows) == 0 {
			t.Errorf("缺少表 %s", table)
		}
	}

	// 断言「全部迁移都应用了」，而不是写死某个版本号——
	// 写死的话每加一个迁移都要回来改测试，那它就守不住任何东西了。
	all, err := load()
	if err != nil {
		t.Fatal(err)
	}
	want := all[len(all)-1].version
	got, err := Applied(e)
	if err != nil || got != want {
		t.Errorf("已应用版本 = %d, %v; 期望 %d（sql/ 下的最高版本）", got, err, want)
	}
}

// 迁移必须可以重复执行——部署脚本、容器重启都会再跑一遍。
func TestRunIsIdempotent(t *testing.T) {
	e := open(t, pragmas)
	if err := Run(e); err != nil {
		t.Fatalf("首次迁移失败: %v", err)
	}
	if err := Run(e); err != nil {
		t.Fatalf("重复迁移失败: %v", err)
	}
	all, _ := load()
	want := strconv.Itoa(len(all))
	rows, _ := e.QueryString("SELECT COUNT(*) AS n FROM schema_migration")
	if rows[0]["n"] != want {
		t.Errorf("schema_migration 有 %s 行; 期望 %s（重复执行不该重复记录）", rows[0]["n"], want)
	}
}

// 「清空频道 = 物理删除」完全依赖 secure_delete。
// 它写漏了不会报错，只会让删掉的内容仍然能从文件里读出来，所以必须启动即失败。
func TestBootstrapRejectsMissingSecureDelete(t *testing.T) {
	e := open(t, "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	err := Run(e)
	if err == nil {
		t.Fatal("secure_delete 缺失时应该报错")
	}
	if !strings.Contains(err.Error(), "secure_delete") {
		t.Errorf("错误信息应指明是哪个 pragma, 得到: %v", err)
	}
}

// igo 只在 DSN 完全没有 _pragma= 时才自动补 busy_timeout 和 journal_mode。
// 这条测试固定住那个行为：一旦写了自己的 pragma，就必须写全。
func TestBootstrapRejectsPartialPragmas(t *testing.T) {
	e := open(t, "_pragma=secure_delete(ON)")
	if err := Run(e); err == nil {
		t.Fatal("只写 secure_delete 时应该报错（journal_mode 不会是 WAL）")
	}
}

func TestLoadRejectsBadFilenames(t *testing.T) {
	ms, err := load()
	if err != nil {
		t.Fatalf("加载迁移失败: %v", err)
	}
	if len(ms) == 0 {
		t.Fatal("没有加载到任何迁移")
	}
	for i := 1; i < len(ms); i++ {
		if ms[i].version <= ms[i-1].version {
			t.Errorf("迁移未按版本号升序: %d 在 %d 之后", ms[i].version, ms[i-1].version)
		}
	}
}
