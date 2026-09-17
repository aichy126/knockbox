package service

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
)

// replyable 发一条可回复的消息，返回它的 uid。
func replyable(t *testing.T, d *dao.DAO, ch *models.Channel, spec *ReplySpec) string {
	t.Helper()
	res, err := NewSend(d).Deliver(ch, SendInput{Title: "窗帘 30 秒后自动打开", Reply: spec}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	return res.UID
}

func choiceSpec() *ReplySpec {
	return &ReplySpec{
		Type: models.ReplyChoice, Options: []string{"打开", "不要打开"},
		Webhook: "https://home.example.com/hook",
	}
}

// 回复改的是一条【已有】的行，不 bump rev 的话别的设备永远同步不到「已回复」，
// 界面上那两颗按钮会一直亮着。
func TestReplyBumpsRev(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, choiceSpec())

	before := count(t, d, "SELECT rev AS n FROM message WHERE uid = '"+uid+"'")
	out, err := NewReply(d).Submit(ch.UserId, uid, "不要打开")
	if err != nil {
		t.Fatal(err)
	}
	if out.Rev <= int64(before) {
		t.Errorf("回复必须推高 rev：回复前 %d，回复后 %d", before, out.Rev)
	}
	after := count(t, d, "SELECT rev AS n FROM message WHERE uid = '"+uid+"'")
	if int64(after) != out.Rev {
		t.Errorf("库里的 rev 应与返回值一致：库 %d，返回 %d", after, out.Rev)
	}
}

// 回调必须和回复写在同一个事务里。分开的话，进程在两步之间挂掉，
// 用户看到自己回复成功、而发送方永远等不到——这是这个功能唯一不能出的错。
func TestReplyEnqueuesHookWithPayloadAndSecret(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, choiceSpec())

	if _, err := NewReply(d).Submit(ch.UserId, uid, "不要打开"); err != nil {
		t.Fatal(err)
	}
	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 {
		t.Fatalf("应排出一条回调，得到 %d 条", len(hooks))
	}
	h := hooks[0]
	if h.URL != "https://home.example.com/hook" {
		t.Errorf("回调地址不对: %s", h.URL)
	}
	// 密钥是频道 token 的快照：token 可以被轮换，而重试可能发生在轮换之后。
	if h.Secret != ch.Token {
		t.Errorf("回调应快照频道 token 当签名密钥")
	}
	var p hookPayload
	if err := json.Unmarshal([]byte(h.Payload), &p); err != nil {
		t.Fatal(err)
	}
	if p.Reply != "不要打开" || p.UID != uid || p.Channel != ch.Id {
		t.Errorf("回调内容不对: %+v", p)
	}
	// 带上标题：接收方多半是一段脚本，不该为了认出自己的问题再回查一次。
	if p.Title == "" {
		t.Error("回调应带上消息标题")
	}
}

// 一条消息只回一次，先到先得。两台设备同时点，第二个必须落空，
// 而且要拿到第一个的答案——用户需要知道结果，不是只被拒绝。
//
// 这个测试断言的是【行为】，不是某一行代码：Submit 里有两道防线
// （事务内的 Replied() 检查、UPDATE 的 replied_at = 0 条件），
// 实测任意留一道它都通过，两道都拆才失败（会成功两次、排出两条回调）。
// 所以它不会因为其中一道被改写而误报，但会在两道都失效时抓住。
func TestReplyIsOnceOnlyUnderConcurrency(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 2, false)
	uid := replyable(t, d, ch, choiceSpec())

	var wg sync.WaitGroup
	results := make([]error, 2)
	answers := []string{"打开", "不要打开"}
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, results[i] = NewReply(d).Submit(ch.UserId, uid, answers[i])
		}(i)
	}
	close(start)
	wg.Wait()

	ok, dup := 0, 0
	for _, err := range results {
		var already *ErrAlreadyReplied
		switch {
		case err == nil:
			ok++
		case errors.As(err, &already):
			dup++
			if already.Reply == "" {
				t.Error("重复回复时应带回已有的答案，否则界面无从显示「已回复 · X」")
			}
		default:
			t.Fatalf("意外错误: %v", err)
		}
	}
	if ok != 1 || dup != 1 {
		t.Errorf("两个并发回复应恰好一成一败，得到 成功 %d 失败 %d", ok, dup)
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM reply_hook"); n != 1 {
		t.Errorf("只该排出一条回调，得到 %d 条", n)
	}
}

