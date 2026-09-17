package service

import (
	"testing"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/uierr"
)

// 这一批是 characterization 测试：后台的 SQL 从 handler 搬进 service 时，
// 拿它们钉住「搬完之后查出来的还是同一批东西」。
//
// 原来这些语句一行都没有被测到（internal/api 里拼 HTML 的那堆覆盖率是 0），
// 所以搬家本身就是给它们补测试的机会——列名拼错在 QueryString 时代是静默的零值，
// 换成带类型的结构体之后仍然是静默的零值，只有断言能发现。

func mustMember(t *testing.T, d *dao.DAO, name string) int64 {
	t.Helper()
	u, err := NewAdmin(d).CreateMember(name)
	if err != nil {
		t.Fatal(err)
	}
	return u.Id
}

func mustSend(t *testing.T, d *dao.DAO, chID string, in SendInput) string {
	t.Helper()
	var ch models.Channel
	if ok, err := d.Engine().Where("id=?", chID).Get(&ch); err != nil || !ok {
		t.Fatalf("频道 %s 不存在: %v", chID, err)
	}
	out, err := NewSend(d).Deliver(&ch, in, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	return out.UID
}

func TestAdminMembersCountsBelongings(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	owner := mustUser(t, d)
	ch := mustChannel(t, d, owner)
	mustSend(t, d, ch, SendInput{Title: "一"})
	mustSend(t, d, ch, SendInput{Title: "二"})
	mustMember(t, d, "另一个人")

	rows, total, err := a.Members(MemberFilter{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("总数应当是 2，得到 %d", total)
	}
	var got *MemberRow
	for i := range rows {
		if rows[i].Id == owner {
			got = &rows[i]
		}
	}
	if got == nil {
		t.Fatal("成员列表里没有 owner")
	}
	// 三个子查询各钉一条：列名拼错时它们会静默变成 0
	if got.Channels != 1 {
		t.Errorf("频道数应当是 1，得到 %d", got.Channels)
	}
	if got.Messages != 2 {
		t.Errorf("消息数应当是 2，得到 %d", got.Messages)
	}
	if got.Devices != 0 {
		t.Errorf("设备数应当是 0，得到 %d", got.Devices)
	}
	if got.Name == "" || got.Role == "" {
		t.Errorf("名字或角色是空的：%+v", got)
	}
}

func TestAdminMembersSearchAndPaging(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	mustUser(t, d)
	mustMember(t, d, "张三")
	mustMember(t, d, "张四")

	rows, total, err := a.Members(MemberFilter{Query: "张", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("搜「张」应当命中 2 个，得到 total=%d len=%d", total, len(rows))
	}
	// total 是【匹配到的】总数，不是全表总数——分页器靠它算页数
	if _, total, _ = a.Members(MemberFilter{Query: "不存在的名字", Limit: 50}); total != 0 {
		t.Errorf("搜不到时 total 应当是 0，得到 %d", total)
	}
	// offset 生效
	rows, _, _ = a.Members(MemberFilter{Query: "张", Limit: 1, Offset: 1})
	if len(rows) != 1 {
		t.Fatalf("limit=1 offset=1 应当返回 1 行，得到 %d", len(rows))
	}
}

func TestAdminMessagesFilters(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	owner := mustUser(t, d)
	other := mustMember(t, d, "别人")
	ch1 := mustChannel(t, d, owner)
	ch2 := mustChannel(t, d, other)

	uid1 := mustSend(t, d, ch1, SendInput{Title: "苹果派", Body: "烤箱"})
	mustSend(t, d, ch1, SendInput{Title: "香蕉船"})
	mustSend(t, d, ch2, SendInput{Title: "苹果汁"})

	all, err := a.Messages(MessageFilter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	// 【不按登录者过滤】：管理界面要看到这台服务器上的全部消息
	if len(all) != 3 {
		t.Fatalf("不筛时应当有 3 条（含别人的），得到 %d", len(all))
	}
	// owner 这一列来自子查询，拼错就是空串
	for _, m := range all {
		if m.Owner == "" {
			t.Errorf("消息 %s 的 owner 是空的", m.UID)
		}
	}

	if got, _ := a.Messages(MessageFilter{Query: "苹果", Limit: 100}); len(got) != 2 {
		t.Errorf("搜「苹果」应当命中 2 条，得到 %d", len(got))
	}
	// 正文也在搜索范围里
	if got, _ := a.Messages(MessageFilter{Query: "烤箱", Limit: 100}); len(got) != 1 {
		t.Errorf("搜正文「烤箱」应当命中 1 条，得到 %d", len(got))
	}
	if got, _ := a.Messages(MessageFilter{ChannelId: ch2, Limit: 100}); len(got) != 1 {
		t.Errorf("按频道筛应当命中 1 条，得到 %d", len(got))
	}
	if got, _ := a.Messages(MessageFilter{UserId: other, Limit: 100}); len(got) != 1 {
		t.Errorf("按成员筛应当命中 1 条，得到 %d", len(got))
	}

	// 游标：倒序，Before 取更早的
	first, _ := a.Messages(MessageFilter{Limit: 1})
	if len(first) != 1 {
		t.Fatal("limit=1 应当返回 1 条")
	}
	rest, _ := a.Messages(MessageFilter{Before: first[0].Id, Limit: 100})
	if len(rest) != 2 {
		t.Errorf("游标之后应当还剩 2 条，得到 %d", len(rest))
	}
	for _, m := range rest {
		if m.Id >= first[0].Id {
			t.Errorf("游标没生效：拿到了 id=%d，而游标是 %d", m.Id, first[0].Id)
		}
	}

	// WithBody 决定带不带正文：列表页不需要，一页 40 条正文是白花的流量
	lite, _ := a.Messages(MessageFilter{ChannelId: ch1, Limit: 100})
	for _, m := range lite {
		if m.Body != "" {
			t.Errorf("没要正文却带回来了：%q", m.Body)
		}
	}
	full, _ := a.Messages(MessageFilter{ChannelId: ch1, Limit: 100, WithBody: true})
	var found bool
	for _, m := range full {
		if m.UID == uid1 && m.Body == "烤箱" {
			found = true
		}
	}
	if !found {
		t.Error("WithBody 没把正文带回来")
	}
}

func TestAdminMessageNotFound(t *testing.T) {
	d, _ := newTestDAO(t)
	_, _, err := NewAdmin(d).Message("不存在的uid")
	if e, ok := uierr.As(err); !ok || e.Code != uierr.MessageNotFound {
		t.Fatalf("应当返回 message.not_found，得到 %v", err)
	}
}

// 详情不按归属过滤：搜索页跨全站列出来的消息，详情页必须都打得开。
func TestAdminMessageIgnoresOwnership(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	mustUser(t, d) // 管理员自己，id=1
	other := mustMember(t, d, "别人")
	ch := mustChannel(t, d, other)
	uid := mustSend(t, d, ch, SendInput{Title: "别人的消息"})

	m, _, err := a.Message(uid)
	if err != nil {
		t.Fatalf("别人的消息应当能打开：%v", err)
	}
	if m.UserId == 1 {
		t.Fatal("这条消息本该属于别人，测试前提不成立")
	}
}

func TestAdminChannelAndOwner(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	owner := mustUser(t, d)
	chID := mustChannel(t, d, owner)

	ch, u, err := a.Channel(chID)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Id != chID || u.Id != owner || u.Name == "" {
		t.Errorf("频道或属主不对：ch=%+v owner=%+v", ch, u)
	}

	if _, _, err := a.Channel("没有这个频道"); err == nil {
		t.Error("频道不存在时应当报错")
	} else if e, ok := uierr.As(err); !ok || e.Code != uierr.ChannelNotFound {
		t.Errorf("应当是 channel.not_found，得到 %v", err)
	}
}

func TestAdminMemberChannelsCounts(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	owner := mustUser(t, d)
	chID := mustChannel(t, d, owner)
	mustSend(t, d, chID, SendInput{Title: "一"})

	rows, err := a.MemberChannels(owner)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("应当有 1 个频道，得到 %d", len(rows))
	}
	if rows[0].Messages != 1 {
		t.Errorf("消息数应当是 1，得到 %d", rows[0].Messages)
	}
	// last 走 COALESCE(MAX(...),0)：没有消息的频道不该让整行解析失败
	if rows[0].LastMsgAt == 0 {
		t.Error("最后一条消息的时间不该是 0")
	}
	empty := mustChannel(t, d, owner)
	rows, _ = a.MemberChannels(owner)
	var e *ChannelRow
	for i := range rows {
		if rows[i].Id == empty {
			e = &rows[i]
		}
	}
	if e == nil {
		t.Fatal("空频道没被列出来")
	}
	if e.LastMsgAt != 0 {
		t.Errorf("空频道的 last 应当是 0，得到 %d", e.LastMsgAt)
	}
}

func TestAdminOverviewCounts(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	owner := mustUser(t, d)
	ch := mustChannel(t, d, owner)
	mustSend(t, d, ch, SendInput{Title: "一"})
	mustSend(t, d, ch, SendInput{Title: "二"})

	const window = int64(24 * 3600)
	o, err := a.Overview(owner, time.Now().Unix()-window, window)
	if err != nil {
		t.Fatal(err)
	}
	if o.Messages != 2 {
		t.Errorf("窗口内消息应当是 2，得到 %d", o.Messages)
	}
	if o.MessagesPrev != 0 {
		t.Errorf("上一个窗口应当是 0，得到 %d", o.MessagesPrev)
	}
	if o.Channels != 1 {
		t.Errorf("频道应当是 1，得到 %d", o.Channels)
	}
	// 没有设备就没有推送记录：成功率应当是「没有」而不是 0%
	if _, ok := o.Rate(); ok {
		t.Error("一次推送都没有时，成功率应当是「没有」")
	}
}

func TestAdminMemberIdByNameFallsBackToNoFilter(t *testing.T) {
	d, _ := newTestDAO(t)
	a := NewAdmin(d)
	owner := mustUser(t, d)

	if got := a.MemberIdByName("owner"); got != owner {
		t.Errorf("按名字应当查到 %d，得到 %d", owner, got)
	}
	// 查不到返回 0 = 不筛选。返回空列表的话，名字打错的人会以为「这个人没有消息」
	if got := a.MemberIdByName("名字打错了"); got != 0 {
		t.Errorf("查不到应当返回 0，得到 %d", got)
	}
	if got := a.MemberIdByName(""); got != 0 {
		t.Errorf("空名字应当返回 0，得到 %d", got)
	}
}

func TestAdminMemberNotFound(t *testing.T) {
	d, _ := newTestDAO(t)
	_, err := NewAdmin(d).Member(999999)
	if e, ok := uierr.As(err); !ok || e.Code != uierr.MemberNotFound {
		t.Fatalf("应当返回 member.not_found，得到 %v", err)
	}
}
