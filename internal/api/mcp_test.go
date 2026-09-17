package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// callKnock 按 MCP 的 JSON-RPC 形态调一次 knock，返回工具结果里那段文本与 isError。
func callKnock(t *testing.T, r *gin.Engine, token string, args map[string]any) (string, bool, int) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "knock", "arguments": args},
	})
	if err != nil {
		t.Fatal(err)
	}
	path := "/mcp"
	if token != "" {
		path += "/" + token
	}
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		return w.Body.String(), true, w.Code
	}
	var out struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应不是 JSON-RPC：%s", w.Body.String())
	}
	if out.Error != nil {
		t.Fatalf("协议层报错：%s", out.Error.Message)
	}
	if len(out.Result.Content) == 0 {
		t.Fatalf("工具结果没有内容：%s", w.Body.String())
	}
	return out.Result.Content[0].Text, out.Result.IsError, w.Code
}

// MCP 的参数名靠 service.SendInput 的 JSON tag 映射。哪天 tag 改了名，
// curl 那条路会连带改掉，而 MCP 这条路只是悄悄收到一条字段全空的消息。
func TestMCPKnockMapsEveryField(t *testing.T) {
	s, r, ch := newSendServer(t, 0, 0)

	text, isErr, _ := callKnock(t, r, ch.Token, map[string]any{
		"type":        "card",
		"title":       "Deploy finished",
		"body":        "all green",
		"link":        "https://example.com/build/7",
		"collapse_id": "deploy-7",
		"items": []map[string]any{
			{"k": "Branch", "v": "main", "style": "ok"},
		},
	})
	if isErr {
		t.Fatalf("发送应当成功：%s", text)
	}
	var res struct {
		UID string `json:"uid"`
		Rev int64  `json:"rev"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		t.Fatalf("工具结果应当是一段 JSON：%s", text)
	}
	if res.UID == "" || res.Rev == 0 {
		t.Errorf("结果要带 uid 与 rev，拿到 %s", text)
	}

	// link 与 items 落在 extra 那一列的 JSON 里
	var got struct {
		Type       string `xorm:"type"`
		Title      string `xorm:"title"`
		Body       string `xorm:"body"`
		CollapseId string `xorm:"collapse_id"`
		Extra      string `xorm:"extra"`
	}
	has, err := s.DAO.Engine().SQL(
		"SELECT type, title, body, collapse_id, extra FROM message WHERE uid = ?", res.UID).Get(&got)
	if err != nil || !has {
		t.Fatalf("消息没落库：%v", err)
	}
	if got.Type != "card" || got.Title != "Deploy finished" || got.Body != "all green" {
		t.Errorf("type/title/body 没映射上：%+v", got)
	}
	if got.CollapseId != "deploy-7" {
		t.Errorf("collapse_id 没映射上：%+v", got)
	}
	if !strings.Contains(got.Extra, "https://example.com/build/7") {
		t.Errorf("link 没映射上：%s", got.Extra)
	}
	if !strings.Contains(got.Extra, "Branch") || !strings.Contains(got.Extra, "main") {
		t.Errorf("card 的 items 没映射上：%s", got.Extra)
	}
}

// agent 能用的回复形态要和 curl 那条路一样是四种。
//
// 这条测试盯的是「schema 里写了、服务端也真收」：两者分开维护，
// enum 少写一个的表现是 agent 压根不知道有这种形态，而不是报错。
func TestMCPKnockAcceptsEveryReplyType(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)

	cases := []struct {
		name  string
		reply map[string]any
	}{
		{"choice", map[string]any{"type": "choice", "options": []string{"部署", "先别"},
			"webhook": "https://x.test/h"}},
		{"multi", map[string]any{"type": "multi", "options": []string{"api", "worker"},
			"webhook": "https://x.test/h"}},
		{"number", map[string]any{"type": "number", "min": 16, "max": 30, "step": 0.5,
			"unit": "°C", "webhook": "https://x.test/h"}},
		{"text", map[string]any{"type": "text", "webhook": "https://x.test/h"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, isErr, _ := callKnock(t, r, ch.Token, map[string]any{
				"title": "要不要继续", "reply": c.reply,
			})
			if isErr {
				t.Fatalf("type=%s 应当收下：%s", c.name, text)
			}
		})
	}

	// 上面只证明服务端收得下——它一直都收得下。真正会掉的是 schema：
	// enum 里少一个，agent 就永远不会发那种形态，而且不会有任何报错。
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/list",
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/mcp/"+ch.Token, strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var list struct {
		Result struct {
			Tools []struct {
				InputSchema struct {
					Properties struct {
						Reply struct {
							Properties struct {
								Type struct {
									Enum []string `json:"enum"`
								} `json:"type"`
							} `json:"properties"`
						} `json:"reply"`
					} `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("tools/list 不是 JSON-RPC：%s", w.Body.String())
	}
	if len(list.Result.Tools) != 1 {
		t.Fatalf("应当只有一件工具，拿到 %d 件", len(list.Result.Tools))
	}
	got := list.Result.Tools[0].InputSchema.Properties.Reply.Properties.Type.Enum
	if len(got) != len(cases) {
		t.Errorf("schema 的 reply.type 应当列出 %d 种形态，拿到 %v", len(cases), got)
	}
	for _, c := range cases {
		if !slices.Contains(got, c.name) {
			t.Errorf("schema 的 reply.type 少了 %q：%v", c.name, got)
		}
	}
}