// 过期判定必须在服务端。客户端也判一次是为了不让用户白点，但那只是显示；
// 判定放在客户端的话，改一下手机时间就能绕过去。
func TestReplyRejectedAfterDeadline(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	spec := choiceSpec()
	spec.Timeout = 30
	uid := replyable(t, d, ch, spec)

	// 把时限拨到过去，等价于「30 秒过完了」。
	if _, err := d.Engine().Exec(
		"UPDATE message SET reply_until = ? WHERE uid = ?", time.Now().Unix()-1, uid); err != nil {
		t.Fatal(err)
	}
	_, err := NewReply(d).Submit(ch.UserId, uid, "不要打开")
	if err == nil {
		t.Fatal("过了时限还能回复")
	}
	if !strings.Contains(err.Error(), "时限") {
		t.Errorf("错误里要说清是时限过了，用户才知道不是自己点错: %v", err)
	}
	// 关键：过期的回复【不能】排出回调。排了的话发送方会在自己早已走完默认分支之后，
	// 收到一个迟到的、相反的答案。
	if n := count(t, d, "SELECT COUNT(*) AS n FROM reply_hook"); n != 0 {
		t.Errorf("过期的回复不该产生回调，得到 %d 条", n)
	}
}

// 时限从落库起算，不从送达起算：APNs 是 best-effort，送达时刻没有上界。
func TestReplyDeadlineCountsFromSend(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	spec := choiceSpec()
	spec.Timeout = 30
	now := time.Now().Unix()
	res, err := NewSend(d).Deliver(ch, SendInput{Title: "x", Reply: spec}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if res.ReplyUntil < now+30 || res.ReplyUntil > now+31 {
		t.Errorf("时限应是落库时刻 + timeout，得到 %d（now=%d）", res.ReplyUntil, now)
	}
}

// 不设时限就一直可回。这是默认——只有「不回就会自动发生别的事」的消息才需要时限。
func TestReplyWithoutTimeoutNeverExpires(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	res, err := NewSend(d).Deliver(ch, SendInput{Title: "x", Reply: choiceSpec()}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if res.ReplyUntil != 0 {
		t.Errorf("没给 timeout 就不该有时限，得到 %d", res.ReplyUntil)
	}
	if _, err := NewReply(d).Submit(ch.UserId, res.UID, "打开"); err != nil {
		t.Fatalf("无时限的消息应当一直可回: %v", err)
	}
}

// choice 的答案必须是发送方给出的选项之一。不校验的话，任何持有设备 token 的人
// 都能把任意字符串送进发送方的回调，而那一端多半拿它去做分支判断。
func TestReplyChoiceMustBeOneOfTheOptions(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, choiceSpec())

	if _, err := NewReply(d).Submit(ch.UserId, uid, "顺便把灯也开了"); err == nil {
		t.Fatal("不在选项里的答案被接受了")
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM reply_hook"); n != 0 {
		t.Errorf("被拒的回复不该产生回调，得到 %d 条", n)
	}
}

// 单向消息不能被回复。没有回调地址的回复无处可去。
func TestReplyRejectedOnOneWayMessage(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	res, err := NewSend(d).Deliver(ch, SendInput{Title: "只是通知一下"}, "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReply(d).Submit(ch.UserId, res.UID, "好的"); err == nil {
		t.Fatal("单向消息被回复了")
	}
}

// 别人的消息回不了。归属校验不能只靠 uid 猜不到来划边界。
func TestReplyRejectedForOtherUser(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, choiceSpec())

	if _, err := NewReply(d).Submit(ch.UserId+999, uid, "打开"); err == nil {
		t.Fatal("别人的消息被回复了")
	}
}

