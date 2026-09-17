package service

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/models"
)

// 「上传」和「发送」是两次请求：/upload 存在的理由就是先传一次、再分别发给 N 个频道，
// 间隔由调用方决定。这段时间里附件的 ref_count 是 0，而回收只看这个数——
// 只要撞上一轮 GC，消息就会引用到一个已经被扫掉的 blob。
// 两边都看不出问题：数据库里的消息好好的，点开才发现图没了。
func TestFreshUploadSurvivesGCBeforeItIsSent(t *testing.T) {
	d, files, blobs := newFileEnv(t)
	u, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannel(d).Create(u.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}

	f, err := files.Store(u.Id, "chart.png", pngBytes(color.RGBA{R: 1, G: 2, B: 3, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	before := blobCount(t, blobs)
	if before == 0 {
		t.Fatal("Store 之后磁盘上应当有 blob")
	}

	// 上传完、消息还没发出去，这时候 GC 跑了一轮
	n, err := files.ReleaseUnused()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("刚上传还没来得及发的附件被回收了（删了 %d 个）", n)
	}
	if got := blobCount(t, blobs); got != before {
		t.Fatalf("blob 数从 %d 变成 %d：消息发出去之前附件就没了", before, got)
	}

	// 随后消息才发出去，附件必须还在
	if _, err := NewSend(d).Deliver(ch, SendInput{Title: "报表", File: f.UID}, ""); err != nil {
		t.Fatal(err)
	}
	if got := refOf(t, d, f.UID); got != 1 {
		t.Fatalf("消息发出后 ref_count 应为 1，得到 %d", got)
	}
	if got := blobCount(t, blobs); got != before {
		t.Fatalf("blob 不见了，剩 %d 个", got)
	}
}

// 宽限期不是永不回收：确实没人要的上传，过了期限还是要扫掉，
// 否则失败的上传会无限堆在磁盘上。
func TestNeverReferencedUploadIsReclaimedAfterGrace(t *testing.T) {
	d, files, blobs := newFileEnv(t)
	f, err := files.Store(1, "junk.png", pngBytes(color.RGBA{R: 9, G: 9, B: 9, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-orphanGrace - time.Hour).Unix()
	if _, err := d.Engine().Exec("UPDATE file SET created_at = ? WHERE id = ?", old, f.Id); err != nil {
		t.Fatal(err)
	}

	n, err := files.ReleaseUnused()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("过了宽限期的无主附件应当被回收 1 个，实际 %d 个", n)
	}
	if got := blobCount(t, blobs); got != 0 {
		t.Fatalf("磁盘上还剩 %d 个 blob", got)
	}
}

// 引用过、现在降回 0 的附件立即回收——宽限期只针对「从来没被引用过」的，
// 不能因为加了宽限期就把正常的回收路径拖慢一天。
func TestReleasedAttachmentIsReclaimedImmediately(t *testing.T) {
	d, files, blobs := newFileEnv(t)
	u, err := NewAccount(d).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannel(d).Create(u.Id, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	f, err := files.Store(u.Id, "a.png", pngBytes(color.RGBA{R: 40, G: 50, B: 60, A: 255}))
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewSend(d).Deliver(ch, SendInput{Title: "x", File: f.UID}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSync(d).Delete(u.Id, []string{m.UID}); err != nil {
		t.Fatal(err)
	}

	// created_at 还是「刚刚」，但它被引用过，所以不该受宽限期保护
	n, err := files.ReleaseUnused()
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("引用降回 0 的附件应当立即回收，实际回收 %d 个", n)
	}
	if got := blobCount(t, blobs); got != 0 {
		t.Fatalf("磁盘上还剩 %d 个 blob", got)
	}
}

// 回收要先删行再删文件。反过来的话，两步之间 Store 会按 sha256 命中那一行，
// 于是新消息引用到一个磁盘上已经不存在的附件——而这种坏法在接口上完全看不出来，
// 要等用户点开图才暴露。
func TestReclaimLeavesNoRowPointingAtMissingBlob(t *testing.T) {
	d, files, blobs := newFileEnv(t)
	raw := pngBytes(color.RGBA{R: 7, G: 7, B: 7, A: 255})
	f, err := files.Store(1, "same.png", raw)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-orphanGrace - time.Hour).Unix()
	if _, err := d.Engine().Exec("UPDATE file SET created_at = ? WHERE id = ?", old, f.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := files.ReleaseUnused(); err != nil {
		t.Fatal(err)
	}

	// 同样的内容再传一次：应当是一条全新的记录，而且磁盘上真的有文件
	again, err := files.Store(1, "same.png", raw)
	if err != nil {
		t.Fatal(err)
	}
	if blobCount(t, blobs) == 0 {
		t.Fatal("重新上传之后磁盘上没有 blob：命中了一条指向已删文件的记录")
	}
	fh, _, err := files.Open(again, false)
	if err != nil {
		t.Fatalf("file 表里的行指向一个不存在的文件: %v", err)
	}
	_ = fh.Close()
	if _, err := os.Stat(filepath.Join(blobs, again.Path)); err != nil {
		t.Fatalf("blob 不在磁盘上: %v", err)
	}
}
