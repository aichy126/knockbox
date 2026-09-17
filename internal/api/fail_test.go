package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/uierr"
)

// 配对码错了，读到那句话的是站在手机前的人。所以它要按请求方的语言来，
// 同时带上 code——客户端将来可以拿 code 自己组织措辞。
func TestPairErrorSpeaksTheCallersLanguage(t *testing.T) {
	_, r, _ := newSendServer(t, 0, 0)

	call := func(acceptLang string) (msg, code string, status int) {
		t.Helper()
		body := `{"pair_code":"nosuchcode","uuid":"11111111-2222-3333-4444-555555555555","name":"t"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pair", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if acceptLang != "" {
			req.Header.Set("Accept-Language", acceptLang)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var out struct {
			Msg  string `json:"msg"`
			Data struct {
				ErrorCode string `json:"error_code"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
			t.Fatalf("响应不是 JSON：%s", w.Body.String())
		}
		return out.Msg, out.Data.ErrorCode, w.Code
	}

	// 没带 Accept-Language 的一律英文：服务端不知道对面是谁，
	// 而英文是这个项目的基准语言。
	msg, code, _ := call("")
	if code != uierr.PairNotFound {
		t.Errorf("应当带上 error_code %q，得到 %q", uierr.PairNotFound, code)
	}
	if !strings.Contains(msg, "pairing code") {
		t.Errorf("默认应当是英文：%q", msg)
	}

	zhMsg, zhCode, _ := call("zh-CN,zh;q=0.9,en;q=0.8")
	if zhCode != code {
		t.Errorf("换一种语言不该换 code：%q vs %q", zhCode, code)
	}
	if !strings.Contains(zhMsg, "配对码") {
		t.Errorf("中文客户端应当拿到中文：%q", zhMsg)
	}
	if zhMsg == msg {
		t.Error("两种语言返回了同一句话，渲染没生效")
	}
}

// 没归过类的错误保持原样透出：那批是发送 API 的契约校验，
// 读它的是写脚本的人，不该被包装成一个 code。
func TestUnclassifiedErrorStillPassesThrough(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)

	body := `{"type":"text","title":"t","body":"b","reply":{"type":"choice","options":["only one"],"webhook":"https://x.test/h"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send/"+ch.Token, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "at least two") {
		t.Errorf("发送侧的契约校验应当原样返回英文：%s", w.Body.String())
	}
}