// 文本回复允许任意内容，但不能是空的——空回复送到发送方那边无法判断。
func TestReplyTextRejectsEmpty(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, &ReplySpec{
		Type: models.ReplyText, Webhook: "https://x.test/hook"})

	if _, err := NewReply(d).Submit(ch.UserId, uid, "   "); err == nil {
		t.Fatal("空白文本回复被接受了")
	}
	if _, err := NewReply(d).Submit(ch.UserId, uid, "先别部署，等我看完日志"); err != nil {
		t.Fatalf("正常文本回复应当被接受: %v", err)
	}
}

// 回调地址【不能】下发给客户端：它是发送方的内部端点，
// 配对过的设备没有任何理由看到它。
func TestReplyWebhookNeverLeavesTheServer(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	secretURL := "https://internal.example.com/very-secret-hook"
	spec := choiceSpec()
	spec.Webhook = secretURL
	uid := replyable(t, d, ch, spec)

	out, err := NewSync(d).Since(ch.UserId, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range out.Messages {
		if m.UID != uid {
			continue
		}
		if strings.Contains(m.Extra, secretURL) {
			t.Error("回调地址泄漏进了 extra")
		}
		if !m.Replyable {
			t.Error("可回复的消息应当在同步流里标出 replyable")
		}
		blob, _ := json.Marshal(m)
		if strings.Contains(string(blob), secretURL) {
			t.Errorf("回调地址泄漏进了同步响应: %s", blob)
		}
		return
	}
	t.Fatal("同步流里没找到那条消息")
}

// 已回复的状态要能同步到别的设备——这正是回复必须 bump rev 的目的。
func TestRepliedStateSyncsToOtherDevices(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 2, false)
	uid := replyable(t, d, ch, choiceSpec())

	// 另一台设备已经同步到这个位置了
	cursor, err := d.CurrentRev()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewReply(d).Submit(ch.UserId, uid, "不要打开"); err != nil {
		t.Fatal(err)
	}
	out, err := NewSync(d).Since(ch.UserId, cursor, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range out.Messages {
		if m.UID == uid {
			if m.Reply != "不要打开" || m.RepliedAt == 0 {
				t.Errorf("增量同步应带上答案与时刻，得到 %+v", m)
			}
			return
		}
	}
	t.Fatal("回复没有出现在增量同步里，别的设备将永远看不到「已回复」")
}

// 多选：答案是一个数组，回调里也该是数组。
func TestMultiReply(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, &ReplySpec{
		Type: models.ReplyMulti, Options: []string{"api", "worker", "cron"},
		Webhook: "https://x.test/h"})

	if _, err := NewReply(d).Submit(ch.UserId, uid, `["worker","api"]`); err != nil {
		t.Fatal(err)
	}
	var msg models.Message
	if _, err := d.Engine().Where("uid = ?", uid).Get(&msg); err != nil {
		t.Fatal(err)
	}
	// 按发送方给的顺序重排，不按用户点的顺序：
	// 回调那一端拿到的顺序因此是稳定的，它不必自己再排一次。
	if msg.Reply != `["api","worker"]` {
		t.Errorf("应按选项原顺序归一，得到 %s", msg.Reply)
	}
	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Reply []string `json:"reply"`
	}
	if err := json.Unmarshal([]byte(hooks[0].Payload), &p); err != nil {
		t.Fatalf("回调里的 reply 应当是数组：%s", hooks[0].Payload)
	}
	if len(p.Reply) != 2 || p.Reply[0] != "api" {
		t.Errorf("回调内容不对：%v", p.Reply)
	}
}

