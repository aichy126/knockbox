package service

import (
	"testing"

	"github.com/aichy126/knockbox/internal/models"
)

// 局部更新：只给哪个字段就只改哪个，其余原样留着。
//
// 这条以前由「按给了哪些字段拼 SET 子句」保证，现在由 COALESCE(?, 列) 保证。
// 两种写法都可能出错，而出错的样子是一样的：用户改了个备注，静音设置被清掉了，
// 而且不报错——所以每个字段都要有一条断言钉住。
func TestChannelUpdateLeavesUntouchedFieldsAlone(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := NewChannel(d)

	str := func(s string) *string { return &s }
	b := func(v bool) *bool { return &v }
	i64 := func(v int64) *int64 { return &v }

	created, err := ch.Create(uid, ChannelInput{
		Meta: str(`{"name":"构建"}`), Muted: b(true), MuteUntil: i64(1893456000),
		Sound: str("bell"), Level: str(models.LevelActive),
	})
	if err != nil {
		t.Fatal(err)
	}

	// 只动 meta
	got, err := ch.Update(uid, created.Id, ChannelInput{Meta: str(`{"name":"部署"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Meta != `{"name":"部署"}` {
		t.Errorf("meta 没改成功：%q", got.Meta)
	}
	if got.Muted != 1 {
		t.Error("只改 meta 却把静音清掉了")
	}
	if got.MuteUntil != 1893456000 {
		t.Errorf("只改 meta 却动了 mute_until：%d", got.MuteUntil)
	}
	if got.Sound != "bell" {
		t.Errorf("只改 meta 却动了 sound：%q", got.Sound)
	}
	if got.Level != models.LevelActive {
		t.Errorf("只改 meta 却动了 level：%q", got.Level)
	}

	// 只动 muted
	got, err = ch.Update(uid, created.Id, ChannelInput{Muted: b(false)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Muted != 0 {
		t.Error("静音没关掉")
	}
	if got.Meta != `{"name":"部署"}` || got.Sound != "bell" {
		t.Errorf("只改静音却动了别的：meta=%q sound=%q", got.Meta, got.Sound)
	}
}

// 空串和「没传」是两回事：前者是把这个字段清空，后者是别动它。
// 这正是 ChannelInput 里那几个字段用指针的理由，COALESCE 必须保住它
// —— 空串不是 SQL NULL，所以会照常写进去。
func TestChannelUpdateTellsEmptyApartFromAbsent(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := NewChannel(d)
	str := func(s string) *string { return &s }

	created, err := ch.Create(uid, ChannelInput{Meta: str(`{"name":"x"}`), Sound: str("bell")})
	if err != nil {
		t.Fatal(err)
	}

	got, err := ch.Update(uid, created.Id, ChannelInput{Sound: str("")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Sound != "" {
		t.Errorf("传空串应当把 sound 清空，得到 %q", got.Sound)
	}
	if got.Meta != `{"name":"x"}` {
		t.Errorf("清空 sound 不该动 meta：%q", got.Meta)
	}
}

// mute_until 传 0 是「取消定时静音」，不是「没传」。
// 0 同样不是 NULL，所以它必须写得进去。
func TestChannelUpdateZeroMuteUntilClearsIt(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := NewChannel(d)
	i64 := func(v int64) *int64 { return &v }

	created, err := ch.Create(uid, ChannelInput{MuteUntil: i64(1893456000)})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ch.Update(uid, created.Id, ChannelInput{MuteUntil: i64(0)})
	if err != nil {
		t.Fatal(err)
	}
	if got.MuteUntil != 0 {
		t.Errorf("传 0 应当取消定时静音，得到 %d", got.MuteUntil)
	}
}
