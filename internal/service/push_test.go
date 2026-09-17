package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	kapns "github.com/aichy126/knockbox/internal/library/apns"
	"github.com/aichy126/knockbox/internal/models"
)

// seedPushJob 造一条「待推」的记录：用户、频道、带 APNs token 的设备、消息、push_log。
func seedPushJob(t *testing.T, d *dao.DAO) (uid int64, ch string) {
	t.Helper()
	uid = mustUser(t, d)
	ch = mustChannel(t, d, uid)
	now := time.Now().Unix()

	dev := &models.Device{
		UUID: "dev-1", UserId: uid, APNsToken: "tok", APNsEnv: models.EnvProduction,
		AuthHash: "h", AuthPrefix: "p", Status: models.DeviceLive, Ctime: now, Utime: now,
	}
	if _, err := d.Engine().Insert(dev); err != nil {
		t.Fatal(err)
	}
	msg := &models.Message{
		UID: "m-1", UserId: uid, ChannelId: ch, Type: models.TypeText,
		Title: "t", Summary: "s", Body: "b", Rev: 1, Ctime: now,
	}
	if _, err := d.Engine().Insert(msg); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Engine().Insert(&models.PushLog{
		MessageId: msg.Id, DeviceId: dev.Id,
		Status: models.PushPending, Ctime: now, Utime: now,
	}); err != nil {
		t.Fatal(err)
	}
	return uid, ch
}

func pushStatus(t *testing.T, d *dao.DAO) int {
	t.Helper()
	var st int
	if _, err := d.Engine().SQL("SELECT status FROM push_log LIMIT 1").Get(&st); err != nil {
		t.Fatal(err)
	}
	return st
}

// 频道被删之后，指向它的投递记录无法再组装 payload。
//
// 这类记录必须被置终态。只是跳过的话，它会永远停在 pending：
// 每 30 秒一轮的兜底扫描都会把它捞出来、再原样丢掉，队列里攒下一批永不消失的幽灵。
func TestClaimDiscardsJobWhoseChannelIsGone(t *testing.T) {
	d, _ := newTestDAO(t)
	uid, ch := seedPushJob(t, d)
	if err := NewChannel(d).Delete(uid, ch); err != nil {
		t.Fatal(err)
	}

	p := NewPusher(d, nil, nil, PusherConfig{})
	jobs, err := p.claim(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("频道已删，不该还能组出任务，得到 %d 条", len(jobs))
	}
	if got := pushStatus(t, d); got != models.PushFailed {
		t.Fatalf("孤儿投递记录应当被置成终态 %d，实际还是 %d", models.PushFailed, got)
	}

	// 再扫一轮不应该再捞到它
	jobs, err = p.claim(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatal("已置终态的记录又被捞出来了")
	}
	var reason string
	_, _ = d.Engine().SQL("SELECT reason FROM push_log LIMIT 1").Get(&reason)
	if reason == "" {
		t.Error("终态记录没有写明原因，排障时无从判断")
	}
}

// 正常情况下 claim 要能把任务完整组出来，别为了修孤儿把正路也堵了。
func TestClaimBuildsJobWhenEverythingExists(t *testing.T) {
	d, _ := newTestDAO(t)
	seedPushJob(t, d)

	jobs, err := NewPusher(d, nil, nil, PusherConfig{}).claim(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("应当组出 1 条任务，得到 %d", len(jobs))
	}
	j := jobs[0]
	if j.msg.UID != "m-1" || j.dev.UUID != "dev-1" || j.ch.Id == "" {
		t.Fatalf("任务字段没填全: msg=%q dev=%q ch=%q", j.msg.UID, j.dev.UUID, j.ch.Id)
	}
	if got := pushStatus(t, d); got != models.PushPending {
		t.Fatalf("正常任务不该被置终态，status=%d", got)
	}
}

