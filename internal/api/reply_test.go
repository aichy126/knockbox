package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/models"
)

// 平铺简写归一成对象。
//
// 完整形态是一个 reply 对象，但这台服务器的门面是一行 curl，而 query string
// 里表达不出嵌套对象。这条路断了的话，`curl ...?choices=a,b` 会静默退化成
// 一条普通的单向消息——发送方以为在等答案，而那条消息上根本没有按钮。
func TestSendAcceptsReplyShorthandFromQuery(t *testing.T) {
	s, r, ch := newSendServer(t, 0, 0)

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/send/"+ch.Token+"?title=窗帘&choices=打开,不要打开&reply_timeout=30"+
			"&reply_webhook=https://home.example.com/hook", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("发送失败：%d %s", w.Code, w.Body.String())
	}

	var got struct {
		Extra      string `xorm:"extra"`
		Webhook    string `xorm:"reply_webhook"`
		ReplyUntil int64  `xorm:"reply_until"`
	}
	has, err := s.DAO.Engine().SQL(
		"SELECT extra, reply_webhook, reply_until FROM message ORDER BY id DESC LIMIT 1").Get(&got)
	if err != nil || !has {
		t.Fatalf("消息没落库：%v", err)
	}
	if got.Webhook != "https://home.example.com/hook" {
		t.Errorf("回调地址没归一上：%q", got.Webhook)
	}
	if got.ReplyUntil == 0 {
		t.Error("reply_timeout 没归一上，这条消息变成了永不过期")
	}
	if !strings.Contains(got.Extra, "不要打开") || !strings.Contains(got.Extra, models.ReplyChoice) {
		t.Errorf("选项没进 extra：%s", got.Extra)
	}
}

// 回调地址【不能】进 extra —— extra 是原样下发给客户端的。
func TestSendKeepsWebhookOutOfExtra(t *testing.T) {
	s, r, ch := newSendServer(t, 0, 0)
	secret := "https://internal.example.com/secret-hook"
	body, _ := json.Marshal(map[string]any{
		"title": "x",
		"reply": map[string]any{
			"type": "choice", "options": []string{"是", "否"}, "webhook": secret,
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send/"+ch.Token, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("发送失败：%d %s", w.Code, w.Body.String())
	}
	var extra string
	if _, err := s.DAO.Engine().SQL(
		"SELECT extra FROM message ORDER BY id DESC LIMIT 1").Get(&extra); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(extra, secret) {
		t.Errorf("回调地址漏进了会下发给客户端的 extra：%s", extra)
	}
}

// 要收回复就必须给地址，而且要在发送那一刻就报错。
func TestSendRejectsReplyWithoutWebhook(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/send/"+ch.Token+"?title=x&choices=是,否", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	// 业务失败走 res.Rfail：HTTP 200 + code != 0，这是这套接口的统一口径
	if !strings.Contains(w.Body.String(), "webhook") {
		t.Errorf("缺回调地址应当明确报错，得到：%d %s", w.Code, w.Body.String())
	}
}

// MCP 那条路要能透传 reply。
// 参数名来自 service.SendInput 的 JSON tag，schema 手抄一份就会悄悄对不上——
// agent 按 schema 传的字段会被静默丢掉，而它以为自己在等一个答案。
func TestMCPKnockPassesReplyThrough(t *testing.T) {
	s, r, ch := newSendServer(t, 0, 0)
	text, isErr, _ := callKnock(t, r, ch.Token, map[string]any{
		"title": "要把 1.1.0 部署到生产吗？",
		"reply": map[string]any{
			"type":    "choice",
			"options": []string{"部署", "先别"},
			"timeout": 1800,
			"webhook": "https://ci.example.com/knockbox/reply",
		},
	})
	if isErr {
		t.Fatalf("发送应当成功：%s", text)
	}
	var res struct {
		UID        string `json:"uid"`
		ReplyUntil int64  `json:"reply_until"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		t.Fatalf("工具结果应当是 JSON：%s", text)
	}
	// agent 要靠 reply_until 知道等到什么时候，而不是自己拿本地时钟加——
	// 两边的钟未必一致，而判定在服务端。
	if res.ReplyUntil == 0 {
		t.Errorf("结果里应带回 reply_until：%s", text)
	}
	var got struct {
		Extra   string `xorm:"extra"`
		Webhook string `xorm:"reply_webhook"`
	}
	if _, err := s.DAO.Engine().SQL(
		"SELECT extra, reply_webhook FROM message WHERE uid = ?", res.UID).Get(&got); err != nil {
		t.Fatal(err)
	}
	if got.Webhook != "https://ci.example.com/knockbox/reply" {
		t.Errorf("MCP 的 reply.webhook 没落库：%q", got.Webhook)
	}
	if !strings.Contains(got.Extra, "先别") {
		t.Errorf("MCP 的 reply.options 没落库：%s", got.Extra)
	}
}

// 回复接口走 DeviceAuth。没有设备凭据一律进不来——
// 频道 token 更不行：它是只写的，连自己发出去那条消息被回了什么都读不到。
func TestReplyEndpointRequiresDeviceAuth(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)
	for _, tc := range []struct {
		name, auth string
	}{
		{"没有凭据", ""},
		{"频道 token", "Bearer " + ch.Token},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/x/reply",
				strings.NewReader(`{"reply":"打开"}`))
			req.Header.Set("Content-Type", "application/json")
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("应当 401，得到 %d %s", w.Code, w.Body.String())
			}
		})
	}
}
