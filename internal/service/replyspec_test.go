package service

import (
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/models"
)

// 没有回调地址就没有回复的去处。
// 这里必须报错而不是默默收下：让用户在手机上认真点了一下、
// 而那个回答谁也收不到，是这个功能最坏的失败方式。
func TestReplySpecRequiresWebhook(t *testing.T) {
	spec := ReplySpec{Type: models.ReplyChoice, Options: []string{"是", "否"}}
	err := spec.validate()
	if err == nil {
		t.Fatal("没给 webhook 却通过了校验")
	}
	if !strings.Contains(err.Error(), "webhook") {
		t.Errorf("错误里要点名是缺 webhook: %v", err)
	}
}

// 发一条带回复的消息但不给地址，必须在【发送那一刻】就失败，
// 不能落库之后才发现。
func TestDeliverRejectsReplyWithoutWebhook(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	_, err := NewSend(d).Deliver(ch, SendInput{
		Title: "x",
		Reply: &ReplySpec{Type: models.ReplyChoice, Options: []string{"是", "否"}},
	}, "1.2.3.4")
	if err == nil {
		t.Fatal("缺 webhook 的消息被发出去了")
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM message"); n != 0 {
		t.Errorf("校验失败的消息不该落库，得到 %d 条", n)
	}
}

func TestReplySpecValidation(t *testing.T) {
	hook := "https://x.test/hook"
	cases := []struct {
		name string
		spec ReplySpec
		bad  string // 期望错误里出现的字眼；空串表示应当通过
	}{
		{"选项少于两个", ReplySpec{Type: models.ReplyChoice, Options: []string{"只有一个"}, Webhook: hook}, "at least two"},
		{"选项重复", ReplySpec{Type: models.ReplyChoice, Options: []string{"好", "好"}, Webhook: hook}, "twice"},
		{"空选项", ReplySpec{Type: models.ReplyChoice, Options: []string{"好", "  "}, Webhook: hook}, "empty entry"},
		{"选项过多", ReplySpec{Type: models.ReplyChoice, Options: make([]string, 20), Webhook: hook}, ""},
		{"text 带选项", ReplySpec{Type: models.ReplyText, Options: []string{"a", "b"}, Webhook: hook}, "does not take"},
		{"未知类型", ReplySpec{Type: "slider", Webhook: hook}, "unsupported"},
		{"负时限", ReplySpec{Type: models.ReplyText, Timeout: -1, Webhook: hook}, "cannot be negative"},
		{"时限过长", ReplySpec{Type: models.ReplyText, Timeout: 99999999, Webhook: hook}, "at most"},
		{"非 http 协议", ReplySpec{Type: models.ReplyText, Webhook: "ftp://x.test/h"}, "http"},
		{"缺主机名", ReplySpec{Type: models.ReplyText, Webhook: "https:///hook"}, "no host"},
		{"正常 choice", ReplySpec{Type: models.ReplyChoice, Options: []string{"打开", "不要打开"}, Webhook: hook}, ""},
		{"正常 text", ReplySpec{Type: models.ReplyText, Webhook: hook}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.spec.validate()
			if c.bad == "" {
				if c.name == "选项过多" {
					// 20 个空串会先撞上「空选项」，这里只要求它被拒
					if err == nil {
						t.Error("20 个选项应当被拒")
					}
					return
				}
				if err != nil {
					t.Errorf("应当通过，却报错: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("应当被拒，却通过了")
			}
			if !strings.Contains(err.Error(), c.bad) {
				t.Errorf("错误里应出现 %q，实际是: %v", c.bad, err)
			}
		})
	}
}

// 给了选项却忘了写 type 是最常见的手误，替他补上比报错有用。
func TestReplySpecInfersTypeFromOptions(t *testing.T) {
	spec := ReplySpec{Options: []string{"打开", "不要打开"}, Webhook: "https://x.test/h"}
	if err := spec.validate(); err != nil {
		t.Fatal(err)
	}
	if spec.Type != models.ReplyChoice {
		t.Errorf("有选项就该推断成 choice，得到 %q", spec.Type)
	}
	// 什么都没给时默认文本回复
	bare := ReplySpec{Webhook: "https://x.test/h"}
	if err := bare.validate(); err != nil {
		t.Fatal(err)
	}
	if bare.Type != models.ReplyText {
		t.Errorf("没有选项就该默认 text，得到 %q", bare.Type)
	}
}

// 选项前后的空白要去掉：发送方拼字符串时很容易带上，
// 而回复要和选项【逐字】相等才算数，留着空白会让用户点了却被拒。
func TestReplySpecTrimsOptions(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	spec := &ReplySpec{
		Type: models.ReplyChoice, Options: []string{" 打开 ", "不要打开"},
		Webhook: "https://x.test/h",
	}
	uid := replyable(t, d, ch, spec)
	if _, err := NewReply(d).Submit(ch.UserId, uid, "打开"); err != nil {
		t.Errorf("去掉空白后的选项应当能回: %v", err)
	}
}