// 消息被保留策略清掉之后，指向它的投递记录同样要置终态。
//
// 原先三张表都是 INNER JOIN，这种行压根不出现在查询结果里，
// 于是永远停在 pending —— 看起来「没有孤儿」，其实是看不见。
func TestClaimDiscardsJobWhoseMessageIsGone(t *testing.T) {
	d, _ := newTestDAO(t)
	seedPushJob(t, d)
	if _, err := d.Engine().Exec("DELETE FROM message"); err != nil {
		t.Fatal(err)
	}

	p := NewPusher(d, nil, nil, PusherConfig{})
	jobs, err := p.claim(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("消息已删，不该组出任务，得到 %d 条", len(jobs))
	}
	if got := pushStatus(t, d); got != models.PushFailed {
		t.Fatalf("应当置终态 %d，实际 %d", models.PushFailed, got)
	}
}

func TestClaimDiscardsJobWhoseDeviceIsGone(t *testing.T) {
	d, _ := newTestDAO(t)
	seedPushJob(t, d)
	if _, err := d.Engine().Exec("DELETE FROM device"); err != nil {
		t.Fatal(err)
	}

	jobs, err := NewPusher(d, nil, nil, PusherConfig{}).claim(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("设备已删，不该组出任务，得到 %d 条", len(jobs))
	}
	if got := pushStatus(t, d); got != models.PushFailed {
		t.Fatalf("应当置终态 %d，实际 %d", models.PushFailed, got)
	}
}

// 设备还在、只是没有 APNs token（用户还没授权通知）：不取也不置终态，
// 等它重新上报 token 时这批会自然被捞起来。
func TestClaimSkipsDeviceWithoutTokenWithoutDiscarding(t *testing.T) {
	d, _ := newTestDAO(t)
	seedPushJob(t, d)
	if _, err := d.Engine().Exec("UPDATE device SET apns_token = ''"); err != nil {
		t.Fatal(err)
	}

	p := NewPusher(d, nil, nil, PusherConfig{})
	if jobs, err := p.claim(10); err != nil || len(jobs) != 0 {
		t.Fatalf("没有 token 的设备不该被取出：jobs=%d err=%v", len(jobs), err)
	}
	if got := pushStatus(t, d); got != models.PushPending {
		t.Fatalf("这不是终态失败，应当仍是 pending，实际 %d", got)
	}

	// token 回来之后应当能正常取出
	if _, err := d.Engine().Exec("UPDATE device SET apns_token = 'tok'"); err != nil {
		t.Fatal(err)
	}
	if jobs, err := p.claim(10); err != nil || len(jobs) != 1 {
		t.Fatalf("token 回来后应当能取出 1 条：jobs=%d err=%v", len(jobs), err)
	}
}

