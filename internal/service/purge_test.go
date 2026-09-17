package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// 金丝雀：发一条含独特字符串的消息，清空，然后直接在【数据库文件和 WAL 文件】里搜它。
//
// 这条测试存在的唯一理由是：SQLite 的 DELETE 默认只把页标记为空闲，内容仍然
// 留在文件里能被读出来。没有 secure_delete 和 wal_checkpoint，这个测试会挂——
// 而肉眼看「记录没了」和「内容真没了」完全一样。
func TestPurgeActuallyScrubsBytesFromDisk(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "t.db")
	e, err := xorm.NewEngine("sqlite3", dbPath+"?"+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close() }()
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	d := dao.New(e)

	const canary = "CANARY-7f3a9c-DO-NOT-SURVIVE"
	u, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannel(d).Create(u.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSend(d).Deliver(ch, SendInput{Title: "x", Body: canary}, ""); err != nil {
		t.Fatal(err)
	}

	// 先确认它确实写进了文件——否则这条测试可能只是没找到而已。
	if !grepFiles(t, dir, canary) {
		t.Fatal("消息应当先出现在数据库文件里，否则这条测试证明不了任何事")
	}

	res, err := NewSync(d).PurgeChannel(u.Id, ch.Id, 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Deleted != 1 {
		t.Fatalf("应删除 1 条，得到 %d", res.Deleted)
	}
	if grepFiles(t, dir, canary) {
		t.Error("清空后正文仍能从数据库文件里读出来：secure_delete 或 wal_checkpoint 没生效")
	}
}

func grepFiles(t *testing.T, dir, needle string) bool {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err == nil && strings.Contains(string(b), needle) {
			return true
		}
	}
	return false
}

// 清空要留下频道级的事件，别的设备才学得到「这些没了」。
func TestPurgeLeavesSyncEventForOtherDevices(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	s := NewSend(d)
	for i := 0; i < 3; i++ {
		if _, err := s.Deliver(ch, SendInput{Body: "x"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	sync := NewSync(d)
	res, err := sync.PurgeChannel(ch.UserId, ch.Id, 0)
	if err != nil {
		t.Fatal(err)
	}
	out, err := sync.Since(ch.UserId, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Purges) != 1 {
		t.Fatalf("同步流里应有 1 条清空事件，得到 %d", len(out.Purges))
	}
	if out.Purges[0].BeforeMsgID != res.BeforeMsgID {
		t.Errorf("清空事件的边界应为 %d，得到 %d", res.BeforeMsgID, out.Purges[0].BeforeMsgID)
	}
	if len(out.Messages) != 0 {
		t.Errorf("硬删之后同步流里不该还有消息，得到 %d 条", len(out.Messages))
	}
}

// 清空过程中到达的新消息不能被静默吃掉——删的是「id <= 快照」而不是「全部」。
func TestPurgeSpareaMessagesArrivingAfterSnapshot(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	s := NewSend(d)
	var lastID int64
	for i := 0; i < 3; i++ {
		if _, err := s.Deliver(ch, SendInput{Body: "old"}, ""); err != nil {
			t.Fatal(err)
		}
	}
	rows, _ := d.Engine().QueryString("SELECT MAX(id) AS n FROM message")
	for _, v := range rows[0] {
		_, _ = fmtSscan(v, &lastID)
	}
	// 拿到快照之后又来了一条
	if _, err := s.Deliver(ch, SendInput{Body: "new"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSync(d).PurgeChannel(ch.UserId, ch.Id, lastID); err != nil {
		t.Fatal(err)
	}
	var left int
	rows, _ = d.Engine().QueryString("SELECT COUNT(*) AS n FROM message")
	for _, v := range rows[0] {
		_, _ = fmtSscanInt(v, &left)
	}
	if left != 1 {
		t.Errorf("快照之后到达的那条应当保留，剩余 %d 条", left)
	}
}

// 删除单条 = 内容当场抹掉，只留不含内容的空壳墓碑。
func TestDeleteScrubsContentButKeepsTombstone(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	res, err := NewSend(d).Deliver(ch, SendInput{Title: "标题", Body: "正文"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sync := NewSync(d)
	if n, err := sync.Delete(ch.UserId, []string{res.UID}); err != nil || n != 1 {
		t.Fatalf("删除应影响 1 条: n=%d err=%v", n, err)
	}
	var m models.Message
	has, err := d.Engine().Where("uid = ?", res.UID).Get(&m)
	if err != nil || !has {
		t.Fatal("墓碑行应当还在，供其它设备同步")
	}
	if m.Title != "" || m.Body != "" || m.Summary != "" {
		t.Errorf("内容应当已被抹掉，得到 title=%q body=%q summary=%q", m.Title, m.Body, m.Summary)
	}
	out, err := sync.Since(ch.UserId, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 || !out.Messages[0].Deleted || out.Messages[0].Body != "" {
		t.Errorf("同步流里应是一条不含内容的墓碑，得到 %+v", out.Messages)
	}
}
