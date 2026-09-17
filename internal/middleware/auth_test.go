package middleware

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aichy126/knockbox/internal/models"
	"github.com/gin-gonic/gin"
)

// 取发送凭据时，URL 路径参数必须压过 Authorization 头。
//
// 若让头优先，已持有设备 token 的客户端调用发送接口时会带上设备 token，
// 服务端拿它去查频道必然落空，返回一个与实际原因无关的 401；
// 而同一条 URL 在不带该头时却是成功的。
func TestSendTokenPrefersPathOverAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got string
	r := gin.New()
	r.POST("/api/v1/send/:token", func(c *gin.Context) { got = SendToken(c); c.Status(200) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/send/ch_from_path", nil)
	req.Header.Set("Authorization", "Bearer device_token_from_header")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if got != "ch_from_path" {
		t.Fatalf("应当取路径里的 ch_from_path，实际取到 %q", got)
	}
}

// 没有路径参数时（POST /api/v1/send），照旧认 Authorization 头。
func TestSendTokenFallsBackToAuthorizationHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var got string
	r := gin.New()
	r.POST("/api/v1/send", func(c *gin.Context) { got = SendToken(c); c.Status(200) })

	req := httptest.NewRequest(http.MethodPost, "/api/v1/send", nil)
	req.Header.Set("Authorization", "Bearer ch_from_header")
	r.ServeHTTP(httptest.NewRecorder(), req)

	if got != "ch_from_header" {
		t.Fatalf("应当回落到头里的 ch_from_header，实际取到 %q", got)
	}
}

// 后台的 JSON 接口在会话过期时必须回 401 JSON，【绝不能】回 302。
//
// 浏览器 fetch 默认 redirect:"follow"：一个 302 会被跟到 /login，
// 拿回 200 的 HTML，然后在 res.json() 上炸掉——前端看到的报错和真实原因无关。
// 这条断言专门盯着 Accept: text/html 那一路，因为那正是浏览器自己会带上的头。
func TestAdminAPINeverRedirects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AdminAuth(
		func(string) (*models.User, error) { return nil, errors.New("no session") },
		"admin.session_expired",
		func(_ *gin.Context, code string, _ ...any) string { return "rendered:" + code },
	))
	r.GET("/admin/api/me", func(c *gin.Context) { c.Status(http.StatusOK) })
	r.GET("/admin/settings", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, accept := range []string{"", "text/html,application/xhtml+xml", "application/json"} {
		req := httptest.NewRequest(http.MethodGet, "/admin/api/me", nil)
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("Accept=%q：想要 401，得到 %d（Location=%q）", accept, w.Code, w.Header().Get("Location"))
		}
		var body struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
			Data struct {
				ErrorCode string `json:"error_code"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("Accept=%q：响应不是 JSON：%s", accept, w.Body.String())
		}
		if body.Code != 1 {
			t.Errorf("Accept=%q：code 应当是 1，得到 %d", accept, body.Code)
		}
		if body.Data.ErrorCode != "admin.session_expired" {
			t.Errorf("Accept=%q：缺 data.error_code，得到 %q", accept, body.Data.ErrorCode)
		}
		// msg 必须是渲染过的，不是硬编码的中文——那是「按读的人的语言说话」的落点
		if body.Msg != "rendered:admin.session_expired" {
			t.Errorf("Accept=%q：msg 没走 render，得到 %q", accept, body.Msg)
		}
	}
}

// 而人在地址栏里敲后台地址时，照旧跳登录页并带上原地址——
// 这一路没有被上面那条改掉。
func TestAdminPageStillRedirectsBrowserToLogin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AdminAuth(
		func(string) (*models.User, error) { return nil, errors.New("no session") },
		"admin.session_expired",
		func(_ *gin.Context, code string, _ ...any) string { return code },
	))
	r.GET("/admin/settings", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/admin/settings?tab=server", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("想要 302，得到 %d", w.Code)
	}
	if got, want := w.Header().Get("Location"), "/login?next=%2Fadmin%2Fsettings%3Ftab%3Dserver"; got != want {
		t.Errorf("Location = %q，想要 %q", got, want)
	}
}