// 一次查询要把组装 payload 需要的字段全带回来。
// 少一列的表现是「推送发出去了但内容不对」，比报错更难发现。
func TestClaimCarriesEveryFieldThePayloadNeeds(t *testing.T) {
	d, _ := newTestDAO(t)
	uid := mustUser(t, d)
	ch := mustChannel(t, d, uid)
	now := time.Now().Unix()

	if _, err := d.Engine().Insert(&models.Device{
		UUID: "dev-x", UserId: uid, APNsToken: "tok-x", APNsEnv: models.EnvSandbox,
		AuthHash: "h", AuthPrefix: "p", Status: models.DeviceLive,
		Ctime: now, Utime: now + 5,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Engine().Exec(
		"UPDATE channel SET level = ?, sound = ? WHERE id = ?",
		models.LevelTimeSensitive, "ding.caf", ch); err != nil {
		t.Fatal(err)
	}
	msg := &models.Message{
		UID: "m-x", UserId: uid, ChannelId: ch, Type: models.TypeMarkdown,
		Title: "标题", Summary: "摘要", Body: "正文正文", Extra: `{"file":"f1"}`,
		CollapseId: "build-42", Rev: 7, Ctime: now - 30,
	}
	if _, err := d.Engine().Insert(msg); err != nil {
		t.Fatal(err)
	}
	var devID int64
	if _, err := d.Engine().SQL("SELECT id FROM device WHERE uuid='dev-x'").Get(&devID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Engine().Insert(&models.PushLog{
		MessageId: msg.Id, DeviceId: devID, Status: models.PushRetry,
		Attempts: 2, Ctime: now, Utime: now,
	}); err != nil {
		t.Fatal(err)
	}

	jobs, err := NewPusher(d, nil, nil, PusherConfig{}).claim(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Fatalf("应当取出 1 条，得到 %d", len(jobs))
	}
	j := jobs[0]
	for _, tc := range []struct {
		field string
		got   any
		want  any
	}{
		{"attempts", j.attempt, 2},
		{"msg.uid", j.msg.UID, "m-x"},
		{"msg.type", j.msg.Type, models.TypeMarkdown},
		{"msg.title", j.msg.Title, "标题"},
		{"msg.summary", j.msg.Summary, "摘要"},
		{"msg.body", j.msg.Body, "正文正文"},
		{"msg.extra", j.msg.Extra, `{"file":"f1"}`},
		{"msg.rev", j.msg.Rev, int64(7)},
		{"msg.collapse_id", j.msg.CollapseId, "build-42"},
		{"msg.created_at", j.msg.Ctime, now - 30},
		{"dev.uuid", j.dev.UUID, "dev-x"},
		{"dev.apns_token", j.dev.APNsToken, "tok-x"},
		{"dev.apns_env", j.dev.APNsEnv, models.EnvSandbox},
		{"dev.updated_at", j.dev.Utime, now + 5},
		{"ch.id", j.ch.Id, ch},
		{"ch.level", j.ch.Level, models.LevelTimeSensitive},
		{"ch.sound", j.ch.Sound, "ding.caf"},
	} {
		if tc.got != tc.want {
			t.Errorf("%s = %v，期望 %v", tc.field, tc.got, tc.want)
		}
	}
}

// 退避要打散。APNs 返 503 或 429 时同一批请求是一起失败的，
// 固定梯度意味着它们又会在同一刻一起回来，把刚缓过来的服务再压一次。
func TestWithJitterSpreadsRetries(t *testing.T) {
	const base = 30 * time.Second
	lo, hi := time.Duration(float64(base)*0.8), time.Duration(float64(base)*1.2)

	seen := map[time.Duration]bool{}
	for i := 0; i < 200; i++ {
		got := withJitter(base)
		if got < lo || got > hi {
			t.Fatalf("抖动应当落在 ±20%% 之内：%s 不在 [%s, %s]", got, lo, hi)
		}
		seen[got] = true
	}
	// 200 次全都撞上同一个值，说明根本没抖
	if len(seen) < 50 {
		t.Errorf("200 次只产生了 %d 个不同的时长，没有真正打散", len(seen))
	}
}

// 抖动不能把退避打成 0 或负数——那等于不退避，失败时会打成忙循环。
func TestWithJitterStaysPositive(t *testing.T) {
	for _, base := range backoff {
		for i := 0; i < 100; i++ {
			if got := withJitter(base); got <= 0 {
				t.Fatalf("base=%s 抖出了非正值 %s", base, got)
			}
		}
	}
}

// 回复的东西一样都不许进 payload。
//
// 通知上不回复是产品决定（Amy 2026-09-16），客户端那条路径已经删了。
// 这里盯的是反向：服务端别把选项或时限又塞回去。真塞了也不会报错 ——
// 客户端只是忽略，多几十个字节谁都看不出来，所以只能靠这条测试拦。
//
// 用「短文本 + 两个选项」这个组合：它正是当初唯一会带上选项的那一种。
func TestPayloadNeverCarriesReply(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	res, err := NewSend(d).Deliver(ch, SendInput{
		Title: "窗帘 30 秒后自动打开",
		Body:  "客厅 · 工作日日程 07:30",
		Reply: &ReplySpec{
			Type: models.ReplyChoice, Options: []string{"打开", "不要打开"},
			Timeout: 30, Webhook: "https://x.test/h",
		},
	}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	var msg models.Message
	if _, err := d.Engine().Where("uid = ?", res.UID).Get(&msg); err != nil {
		t.Fatal(err)
	}
	b, err := kapns.Build((&Pusher{}).buildPayload(job{msg: msg, ch: models.Channel{Id: ch.Id}}))
	if err != nil {
		t.Fatal(err)
	}
	// 按 JSON 的键查而不是看结构体字段：字段删了，将来再加回来也必须在这里被发现。
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"a", "ru"} {
		if v, ok := got[k]; ok {
			t.Errorf("payload 不该有回复相关的键 %q，得到 %v", k, v)
		}
	}
	// ⚠️ 光查顶层键不够了：内联的 extra（"e"）是一个 JSON 字符串，
	// 回复的选项完全可以藏在它内部而顶层看不出来。所以连整条字节一起查——
	// 这也是 extraForPush 必须剥掉 reply 的原因。
	if bytes.Contains(b, []byte("不要打开")) {
		t.Errorf("回复选项出现在 payload 里：%s", b)
	}
	if e, ok := got["e"].(string); ok && strings.Contains(e, "reply") {
		t.Errorf("内联的 extra 里不该有 reply：%s", e)
	}
	// 另一半：消息本身照样可回，选项照样在 extra 里，app 内那条路不受影响。
	if !msg.Replyable() {
		t.Error("消息本身应当仍是可回复的")
	}
	if !strings.Contains(msg.Extra, "不要打开") {
		t.Error("选项应当留在 extra 里供 app 使用")
	}
	if msg.ReplyUntil == 0 {
		t.Error("时限应当落在 message.reply_until 上，客户端从 /sync 取")
	}
}

// 卡片的内容在 extra 里、body 是空的。不内联 extra 的话，卡片通知必然要让
// 通知扩展补一趟网络——而那是整条链路上最不可靠的一环（5 秒超时、不重试）。
func TestPayloadInlinesCardItems(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	res, err := NewSend(d).Deliver(ch, SendInput{
		Type:  models.TypeCard,
		Title: "9 月 15 日报表",
		Items: []CardItem{{K: "长截一下", V: "下载 4"}, {K: "下载来自", V: "美国 3"}},
	}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	var msg models.Message
	if _, err := d.Engine().Where("uid = ?", res.UID).Get(&msg); err != nil {
		t.Fatal(err)
	}
	b, err := kapns.Build((&Pusher{}).buildPayload(job{msg: msg, ch: models.Channel{Id: ch.Id}}))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	e, _ := got["e"].(string)
	if !strings.Contains(e, "下载 4") {
		t.Errorf("卡片内容应当内联进 payload，得到 e=%q", e)
	}
	// 带得下就不该标截断——客户端靠 tr 决定要不要联网补
	if _, ok := got["tr"]; ok {
		t.Errorf("这张卡片装得下，不该标截断：%s", b)
	}
}

// 只有回复、没有别的内容时，剥完就什么都不剩——别拿一个 `{}` 去占 payload 的字节。
func TestExtraForPushDropsEmptyResult(t *testing.T) {
	in := SendInput{Body: "x", Reply: &ReplySpec{
		Type: models.ReplyChoice, Options: []string{"好", "不好"}, Webhook: "https://x.test/h",
	}}
	raw, err := encodeExtra(in)
	if err != nil {
		t.Fatal(err)
	}
	if raw == "" {
		t.Fatal("这条 extra 本身不该是空的")
	}
	if got := extraForPush(raw); got != "" {
		t.Errorf("剥掉 reply 后没有内容了，应当返回空，得到 %q", got)
	}
}
