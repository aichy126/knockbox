package service

import "encoding/json"

// extra 把 card / link / actions 这些结构化的东西收进一个 JSON 列。
// 服务端不解释它们的语义，只负责原样存取——渲染是 app 的事。
type extra struct {
	Link    string     `json:"link,omitempty"`
	Copy    string     `json:"copy,omitempty"`
	Items   []CardItem `json:"items,omitempty"`
	Actions []Action   `json:"actions,omitempty"`
	File    string     `json:"file,omitempty"`
	// Reply 客户端画回复界面要的东西：回复的形态和选项。
	// 【不含回调地址和时限】——地址是发送方的内部端点，时限在 message.reply_until
	// 单独一列（客户端要拿它算倒计时，而 extra 是个不透明 blob，不适合放要比较的值）。
	Reply *replyView `json:"reply,omitempty"`
}

// replyView extra 里的回复规格，只有客户端画界面用得上的部分。
type replyView struct {
	Type    string   `json:"type"`
	Options []string `json:"options,omitempty"`
	// 下面四个只对 type=number 有意义，客户端画滑块要用。
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Step *float64 `json:"step,omitempty"`
	Unit string   `json:"unit,omitempty"`
}

func encodeExtra(in SendInput) (string, error) {
	e := extra{Link: in.Link, Copy: in.Copy, Items: in.Items, Actions: in.Actions, File: in.File}
	if in.Reply != nil {
		e.Reply = &replyView{
			Type: in.Reply.Type, Options: in.Reply.Options,
			Min: in.Reply.Min, Max: in.Reply.Max, Step: in.Reply.Step, Unit: in.Reply.Unit,
		}
	}
	if e.Link == "" && e.Copy == "" && len(e.Items) == 0 && len(e.Actions) == 0 &&
		e.File == "" && e.Reply == nil {
		return "", nil
	}
	b, err := json.Marshal(e)
	return string(b), err
}

// extraForPush 把 extra 裁成能进 APNs payload 的样子。
//
// 【为什么要内联它】卡片的内容——items、链接、复制按钮——全在 extra 里，
// 而卡片的 body 通常是空的。只内联 body 的话，卡片通知必然要让通知扩展
// 补一趟网络，而那趟网络是整条链路上最不可靠的一环：5 秒超时、不重试、
// 没网就没有。带上它，小卡片就和短文本一样完全不用联网。
//
// ⚠️ 【必须剥掉 reply】不变量是「回复的任何东西都不进 payload」。
// extra 是原样同步给客户端的完整结构，直接内联等于把选项和形态下发了——
// 而且是藏在一个 JSON 字符串里下发的，按顶层键查的测试根本发现不了。
//
// 剥完什么都不剩就返回空：payload 每个字节都要省，`{}` 不值得占位置。
func extraForPush(raw string) string {
	e, err := decodeExtra(raw)
	if err != nil {
		return ""
	}
	e.Reply = nil
	if e.Link == "" && e.Copy == "" && len(e.Items) == 0 && len(e.Actions) == 0 && e.File == "" {
		return ""
	}
	b, err := json.Marshal(e)
	if err != nil {
		return ""
	}
	return string(b)
}

// decodeExtra 读回附加信息。服务端只在需要时解释它（比如推送时要取附件 uid），
// 平时原样存取。
func decodeExtra(raw string) (extra, error) {
	var e extra
	if raw == "" {
		return e, nil
	}
	err := json.Unmarshal([]byte(raw), &e)
	return e, err
}