// 多选允许一个都不选：「这几个都不要」是一个有意义的答案。
// 它和「还没回」不同 —— 后者看 replied_at，两者分得开。
func TestMultiReplyAcceptsEmptySelection(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, &ReplySpec{
		Type: models.ReplyMulti, Options: []string{"a", "b"}, Webhook: "https://x.test/h"})

	if _, err := NewReply(d).Submit(ch.UserId, uid, `[]`); err != nil {
		t.Fatalf("一个都不选应当被接受：%v", err)
	}
	var msg models.Message
	if _, err := d.Engine().Where("uid = ?", uid).Get(&msg); err != nil {
		t.Fatal(err)
	}
	if !msg.Replied() {
		t.Error("空选择也是回过了，replied_at 必须有值")
	}
}

// 多选里混进一个不在选项里的，整条拒绝。
func TestMultiReplyRejectsUnknownOption(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, &ReplySpec{
		Type: models.ReplyMulti, Options: []string{"a", "b"}, Webhook: "https://x.test/h"})

	if _, err := NewReply(d).Submit(ch.UserId, uid, `["a","顺便把别的也重启了"]`); err == nil {
		t.Fatal("混进未知选项却被接受了")
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM reply_hook"); n != 0 {
		t.Errorf("被拒的回复不该产生回调，得到 %d 条", n)
	}
}

// 数值要落到 step 的网格上，而不只是校验范围。
//
// 发送方声明 step=0.5 时，它期待拿到的是 24.0 或 24.5，不是 24.3178——
// 只校验范围的话，任何精度的值都能送进它的回调。
func TestNumberReplySnapsToStep(t *testing.T) {
	lo, hi, st := 16.0, 30.0, 0.5
	cases := []struct{ in, want string }{
		{"24", "24"},
		{"24.5", "24.5"},
		{"24.3178", "24.5"}, // 落到最近的网格点
		{"24.2", "24"},
		{"16", "16"},
		{"30", "30"},
		{"29.9", "30"}, // 靠近上界，四舍五入后不能越界
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			d := newDAO(t)
			ch := fixture(t, d, 1, false)
			uid := replyable(t, d, ch, &ReplySpec{
				Type: models.ReplyNumber, Min: &lo, Max: &hi, Step: &st,
				Unit: "°C", Webhook: "https://x.test/h"})
			if _, err := NewReply(d).Submit(ch.UserId, uid, c.in); err != nil {
				t.Fatal(err)
			}
			var msg models.Message
			if _, err := d.Engine().Where("uid = ?", uid).Get(&msg); err != nil {
				t.Fatal(err)
			}
			if msg.Reply != c.want {
				t.Errorf("%s 应归一成 %s，得到 %s", c.in, c.want, msg.Reply)
			}
		})
	}
}

// 超出范围的数值要拒绝，而且说清该在什么范围里。
func TestNumberReplyRejectsOutOfRange(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	lo, hi := 16.0, 30.0
	uid := replyable(t, d, ch, &ReplySpec{
		Type: models.ReplyNumber, Min: &lo, Max: &hi, Webhook: "https://x.test/h"})

	for _, bad := range []string{"15", "31", "abc", "NaN", ""} {
		if _, err := NewReply(d).Submit(ch.UserId, uid, bad); err == nil {
			t.Errorf("%q 应当被拒绝", bad)
		}
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM reply_hook"); n != 0 {
		t.Errorf("被拒的回复不该产生回调，得到 %d 条", n)
	}
}

// 数值在回调里是数字，不是字符串——接收方多半直接拿它做判断。
func TestNumberReplyIsANumberInTheCallback(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	lo, hi, st := 16.0, 30.0, 0.5
	uid := replyable(t, d, ch, &ReplySpec{
		Type: models.ReplyNumber, Min: &lo, Max: &hi, Step: &st,
		Unit: "°C", Webhook: "https://x.test/h"})

	if _, err := NewReply(d).Submit(ch.UserId, uid, "24.5"); err != nil {
		t.Fatal(err)
	}
	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Reply float64 `json:"reply"`
	}
	if err := json.Unmarshal([]byte(hooks[0].Payload), &p); err != nil {
		t.Fatalf("回调里的 reply 应当是数字：%s", hooks[0].Payload)
	}
	if p.Reply != 24.5 {
		t.Errorf("回调里的值不对：%v", p.Reply)
	}
}
