package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