// MCP 走的是同一条发送路径，速率限制必须照样生效——
// 不然它就是一个绕过限流的后门，而限流是自建模式下唯一拦得住泄露 token 的东西。
func TestMCPKnockSharesSendRateLimit(t *testing.T) {
	_, r, ch := newSendServer(t, 1, 2) // 长期 1/s，可攒 2 个

	for i := 0; i < 2; i++ {
		if text, isErr, _ := callKnock(t, r, ch.Token, map[string]any{"body": "hi"}); isErr {
			t.Fatalf("突发额度内的第 %d 条被挡了：%s", i+1, text)
		}
	}
	text, isErr, _ := callKnock(t, r, ch.Token, map[string]any{"body": "hi"})
	if !isErr {
		t.Fatalf("超过速率应当报错，却成功了：%s", text)
	}
	if !strings.Contains(text, "频繁") {
		t.Errorf("提示要说清是频率问题：%s", text)
	}
}

// 标题和正文都空的一条在手机上是一条看不出是什么的通知，挡在这里比发出去好。
func TestMCPKnockRejectsEmptyMessage(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)
	text, isErr, _ := callKnock(t, r, ch.Token, map[string]any{})
	if !isErr {
		t.Fatalf("空消息应当报错，却成功了：%s", text)
	}
}

// file 是别处上传的附件引用，MCP 这条路传不了附件。留着它等于开一个猜 uid 的入口，
// 而且猜中的 uid 可能属于别人。
func TestMCPKnockIgnoresFileReference(t *testing.T) {
	s, r, ch := newSendServer(t, 0, 0)
	f, err := s.Files.Store(ch.UserId, "x.png", testPNG(8, 8))
	if err != nil {
		t.Fatal(err)
	}
	text, isErr, _ := callKnock(t, r, ch.Token, map[string]any{"body": "hi", "file": f.UID})
	if isErr {
		t.Fatalf("发送应当成功：%s", text)
	}
	var n int64
	if _, err := s.DAO.Engine().SQL("SELECT COUNT(*) FROM message WHERE file_id != 0").Get(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("MCP 发出的消息不该带上附件，却有 %d 条带了", n)
	}
}

// 没有凭据的请求要在鉴权就被挡住，不能进到工具里。
func TestMCPRequiresChannelToken(t *testing.T) {
	_, r, _ := newSendServer(t, 0, 0)
	_, isErr, code := callKnock(t, r, "", nil)
	if !isErr || code != http.StatusUnauthorized {
		t.Errorf("没带 token 应当 401，得到 %d", code)
	}
}
