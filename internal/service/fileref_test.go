package service

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// 附件的生命周期挂在【引用计数】上，不是挂在某一条消息上。
//
// 这一组测试存在的理由：内容寻址意味着同一张图可能被多条消息共用，
// 「删了消息就删图」在共用时会把别人的图一起删掉。而反过来，
// 引用减到 0 却不扫地，磁盘会无声地涨。两个方向都只能靠测试守住——
// 肉眼看数据库里的行，看不出磁盘上的 blob 还在不在。

func newFileEnv(t *testing.T) (*dao.DAO, *File, string) {
	t.Helper()
	dir := t.TempDir()
	e, err := xorm.NewEngine("sqlite3", filepath.Join(dir, "t.db")+"?"+pragmas)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	d := dao.New(e)
	blobs := filepath.Join(dir, "files")
	return d, NewFile(d, blobs, 64, 1600, 600, 70, "http://x", 3600), blobs
}

func pngBytes(c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func blobCount(t *testing.T, dir string) int {
	t.Helper()
	n := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func refOf(t *testing.T, d *dao.DAO, uid string) int64 {
	t.Helper()
	var f models.File
	has, err := d.Engine().Where("uid = ?", uid).Get(&f)
	if err != nil || !has {
		t.Fatalf("找不到 file %s（err=%v）", uid, err)
	}
	return f.RefCount
}

// 两条消息共用同一张图时，删掉其中一条不能把 blob 删掉——
// 另一条还要用它。这是内容寻址最容易写错的地方。
func TestSharedAttachmentSurvivesDeletingOneMessage(t *testing.T) {
	d, files, blobs := newFileEnv(t)
	u, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	a, err := NewChannel(d).Create(u.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewChannel(d).Create(u.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := files.Store(u.Id, "chart.png", pngBytes(color.RGBA{R: 200, G: 30, B: 30, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	before := blobCount(t, blobs)
	if before == 0 {
		t.Fatal("Store 之后磁盘上应当有 blob")
	}

	s := NewSend(d)
	m1, err := s.Deliver(a, SendInput{Title: "一", File: f.UID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Deliver(b, SendInput{Title: "二", File: f.UID}, ""); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, d, f.UID); got != 2 {
		t.Fatalf("两条消息引用同一张图，ref_count 应为 2，得到 %d", got)
	}

	if _, err := NewSync(d).Delete(u.Id, []string{m1.UID}); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, d, f.UID); got != 1 {
		t.Fatalf("删掉一条消息后 ref_count 应为 1，得到 %d", got)
	}

	n, err := files.ReleaseUnused()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("还有人引用这张图，不该被回收，却删了 %d 个", n)
	}
	if got := blobCount(t, blobs); got != before {
		t.Fatalf("blob 数从 %d 变成 %d：另一条消息的图被误删了", before, got)
	}
}

// 最后一个引用消失后，blob 必须真的从磁盘上消失，file 表那行也要删掉。
func TestLastReferenceReleasesBlobFromDisk(t *testing.T) {
	d, files, blobs := newFileEnv(t)
	u, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannel(d).Create(u.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := files.Store(u.Id, "chart.png", pngBytes(color.RGBA{R: 20, G: 120, B: 220, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewSend(d).Deliver(ch, SendInput{Title: "只此一条", File: f.UID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, d, f.UID); got != 1 {
		t.Fatalf("ref_count 应为 1，得到 %d", got)
	}

	if _, err := NewSync(d).Delete(u.Id, []string{m.UID}); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, d, f.UID); got != 0 {
		t.Fatalf("最后一个引用删掉后 ref_count 应为 0，得到 %d", got)
	}
	if _, err := files.ReleaseUnused(); err != nil {
		t.Fatal(err)
	}
	if got := blobCount(t, blobs); got != 0 {
		t.Fatalf("没人引用了，磁盘上还剩 %d 个 blob", got)
	}
	var left models.File
	has, _ := d.Engine().Where("uid = ?", f.UID).Get(&left)
	if has {
		t.Error("blob 删了，file 表那行还在：下次扫描会反复尝试删一个不存在的文件")
	}
}

// 清空频道同样要走引用计数这条路。
func TestPurgeChannelReleasesAttachments(t *testing.T) {
	d, files, blobs := newFileEnv(t)
	u, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannel(d).Create(u.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := files.Store(u.Id, "a.png", pngBytes(color.RGBA{R: 9, G: 9, B: 9, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSend(d).Deliver(ch, SendInput{Title: "x", File: f.UID}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSync(d).PurgeChannel(u.Id, ch.Id, 0); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, d, f.UID); got != 0 {
		t.Fatalf("清空频道后 ref_count 应为 0，得到 %d", got)
	}
	if _, err := files.ReleaseUnused(); err != nil {
		t.Fatal(err)
	}
	if got := blobCount(t, blobs); got != 0 {
		t.Fatalf("清空频道后磁盘上还剩 %d 个 blob", got)
	}
}

// 附件表是全站共用的，而频道 token 只能写自己的频道。
// 少了归属这一句判断，持有任意一个 token 的人只要拿到别人的附件 uid，
// 就能把别人的附件挂到自己的消息上——多人共用的实例上这是跨账户的。
func TestDeliverRejectsAnotherUsersAttachment(t *testing.T) {
	d, files, _ := newFileEnv(t)
	owner, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewAccount(d).Create("other", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	f, err := files.Store(owner.Id, "private.png", pngBytes(color.RGBA{R: 3, G: 3, B: 3, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannel(d).Create(other.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := NewSend(d).Deliver(ch, SendInput{Title: "借一张", File: f.UID}, ""); err == nil {
		t.Fatal("引用别人的附件必须被拒，不能照发")
	}
	if got := refOf(t, d, f.UID); got != 0 {
		t.Fatalf("被拒的引用不该记到别人的附件上，ref_count = %d", got)
	}
	var msgs int64
	if msgs, err = d.Engine().Where("channel_id = ?", ch.Id).Count(&models.Message{}); err != nil {
		t.Fatal(err)
	}
	if msgs != 0 {
		t.Fatalf("整条消息都不该落库，却存下了 %d 条", msgs)
	}

	// 自己的附件照常挂得上，别把正常路径一起挡掉。
	own, err := NewChannel(d).Create(owner.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSend(d).Deliver(own, SendInput{Title: "自己的", File: f.UID}, ""); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, d, f.UID); got != 1 {
		t.Fatalf("自己的附件应当挂上，ref_count = %d", got)
	}
}
