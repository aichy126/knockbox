package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/uierr"
)

func no() *bool { b := false; return &b }

func f64(v float64) *float64 { return &v }

// formSpec 「点一个 + 选填一句」，组合回复最常见的那种问法。
func formSpec() *ReplySpec {
	return &ReplySpec{
		Fields: []ReplyField{
			{ID: "action", Label: "要部署吗？", Type: models.ReplyChoice, Options: []string{"部署", "暂缓", "回滚"}},
			{ID: "note", Label: "备注", Type: models.ReplyText, Placeholder: "要补充什么吗？", Required: no()},
		},
		Webhook: "https://home.example.com/hook",
	}
}

// onlyHook 取那一条回调的内容，按原始 JSON 解开：要看的是「键在不在」，结构体看不出来。
func onlyHook(t *testing.T, d *dao.DAO) map[string]any {
	t.Helper()
	var hooks []models.ReplyHook
	if err := d.Engine().Find(&hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks) != 1 {
		t.Fatalf("应排出一条回调，得到 %d 条", len(hooks))
	}
	var p map[string]any
	if err := json.Unmarshal([]byte(hooks[0].Payload), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func raw(m map[string]any) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		b, _ := json.Marshal(v)
		out[k] = b
	}
	return out
}

func TestReplyFieldsSpecValidation(t *testing.T) {
	hook := "https://x.test/h"
	choice := ReplyField{ID: "a", Type: models.ReplyChoice, Options: []string{"是", "否"}}
	many := make([]ReplyField, 11)
	for i := range many {
		many[i] = ReplyField{ID: "f" + strings.Repeat("x", i), Type: models.ReplyText}
	}
	cases := []struct {
		name string
		spec ReplySpec
		want string
	}{
		{"超过 10 个", ReplySpec{Fields: many, Webhook: hook}, "at most 10"},
		{"id 带大写", ReplySpec{Fields: []ReplyField{{ID: "Note", Type: models.ReplyText}}, Webhook: hook}, ".id must be"},
		{"id 重复", ReplySpec{Fields: []ReplyField{choice, choice}, Webhook: hook}, "twice"},
		{"控件没写 type", ReplySpec{Fields: []ReplyField{{ID: "a", Options: []string{"是", "否"}}}, Webhook: hook}, "type is required"},
		{"顶层 type 与 fields 同用", ReplySpec{Type: models.ReplyChoice, Fields: []ReplyField{choice}, Webhook: hook}, "cannot be used together"},
		{"顶层 options 与 fields 同用", ReplySpec{Options: []string{"a", "b"}, Fields: []ReplyField{choice}, Webhook: hook}, "go on each field"},
		{"控件里的错误指明是第几个", ReplySpec{Fields: []ReplyField{choice, {ID: "b", Type: models.ReplyChoice, Options: []string{"只有一个"}}}, Webhook: hook}, "reply.fields[1].options"},
		{"placeholder 只给 text", ReplySpec{Fields: []ReplyField{{ID: "a", Type: models.ReplyChoice, Options: []string{"是", "否"}, Placeholder: "x"}}, Webhook: hook}, "only for type=text"},
		{"label 过长", ReplySpec{Fields: []ReplyField{{ID: "a", Label: strings.Repeat("问", 61), Type: models.ReplyText}}, Webhook: hook}, ".label is at most"},
		{"正常的一组", *formSpec(), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			spec := c.spec
			err := spec.validate()
			if c.want == "" {
				if err != nil {
					t.Fatalf("不该报错: %v", err)
				}
				if spec.Type != models.ReplyForm {
					t.Errorf("组合回复的 type 应记成 form，得到 %q", spec.Type)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Errorf("要报含 %q 的错，得到 %v", c.want, err)
			}
		})
	}
}

// 「点一个 + 选填一句」：选填的没填，回调里【没有这个键】，而不是 ""。
// 组合回复不给 reply —— 几个答案里没有「那一个」。
func TestFormReplyOmitsUnansweredOptionalField(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, formSpec())

	if _, err := NewReply(d).Submit(ch.UserId, uid, "", raw(map[string]any{"action": "暂缓"})); err != nil {
		t.Fatal(err)
	}
	p := onlyHook(t, d)
	if _, ok := p["reply"]; ok {
		t.Errorf("组合回复不该带 reply：%v", p)
	}
	ans, _ := p["answers"].(map[string]any)
	if ans["action"] != "暂缓" {
		t.Errorf("answers.action 不对：%v", ans)
	}
	if _, ok := ans["note"]; ok {
		t.Errorf("选填没填应省掉这个键，得到 %v", ans)
	}
}

