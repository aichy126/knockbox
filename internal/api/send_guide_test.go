package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/api/web"
)

// 给 agent 的提示词里，每一段 JSON 示例都必须是这台服务器收的。
//
// agent 会照着示例原样发。发送格式一变、示例没跟上，它发出去就是 400，
// 而那段提示词是复制走的，页面上看不出任何不对。
func TestAgentPromptExamplesAreAccepted(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)
	body := regexp.MustCompile(`(?s)-d '(\{.*?\})' \{\{URL\}\}`)
	hook := strings.NewReplacer("https://你的地址/hook", "https://hook.example.com/hook",
		"https://your-host/hook", "https://hook.example.com/hook")

	for _, lang := range []web.Lang{web.LangZH, web.LangEN} {
		prompt := agentPrompt("{{URL}}", lang)
		found := body.FindAllStringSubmatch(prompt, -1)
		if len(found) < 3 {
			t.Fatalf("%s：只找到 %d 段 JSON 示例，正则或提示词变了", lang, len(found))
		}
		for _, m := range found {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/send/"+ch.Token,
				strings.NewReader(hook.Replace(m[1])))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			// 业务失败也是 HTTP 200，看的是 code
			var res struct {
				Code int `json:"code"`
			}
			if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &res) != nil || res.Code != 0 {
				t.Errorf("%s 的示例被拒：%d %s\n%s", lang, w.Code, w.Body.String(), m[1])
			}
		}
	}
}
