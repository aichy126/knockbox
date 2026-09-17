package service

import (
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/models"
)

// 增量同步的游标语义：out.Rev 是客户端下次该带回来的 since。
//
// 没取完时它必须停在「本页最后一条」，不能跳到全局水位线——
// 跳过去的话下一次请求会从水位线开始，中间那一段永远拉不到了。
func TestSinceCursorStopsAtLastRowWhenTruncated(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch, err := NewChannel(d).Create(uid, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	s := NewSend(d)
	for i := 0; i < 5; i++ {
		if _, err := s.Deliver(ch, SendInput{Body: "m"}, ""); err != nil {
			t.Fatal(err)
		}
	}

	sync := NewSync(d)
	page, err := sync.Since(uid, 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("limit=2 应当回 2 条，得到 %d", len(page.Messages))
	}
	if !page.HasMore {
		t.Fatal("还有 3 条没取，has_more 应当为真")
	}
	last := page.Messages[len(page.Messages)-1].Rev
	if page.Rev != last {
		t.Fatalf("没取完时游标应当停在本页最后一条 rev=%d，得到 %d", last, page.Rev)
	}

	// 用它继续拉，一条都不能漏
	seen := len(page.Messages)
	cursor := page.Rev
	for i := 0; i < 5 && seen < 5; i++ {
		page, err = sync.Since(uid, cursor, 2)
		if err != nil {
			t.Fatal(err)
		}
		seen += len(page.Messages)
		cursor = page.Rev
	}
	if seen != 5 {
		t.Fatalf("按游标翻完应当拿到 5 条，实际 %d 条", seen)
	}
	if page.HasMore {
		t.Error("翻完之后 has_more 应当为假")
	}
}

// 取完时游标是全局水位线，这样删除、已读这些改动也不会被漏掉。
func TestSinceCursorIsWatermarkWhenDrained(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch, err := NewChannel(d).Create(uid, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSend(d).Deliver(ch, SendInput{Body: "m"}, ""); err != nil {
		t.Fatal(err)
	}
	cur, err := d.CurrentRev()
	if err != nil {
		t.Fatal(err)
	}
	page, err := NewSync(d).Since(uid, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if page.HasMore {
		t.Fatal("只有一条，不该还有更多")
	}
	if page.Rev != cur {
		t.Errorf("取完时游标应当是全局水位线 %d，得到 %d", cur, page.Rev)
	}
}

// 删除要以墓碑的形式出现在同步流里，而且不带任何内容。
func TestSinceCarriesDeletionAsEmptyTombstone(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch, err := NewChannel(d).Create(uid, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewSend(d).Deliver(ch, SendInput{Title: "秘密", Body: "正文"}, "")
	if err != nil {
		t.Fatal(err)
	}
	sync := NewSync(d)
	first, err := sync.Since(uid, 0, 200)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sync.Delete(uid, []string{m.UID}); err != nil {
		t.Fatal(err)
	}

	page, err := sync.Since(uid, first.Rev, 200)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("删除应当作为一条变更出现，得到 %d 条", len(page.Messages))
	}
	v := page.Messages[0]
	if !v.Deleted {
		t.Error("没有标记为已删除")
	}
	if v.Title != "" || v.Body != "" || v.Summary != "" || v.Extra != "" {
		t.Errorf("墓碑不该带任何内容：%+v", v)
	}
}

// 客户端的 since 比 GC 水位线还旧，说明它缺的那块永远补不回来，必须全量重载。
func TestSinceResetsWhenBehindGCWatermark(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	if err := d.KVSet(models.KVGCWatermark, "100", 0); err != nil {
		t.Fatal(err)
	}
	page, err := NewSync(d).Since(uid, 50, 200)
	if err != nil {
		t.Fatal(err)
	}
	if !page.Reset {
		t.Error("since 落在水位线之前，应当要求客户端全量重载")
	}

	page, err = NewSync(d).Since(uid, 150, 200)
	if err != nil {
		t.Fatal(err)
	}
	if page.Reset {
		t.Error("since 在水位线之后，不该要求重载")
	}
}
