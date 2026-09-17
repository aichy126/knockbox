package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// newSendServer 起一个带真实发送路由的服务端，并建好一个频道。
func newSendServer(t *testing.T, qps, burst int) (*Server, *gin.Engine, *models.Channel) {
	t.Helper()
	s, r := newServerWith(t, false, 100, func(s *Server) {
		s.SendQPS, s.SendBurst = qps, burst
	})
	u, err := service.NewAccount(s.DAO).Create("owner", "hunter2hunter2", models.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	ch, err := service.NewChannel(s.DAO).Create(u.Id, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	return s, r, ch
}

func postSend(r *gin.Engine, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send/"+token, strings.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 发送接口原来没有任何限流，而自建模式下配额也整个关闭：
// 一个泄露的频道 token 可以无限写库、无限占盘，没有任何东西拦得住。
func TestSendIsRateLimitedPerChannel(t *testing.T) {
	s, r, ch := newSendServer(t, 1, 3) // 长期 1/s，可攒 3 个

	for i := 0; i < 3; i++ {
		if w := postSend(r, ch.Token, "hi"); w.Code != http.StatusOK {
			t.Fatalf("突发额度内的第 %d 条被挡了：%d %s", i+1, w.Code, w.Body.String())
		}
	}
	w := postSend(r, ch.Token, "hi")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("超过速率应当返回 429，得到 %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "频繁") {
		t.Errorf("提示要说清是频率问题：%s", w.Body.String())
	}

	// 被挡下的那条绝不能落库——通知系统最不能干的事就是「说失败了其实存了」
	var n int64
	_, _ = s.DAO.Engine().SQL("SELECT COUNT(*) FROM message").Get(&n)
	if n != 3 {
		t.Errorf("应当只有 3 条落库，实际 %d 条", n)
	}
}

// 限流按频道分桶：一个频道被刷爆不该连累别的频道。
func TestSendLimitIsolatesChannels(t *testing.T) {
	s, r, noisy := newSendServer(t, 1, 2)
	u, err := service.NewAccount(s.DAO).GetByUsername("owner")
	if err != nil {
		t.Fatal(err)
	}
	quiet, err := service.NewChannel(s.DAO).Create(u.Id, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		postSend(r, noisy.Token, "x")
	}
	if w := postSend(r, noisy.Token, "x"); w.Code != http.StatusTooManyRequests {
		t.Fatalf("noisy 的额度该用光了，得到 %d", w.Code)
	}
	if w := postSend(r, quiet.Token, "x"); w.Code != http.StatusOK {
		t.Errorf("另一个频道不该受影响，得到 %d %s", w.Code, w.Body.String())
	}
}

// send_qps = 0 表示完全不限，给需要全开的自建实例留的口子。
func TestSendLimitDisabledWhenQPSZero(t *testing.T) {
	_, r, ch := newSendServer(t, 0, 0)
	for i := 0; i < 50; i++ {
		if w := postSend(r, ch.Token, "x"); w.Code != http.StatusOK {
			t.Fatalf("send_qps=0 应当不限，第 %d 条得到 %d", i+1, w.Code)
		}
	}
}

// 批量投递里被限的 token 各自报错，不连累同一批里其余的频道——
// 和「一个坏 token 不该让整批失败」是同一条原则。
func TestBatchReportsRateLimitPerToken(t *testing.T) {
	s, r, a := newSendServer(t, 1, 1)
	u, err := service.NewAccount(s.DAO).GetByUsername("owner")
	if err != nil {
		t.Fatal(err)
	}
	b, err := service.NewChannel(s.DAO).Create(u.Id, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}

	// 先把 a 的额度用掉
	if w := postSend(r, a.Token, "x"); w.Code != http.StatusOK {
		t.Fatal(w.Body.String())
	}

	payload, _ := json.Marshal(map[string]any{
		"tokens": []string{a.Token, b.Token},
		"body":   "批量",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send/batch", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.Token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var out struct {
		Data struct {
			Results []service.BatchResult `json:"results"`
			OK      int                   `json:"ok"`
			Failed  int                   `json:"failed"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("响应解析失败：%s", w.Body.String())
	}
	if len(out.Data.Results) != 2 {
		t.Fatalf("应当逐条报告 2 个 token，得到 %d 条：%s", len(out.Data.Results), w.Body.String())
	}
	// 顺序必须与请求里的 tokens 一致，调用方要能对上号
	if out.Data.Results[0].Token != a.Token || out.Data.Results[1].Token != b.Token {
		t.Errorf("结果顺序与请求不一致：%+v", out.Data.Results)
	}
	if out.Data.Results[0].OK {
		t.Error("a 的额度已用光，应当报错")
	}
	if !strings.Contains(out.Data.Results[0].Error, "频繁") {
		t.Errorf("a 的错误应当说明是频率问题：%q", out.Data.Results[0].Error)
	}
	if !out.Data.Results[1].OK {
		t.Errorf("b 不该受 a 连累：%q", out.Data.Results[1].Error)
	}
	if out.Data.OK != 1 || out.Data.Failed != 1 {
		t.Errorf("汇总数字不对：ok=%d failed=%d", out.Data.OK, out.Data.Failed)
	}
}

// 无效 token 不该消耗额度，否则拿一堆乱码 token 就能把别人的桶刷空。
func TestBatchInvalidTokenDoesNotConsumeQuota(t *testing.T) {
	_, r, ch := newSendServer(t, 1, 2)

	payload, _ := json.Marshal(map[string]any{
		"tokens": []string{"ch_NOPE", "ch_ALSONOPE", ch.Token},
		"body":   "x",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/send/batch", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+ch.Token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 真实频道那条应当成功，而且额度只被它自己消耗了一个
	if w := postSend(r, ch.Token, "x"); w.Code != http.StatusOK {
		t.Errorf("两个无效 token 不该消耗真实频道的额度，得到 %d", w.Code)
	}
}
