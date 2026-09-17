package apns

import (
	"encoding/json"
	"fmt"
	"strings"
)

// payloadLimit APNs 的硬上限是 4096 字节，留一点余量。
// 超了是 413 PayloadTooLarge，整条推送失败。
const payloadLimit = 3800

// Alert APNs 的 alert 段。
type Alert struct {
	Title    string `json:"title,omitempty"`
	Subtitle string `json:"subtitle,omitempty"`
	Body     string `json:"body,omitempty"`
}

// Payload 一条推送的完整载荷。
//
// 自定义字段刻意用短名：payload 有 4KB 上限，键名也占字节。
//
//	m  消息 uid —— app 拿它去取全文
//	c  频道 id  —— app 靠它认频道；【不是】发送 token，
//	              因为 payload 会经过 Apple 并留在通知的 userInfo 里
//	r  rev     —— 让 app 知道该同步到哪，也用来发现漏收
//	t  类型
//	b  内联正文 —— 预算够时才带。带上的话短文本消息完全不用联网，
//	              延迟为零、离线可用；预算不够时它第一个被丢
//	e  内联附加信息 —— 卡片的 items、链接、复制按钮都在这里，而卡片的 b 通常是空的。
//	              【已剥掉 reply】见下。与 b 同进同退：预算不够时一起丢
//	tr 内容被截断，展开需要拉取。【客户端据此决定要不要联网补】
//	              不能拿「b 是不是空」当判据——卡片的 b 本来就是空的
//
// 【回复的任何东西都不进 payload】：选项、时限、形态都不下发。
// 通知只负责把人叫进来，回复一律在 app 内完成——这是产品决定，不是压不下。
// 客户端从 /sync 拿这些（extra.reply + reply_until），那里没有 4KB 的限制。
type Payload struct {
	APS struct {
		Alert             Alert   `json:"alert"`
		Sound             string  `json:"sound,omitempty"`
		Badge             *int    `json:"badge,omitempty"`
		ThreadID          string  `json:"thread-id,omitempty"`
		MutableContent    int     `json:"mutable-content,omitempty"`
		InterruptionLevel string  `json:"interruption-level,omitempty"`
		RelevanceScore    float64 `json:"relevance-score,omitempty"`
		Category          string  `json:"category,omitempty"`
	} `json:"aps"`
	MsgUID    string `json:"m"`
	ChannelID string `json:"c"`
	Rev       int64  `json:"r"`
	Type      string `json:"t,omitempty"`
	Body      string `json:"b,omitempty"`
	Extra     string `json:"e,omitempty"`
	Truncated int    `json:"tr,omitempty"`
	// i 缩略图签名链接 —— 通知服务扩展下这张图做成 UNNotificationAttachment，
	// 通知上那块 app 图标就换成图片本身。给的是【缩略图】不是原图：
	// 扩展只有 24MB 内存预算，让它下原图是自找 OOM。
	Image string `json:"i,omitempty"`
}

// Build 组装并按需降级，保证一定能发出去。
//
// 降级顺序：丢内联正文 → 截标题 → 截摘要 → 再截 → 丢副标题 → 丢附件链接 →
// 只留最小骨架。关键在于这个降级【永远不会让推送发不出去】：正文本来就在库里，
// payload 只是一张通知条，丢掉的部分客户端都能再取回来。
// 反过来，超长就整条失败的话，用户会连「有一条新消息」都不知道。
func Build(p Payload) ([]byte, error) {
	if b, ok := tryMarshal(p); ok {
		return b, nil
	}
	// ① 内联内容最先丢。b 与 e 一起丢：卡片只剩一半比全靠拉更难处理，
	// 而 tr=1 的语义是「有东西没带上，要自己去拉」，拉回来的是完整的一条。
	p.Body, p.Extra, p.Truncated = "", "", 1
	if b, ok := tryMarshal(p); ok {
		return b, nil
	}
	p.APS.Alert.Title = truncate(p.APS.Alert.Title, 100) // ② 截标题
	if b, ok := tryMarshal(p); ok {
		return b, nil
	}
	p.APS.Alert.Body = truncate(p.APS.Alert.Body, 300) // ③ 截摘要
	if b, ok := tryMarshal(p); ok {
		return b, nil
	}
	p.APS.Alert.Body = truncate(p.APS.Alert.Body, 120) // ④ 再截
	if b, ok := tryMarshal(p); ok {
		return b, nil
	}
	p.APS.Alert.Subtitle = "" // ⑤ 丢副标题
	if b, ok := tryMarshal(p); ok {
		return b, nil
	}
	p.Image = "" // ⑥ 丢附件链接——图没了还能看正文，通知发不出去就什么都没有
	if b, ok := tryMarshal(p); ok {
		return b, nil
	}
	// ⑦ 最小骨架。剩下的都是有界的 id，正常情况下一定装得下——但不靠这个假设：
	// 仍然超就连标题一起丢，再超就报错。发一个 Apple 必然拒收的 payload
	// 不比报错好，报错至少会落在 push_log 的 reason 里。
	min := Payload{MsgUID: p.MsgUID, ChannelID: p.ChannelID, Rev: p.Rev, Truncated: 1}
	min.APS.Alert.Title = truncate(p.APS.Alert.Title, 60)
	min.APS.MutableContent = p.APS.MutableContent
	min.APS.ThreadID = p.APS.ThreadID
	if b, ok := tryMarshal(min); ok {
		return b, nil
	}
	min.APS.Alert.Title = ""
	if b, ok := tryMarshal(min); ok {
		return b, nil
	}
	return nil, fmt.Errorf("payload 压不到 %d 字节以内", payloadLimit)
}

func tryMarshal(p Payload) ([]byte, bool) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, false
	}
	return b, len(b) <= payloadLimit
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimRightFunc(string(r[:n-1]), func(r rune) bool { return r == ' ' }) + "…"
}