// 每个控件的答案类型与单控件写法一致：多选是数组，数字是数字且落在 step 网格上。
func TestFormReplyKeepsTypesPerField(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, &ReplySpec{
		Fields: []ReplyField{
			{ID: "envs", Type: models.ReplyMulti, Options: []string{"api", "worker", "web"}},
			{ID: "pct", Type: models.ReplyNumber, Min: f64(0), Max: f64(100), Step: f64(5)},
			{ID: "note", Type: models.ReplyText},
		},
		Webhook: "https://home.example.com/hook",
	})
	_, err := NewReply(d).Submit(ch.UserId, uid, "", raw(map[string]any{
		"envs": []string{"web", "api"}, "pct": 42, "note": "  先灰度  ",
	}))
	if err != nil {
		t.Fatal(err)
	}
	ans, _ := onlyHook(t, d)["answers"].(map[string]any)
	envs, _ := ans["envs"].([]any)
	if len(envs) != 2 || envs[0] != "api" || envs[1] != "web" {
		t.Errorf("多选应是按发送方顺序排好的数组，得到 %v", ans["envs"])
	}
	if ans["pct"] != float64(40) {
		t.Errorf("数字应是落到 step 网格上的数字，得到 %v (%T)", ans["pct"], ans["pct"])
	}
	if ans["note"] != "先灰度" {
		t.Errorf("文字应去掉首尾空白，得到 %q", ans["note"])
	}
}

// 必填的没填、送来规格里没有的 id：都要拒，而且不留下任何回调。
func TestFormReplyRejectsMissingRequiredAndUnknownField(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, formSpec())

	_, err := NewReply(d).Submit(ch.UserId, uid, "", raw(map[string]any{"note": "只写了备注"}))
	var ue *uierr.Error
	if !errors.As(err, &ue) || ue.Code != uierr.ReplyMissing {
		t.Errorf("必填没填应报 %s，得到 %v", uierr.ReplyMissing, err)
	}
	_, err = NewReply(d).Submit(ch.UserId, uid, "", raw(map[string]any{"action": "暂缓", "extra": "x"}))
	if err == nil {
		t.Error("规格里没有的 id 应拒收")
	}
	if n := count(t, d, "SELECT COUNT(*) AS n FROM reply_hook"); n != 0 {
		t.Errorf("被拒的回复不该排出回调，得到 %d 条", n)
	}
}

// 单控件写法：回调里 reply 照旧，另外多一个 answers（键固定是 "reply"），
// 已经接好的人一行都不用改，新接的人可以只认 answers。
func TestSingleReplyAlsoCarriesAnswers(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, choiceSpec())
	if _, err := NewReply(d).Submit(ch.UserId, uid, "不要打开", nil); err != nil {
		t.Fatal(err)
	}
	p := onlyHook(t, d)
	if p["reply"] != "不要打开" {
		t.Errorf("reply 应保持原样，得到 %v", p["reply"])
	}
	ans, _ := p["answers"].(map[string]any)
	if ans["reply"] != "不要打开" {
		t.Errorf("answers.reply 应与 reply 相同，得到 %v", ans)
	}
}

// 文字回复写了 required: false 就能空着交（「没什么要补充的」）。
// 空着交的 reply 是 ""，而且这个键不能被省掉——省掉的话接收方分不清「答了空」和「格式变了」。
func TestOptionalTextReplyCanBeEmpty(t *testing.T) {
	d := newDAO(t)
	ch := fixture(t, d, 1, false)
	uid := replyable(t, d, ch, &ReplySpec{Type: models.ReplyText, Required: no(), Webhook: "https://home.example.com/hook"})
	if _, err := NewReply(d).Submit(ch.UserId, uid, "   ", nil); err != nil {
		t.Fatalf("选填的文字回复应能空着交：%v", err)
	}
	p := onlyHook(t, d)
	if v, ok := p["reply"]; !ok || v != "" {
		t.Errorf("空着交的 reply 应是 \"\" 且不能省掉，得到 %v (在不在: %v)", v, ok)
	}
}
