package apns

import (
	"encoding/json"
	"strings"
	"testing"
)

func mk(bodyLen int) Payload {
	var p Payload
	p.APS.Alert.Title = "构建失败"
	p.APS.Alert.Body = "main 分支 · 42s"
	p.MsgUID, p.ChannelID, p.Rev = "01J8QW7E3T9K2M", "01J8Q3MZ8A0DFC", 10481
	p.Body = strings.Repeat("日", bodyLen)
	return p
}

// 正文塞得下就内联——短文本消息因此完全不用联网。
func TestBuildInlinesSmallBody(t *testing.T) {
	b, err := Build(mk(20))
	if err != nil {
		t.Fatal(err)
	}
	var got Payload
	_ = json.Unmarshal(b, &got)
	if got.Body == "" || got.Truncated != 0 {
		t.Errorf("小正文应内联且不标截断，得到 body=%q tr=%d", got.Body, got.Truncated)
	}
}

// 超预算时内联正文第一个被丢，但推送仍然发得出去：正文在库里，
// 客户端能再取；推送发不出去则整条消息对用户不存在。
func TestBuildDropsBodyBeforeFailing(t *testing.T) {
	b, err := Build(mk(5000))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > payloadLimit {
		t.Fatalf("payload 应降级到 %d 字节内，得到 %d", payloadLimit, len(b))
	}
	var got Payload
	_ = json.Unmarshal(b, &got)
	if got.Body != "" || got.Truncated != 1 {
		t.Errorf("超预算时应丢掉内联正文并标 tr=1，得到 body 长 %d tr=%d", len(got.Body), got.Truncated)
	}
	if got.MsgUID == "" || got.Rev == 0 {
		t.Error("消息 id 和 rev 任何时候都不能丢——app 靠它们取全文和补洞")
	}
}

// 极端情况也必须发得出去。
func TestBuildAlwaysFits(t *testing.T) {
	p := mk(9000)
	p.APS.Alert.Title = strings.Repeat("标", 2000)
	p.APS.Alert.Body = strings.Repeat("摘", 2000)
	p.APS.Alert.Subtitle = strings.Repeat("副", 500)
	b, err := Build(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) > payloadLimit {
		t.Fatalf("最终降级后仍超限：%d 字节", len(b))
	}
}

// 最小骨架也要校验长度。
//
// 正常情况下它只剩几个有界的 id，一定装得下；但那是巧合不是保证。
// 装不下时报错，比发一个 Apple 必然拒收的 payload 好——报错至少会落在
// push_log 的 reason 里，而被拒收只会表现为「这条没收到」。
func TestBuildErrorsWhenEvenSkeletonCannotFit(t *testing.T) {
	var p Payload
	// 频道 id 本该是 26 字符的 ULID，这里给一个压不掉的超长值
	p.ChannelID = strings.Repeat("c", payloadLimit*2)
	p.MsgUID = "01HQ8ZK9ABCDEFGHJKMNPQRSTV"
	p.APS.Alert.Title = "标题"

	if _, err := Build(p); err == nil {
		t.Fatal("压不进上限时应当报错，而不是发出一个会被拒收的 payload")
	}
}

// 只要骨架装得下就必须发得出去，哪怕标题要被整个丢掉。
func TestBuildFallsBackToSkeleton(t *testing.T) {
	var p Payload
	p.MsgUID = "01HQ8ZK9ABCDEFGHJKMNPQRSTV"
	p.ChannelID = "01HQ8ZK9ABCDEFGHJKMNPQRSTW"
	p.APS.Alert.Title = strings.Repeat("题", 3000)
	p.APS.Alert.Subtitle = strings.Repeat("副", 3000)
	p.APS.Alert.Body = strings.Repeat("正", 3000)
	p.Body = strings.Repeat("文", 3000)
	p.Image = "https://example.com/" + strings.Repeat("x", 2000)

	b, err := Build(p)
	if err != nil {
		t.Fatalf("骨架装得下，不该报错：%v", err)
	}
	if len(b) > payloadLimit {
		t.Fatalf("降级之后仍然超限：%d 字节", len(b))
	}
	var out Payload
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.MsgUID != p.MsgUID || out.ChannelID != p.ChannelID {
		t.Error("降级可以丢内容，但消息 id 和频道 id 必须留下——app 靠它们取全文")
	}
	if out.Truncated == 0 {
		t.Error("内容被截断了，必须标记出来，否则客户端不知道要去取全文")
	}
}
