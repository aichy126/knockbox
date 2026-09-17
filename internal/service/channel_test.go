package service

import (
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/models"
)

func ptr[T any](v T) *T { return &v }

// Meta 用指针是为了把「没传这个字段」和「传了空串」分开。
// 用字符串的话空串被当成「没传」，备注一旦写上就再也删不掉。
func TestUpdateCanClearMeta(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	s := NewChannel(d)

	ch, err := s.Create(uid, ChannelInput{Meta: ptr(`{"name":"构建通知"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if ch.Meta == "" {
		t.Fatal("建频道时的 meta 没存下来")
	}

	// 传空串 = 清空
	got, err := s.Update(uid, ch.Id, ChannelInput{Meta: ptr("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta != "" {
		t.Errorf("传空串应当把 meta 清掉，仍是 %q", got.Meta)
	}
}

// 不传 meta 时不能顺手把它抹掉——只改静音的请求不该动备注。
func TestUpdateLeavesMetaAloneWhenOmitted(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	s := NewChannel(d)

	const meta = `{"name":"告警"}`
	ch, err := s.Create(uid, ChannelInput{Meta: ptr(meta)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Update(uid, ch.Id, ChannelInput{Muted: ptr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta != meta {
		t.Errorf("没传 meta 时不该改动它：%q", got.Meta)
	}
	if got.Muted == 0 {
		t.Error("静音没生效")
	}
}

// 非法的打扰级别要在落库前挡下来，它会直接进 APNs payload。
func TestUpdateRejectsInvalidLevel(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	s := NewChannel(d)
	ch, err := s.Create(uid, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(uid, ch.Id, ChannelInput{Level: ptr("urgent")}); err == nil {
		t.Error("非法级别被接受了")
	}
	if _, err := s.Create(uid, ChannelInput{Level: ptr("urgent")}); err == nil {
		t.Error("建频道时也应当挡下非法级别")
	}
}

// 换凭据不影响频道身份，历史消息还挂在同一个频道上。
func TestRotateTokenKeepsIdentity(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	s := NewChannel(d)
	ch, err := s.Create(uid, ChannelInput{Meta: ptr(`{"name":"x"}`)})
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := s.RotateToken(uid, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.Id != ch.Id {
		t.Errorf("频道 id 变了：%s → %s", ch.Id, rotated.Id)
	}
	if rotated.Token == ch.Token {
		t.Error("token 没换")
	}
	if rotated.Meta != ch.Meta {
		t.Error("换 token 不该影响 meta")
	}
}

// 别人的频道碰不到。
func TestChannelScopedToOwner(t *testing.T) {
	d, _ := newTestDAO(t)
	owner := mustUser(t, d)
	other, err := NewAccount(d).Create("other", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := NewChannel(d).Create(owner, ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	s := NewChannel(d)
	if _, err := s.Get(other.Id, ch.Id); err == nil {
		t.Error("能读到别人的频道")
	}
	if _, err := s.Update(other.Id, ch.Id, ChannelInput{Muted: ptr(true)}); err == nil {
		t.Error("能改别人的频道")
	}
	if err := s.Delete(other.Id, ch.Id); err == nil {
		t.Error("能删别人的频道")
	}
}
