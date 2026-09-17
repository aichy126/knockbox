package service

import (
	"path/filepath"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// newTestDAO 起一个临时库，返回 DAO 和它所在的目录——
// 目录是给「去文件里 grep 已删内容」那类测试用的。
func newTestDAO(t *testing.T) (*dao.DAO, string) {
	t.Helper()
	dir := t.TempDir()
	e, err := xorm.NewEngine("sqlite3", filepath.Join(dir, "test.db")+"?"+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	return dao.New(e), dir
}

func mustUser(t *testing.T, d *dao.DAO) int64 {
	t.Helper()
	u, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	return u.Id
}

func mustChannel(t *testing.T, d *dao.DAO, uid int64) string {
	t.Helper()
	ch, err := NewChannel(d).Create(uid, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	return ch.Id
}
