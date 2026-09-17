package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 端到端：发一条带选项的消息 → 用户回一个 → 发送方的回调端收到答案。
// 这是这个功能的完整故事，任何一环断了它都失败。
func TestEndToEndCurtainScenario(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)

	type received struct {
		Body []byte
		Sig  string
	}
	got := make(chan received, 1)
	home := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got <- received{Body: b, Sig: r.Header.Get(SignatureHeader)}
		w.WriteHeader(http.StatusOK)
	}))
	defer home.Close()

	// 1. 家庭自动化发一条：窗帘 30 秒后自动打开
	res, err := NewSend(d).Deliver(ch, SendInput{
		Title: "窗帘 30 秒后自动打开",
		Body:  "客厅 · 工作日日程 07:30",
		Reply: &ReplySpec{
			Type: "choice", Options: []string{"打开", "不要打开"},
			Timeout: 30, Webhook: home.URL,
		},
	}, "10.0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	if res.ReplyUntil == 0 {
		t.Fatal("发送方拿不到时限，它不知道该等到什么时候")
	}

	// 2. 手机上点「不要打开」
	out, err := NewReply(d).Submit(ch.UserId, res.UID, "不要打开")
	if err != nil {
		t.Fatal(err)
	}

	// 3. 回调 worker 把答案投给自动化
	w := NewWebhook(d, WebhookConfig{AllowPrivate: func() bool { return true }})
	if _, err := w.drain(context.Background()); err != nil {
		t.Fatal(err)
	}

	select {
	case r := <-got:
		var p hookPayload
		if err := json.Unmarshal(r.Body, &p); err != nil {
			t.Fatal(err)
		}
		if p.Reply != "不要打开" {
			t.Errorf("自动化收到的答案不对: %q", p.Reply)
		}
		if p.UID != res.UID || p.Title != "窗帘 30 秒后自动打开" {
			t.Errorf("回调内容对不上原消息: %+v", p)
		}
		if p.RepliedAt != out.RepliedAt {
			t.Errorf("回复时刻对不上: %d vs %d", p.RepliedAt, out.RepliedAt)
		}
		if r.Sig == "" {
			t.Error("回调没带签名，接收方无从验证来源")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("自动化没收到回调")
	}

	// 4. 手机上再点一次「打开」——必须落空，而且要拿到已有的答案
	_, err = NewReply(d).Submit(ch.UserId, res.UID, "打开")
	if err == nil {
		t.Fatal("同一条消息被回了两次")
	}
	// 5. 不能产生第二次回调：自动化已经按「不要打开」执行完了
	if n := count(t, d, "SELECT COUNT(*) AS n FROM reply_hook"); n != 1 {
		t.Errorf("只该有一条回调，得到 %d 条", n)
	}
}
