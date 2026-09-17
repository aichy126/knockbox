// Package migrate 版本化 schema 迁移。
//
// 为什么不用 xorm 的 Sync2：SQLite 的 ALTER TABLE 能力极弱（不能改列类型、不能删约束），
// Sync2 在 SQLite 下会静默跳过很多变更，给人「迁移成功了」的假象。
// 所以 DDL 由 sql/*.sql 独占，models 里的结构体只负责列映射，不负责建表。
package migrate

import (
	"embed"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aichy126/igo/log"
	"xorm.io/xorm"
)

//go:embed sql/*.sql
var files embed.FS

type migration struct {
	version int
	name    string
	body    string
}

// DSN 是这台服务需要的完整 pragma 串。
//
// 提成常量是因为 bootstrap 会在缺任何一项时拒绝启动：若测试里手抄一份而漏掉
// busy_timeout，报出来的错会指向「配置写错了」，实际上只是没抄全。
// 一份定义，两边引用。
const DSN = "_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=secure_delete(ON)" +
	"&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=auto_vacuum(incremental)"

// Run 应用所有未执行的迁移。幂等：已应用的版本会被跳过。
func Run(engine *xorm.Engine) error {
	if err := bootstrap(engine); err != nil {
		return err
	}
	if _, err := engine.Exec(`CREATE TABLE IF NOT EXISTS schema_migration (
		version INTEGER PRIMARY KEY, name TEXT NOT NULL, applied_at INTEGER NOT NULL)`); err != nil {
		return fmt.Errorf("创建 schema_migration 表失败: %w", err)
	}

	applied, err := appliedVersions(engine)
	if err != nil {
		return err
	}
	all, err := load()
	if err != nil {
		return err
	}

	for _, m := range all {
		if applied[m.version] {
			continue
		}
		if err := apply(engine, m); err != nil {
			return fmt.Errorf("迁移 %04d_%s 失败: %w", m.version, m.name, err)
		}
	}
	return nil
}

// apply 在一个事务里执行一个迁移文件。
// SQLite 支持事务性 DDL（不像 MySQL），所以迁移失败会整个回滚，不会留下半截 schema。
func apply(engine *xorm.Engine, m migration) error {
	sess := engine.NewSession()
	defer func() { _ = sess.Close() }()
	if err := sess.Begin(); err != nil {
		return err
	}
	if _, err := sess.Exec(m.body); err != nil {
		_ = sess.Rollback()
		return err
	}
	if _, err := sess.Exec(
		"INSERT INTO schema_migration(version, name, applied_at) VALUES (?, ?, ?)",
		m.version, m.name, time.Now().Unix()); err != nil {
		_ = sess.Rollback()
		return err
	}
	return sess.Commit()
}

// bootstrap 断言连接级 pragma 是否按预期生效。
//
// 这些 pragma 全部由 DSN 上的 _pragma= 决定、逐连接生效，用一条 PRAGMA 语句去设是【无效】的：
// 连接池会让它设在 A 连接上、而真正干活的是 B 连接。auto_vacuum 尤其隐蔽——它只在
// 「空库 + 写入它的那条连接首次写入」时才落进文件头，设错了不报错，等到发现时库已非空、
// 只能整库 VACUUM 才能改。
//
// 所以这里只断言、不设置，让配置写漏在【启动时】就炸。
func bootstrap(engine *xorm.Engine) error {
	// igo 只在 DSN 完全没有 _pragma= 时才自动补 busy_timeout 和 journal_mode；
	// 一旦写了任意一条，其余的都要自己写全，而且它不补是静默的。
	hard := map[string]func(string) bool{
		"journal_mode":  func(v string) bool { return strings.EqualFold(v, "wal") },
		"secure_delete": func(v string) bool { return v == "1" || strings.EqualFold(v, "on") },
		"busy_timeout":  func(v string) bool { n, _ := strconv.Atoi(v); return n >= 1000 },
	}
	for pragma, ok := range hard {
		v, err := scalar(engine, "PRAGMA "+pragma)
		if err != nil {
			return fmt.Errorf("读取 PRAGMA %s 失败: %w", pragma, err)
		}
		if !ok(v) {
			return fmt.Errorf(
				"PRAGMA %s = %q 不符合要求，DSN 里的 _pragma= 写漏了。"+
					"完整的串见 config.toml.example", pragma, v)
		}
	}

	// auto_vacuum 只在建表前生效，对已有的库无法补救，所以只警告不中断。
	if v, err := scalar(engine, "PRAGMA auto_vacuum"); err == nil && v != "2" {
		log.Warn("auto_vacuum 未启用，删除的空间不会自动回收",
			log.Any("value", v),
			log.Any("fix", "新库请在 DSN 里加 _pragma=auto_vacuum(incremental)；已有的库需要整库 VACUUM"))
	}
	return nil
}

func scalar(engine *xorm.Engine, sql string) (string, error) {
	rows, err := engine.QueryString(sql)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("%s 无返回", sql)
	}
	for _, v := range rows[0] {
		return v, nil
	}
	return "", fmt.Errorf("%s 返回空行", sql)
}

func appliedVersions(engine *xorm.Engine) (map[int]bool, error) {
	rows, err := engine.QueryString("SELECT version FROM schema_migration")
	if err != nil {
		return nil, err
	}
	out := make(map[int]bool, len(rows))
	for _, r := range rows {
		n, err := strconv.Atoi(r["version"])
		if err != nil {
			return nil, fmt.Errorf("schema_migration 里有非法版本号 %q: %w", r["version"], err)
		}
		out[n] = true
	}
	return out, nil
}

// load 读出 sql/ 下全部迁移，按版本号升序。文件名形如 0001_init.sql。
func load() ([]migration, error) {
	entries, err := files.ReadDir("sql")
	if err != nil {
		return nil, err
	}
	var out []migration
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		base := strings.TrimSuffix(e.Name(), ".sql")
		idx := strings.Index(base, "_")
		if idx <= 0 {
			return nil, fmt.Errorf("迁移文件名格式应为 <版本号>_<名字>.sql，得到 %q", e.Name())
		}
		version, err := strconv.Atoi(base[:idx])
		if err != nil {
			return nil, fmt.Errorf("迁移文件 %q 的版本号非法: %w", e.Name(), err)
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("迁移版本号 %d 重复: %s 与 %s", version, prev, e.Name())
		}
		seen[version] = e.Name()

		body, err := files.ReadFile(path.Join("sql", e.Name()))
		if err != nil {
			return nil, err
		}
		out = append(out, migration{version: version, name: base[idx+1:], body: string(body)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// Applied 返回已应用的最高版本号。
func Applied(engine *xorm.Engine) (int, error) {
	v, err := scalar(engine, "SELECT COALESCE(MAX(version), 0) FROM schema_migration")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(v)
}
