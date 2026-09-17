package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/gin-gonic/gin"
)

// 配置缺项时要有兜底，而且兜底值必须与 config.toml.example 写的默认值一致——
// 对不上的话，「照文档配」和「什么都不配」会得到两种行为。
func TestRouterFillsDefaultsForUnsetKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _ := newJoinServer(t, false, 0)
	if s.PairTTL != 10*time.Minute {
		t.Errorf("pair_ttl 兜底应当是 600 秒，得到 %s", s.PairTTL)
	}
	if s.PairPerMin != 5 {
		t.Errorf("pair_per_min 兜底应当是 5，得到 %d", s.PairPerMin)
	}
	if s.BodyMaxBytes != 1<<20 {
		t.Errorf("body_max_kb 兜底应当是 1024 KB，得到 %d 字节", s.BodyMaxBytes)
	}
}

// 配对码有效期原来在四处各写一遍 10 分钟，配置里那个键根本没人读。
// 现在它要同时决定库里的过期时间、cookie 的寿命和页面上的说法。
func TestPairTTLDrivesEverySurface(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, r := newJoinServer(t, true, 100)
	s.PairTTL = 2 * time.Minute

	w := get(r, "/", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("接入页返回 %d", w.Code)
	}
	// 默认语言是英文，所以断言的是英文那句
	if !strings.Contains(w.Body.String(), "Valid for 2 minutes") {
		t.Error("页面上仍然写着别的有效期")
	}
	ck := joinCookie(w)
	if ck == nil {
		t.Fatal("没拿到 cookie")
	}
	if ck.MaxAge != 120 {
		t.Errorf("cookie 寿命应当跟着有效期走，得到 %d 秒", ck.MaxAge)
	}

	var expires int64
	if _, err := s.DAO.Engine().SQL(
		"SELECT expires_at FROM pair_code WHERE code=?", ck.Value).Get(&expires); err != nil {
		t.Fatal(err)
	}
	if left := time.Until(time.Unix(expires, 0)); left > 2*time.Minute || left < time.Minute {
		t.Errorf("库里的过期时间不符：还剩 %s", left)
	}
}

// 配对接口的限流上限来自 limit.pair_per_min。配对码只有 40 位，
// 靠「单次使用 + 有限有效期 + 这条限流」三样一起兜住，缺一不可。
func TestPairEndpointUsesConfiguredRateLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	_, r := newServerWith(t, false, 100, func(s *Server) { s.PairPerMin = 2 })

	post := func() int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/pair",
			strings.NewReader(`{"pair_code":"ZZZZZZZZ","uuid":"u1"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	for i := 0; i < 2; i++ {
		if got := post(); got == http.StatusTooManyRequests {
			t.Fatalf("上限是 2，第 %d 次不该被限流", i+1)
		}
	}
	if got := post(); got != http.StatusTooManyRequests {
		t.Fatalf("第 3 次应当被限流，得到 %d", got)
	}
}

// 正文超过 limit.body_max_kb 要明确报错。
// 原来是 io.ReadAll(io.LimitReader(...))，超长的正文被静默截断落库：
// 调用方收到成功，存下来的却是半截内容。
func TestOversizeBodyIsRejectedNotTruncated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s, _ := newJoinServer(t, false, 0)
	s.BodyMaxBytes = 64

	r := gin.New()
	r.POST("/send", func(c *gin.Context) {
		c.Set(middleware.CtxUserID, int64(1))
		in, err := bindSend(c, s.BodyMaxBytes)
		if err != nil {
			c.String(http.StatusBadRequest, err.Error())
			return
		}
		c.String(http.StatusOK, in.Body)
	})

	long := strings.Repeat("x", 200)
	req := httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(long))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("超长正文应当被拒，得到 %d，正文长度 %d", w.Code, len(w.Body.String()))
	}
	if !strings.Contains(w.Body.String(), "exceeds the limit") {
		t.Errorf("错误提示没说清原因：%s", w.Body.String())
	}

	// 上限以内的照常收下，且一个字节都不能少
	ok := strings.Repeat("y", 64)
	req = httptest.NewRequest(http.MethodPost, "/send", strings.NewReader(ok))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("上限以内的正文被拒了：%s", w.Body.String())
	}
	if w.Body.String() != ok {
		t.Errorf("正文被改动了：收到 %d 字节，期望 %d", len(w.Body.String()), len(ok))
	}
}
