package api

import (
	"net/http"

	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/gin-gonic/gin"
)

// setSessionCookie HttpOnly 挡住 JS 读取，SameSite=Lax 挡 CSRF；
// Secure 只在对外地址是 https 时加——自建者可能先用 http 在内网跑通。
//
// 登录与登出本身在 admin_api_write.go：前端要的是一个能判成败的信封，
// 而不是表单 + 302——fetch 会把 302 跟到登录页再在 res.json() 上炸掉。
func (s *Server) setSessionCookie(c *gin.Context, val string, maxAge int) {
	secure := len(s.ExternalURL) > 5 && s.ExternalURL[:5] == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.SessionCookie, val, maxAge, "/", "", secure, true)
}
