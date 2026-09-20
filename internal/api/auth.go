package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) html(c *gin.Context, status int, name string, d web.Data) {
	d.ServerName = s.Name
	out, err := web.Render(name, reqLang(c), d)
	if err != nil {
		c.String(http.StatusInternalServerError, "rendering the page failed: %v", err)
		return
	}
	c.Data(status, "text/html; charset=utf-8", []byte(out))
}

func (s *Server) loginPage(c *gin.Context) {
	if raw, err := c.Cookie(middleware.SessionCookie); err == nil && raw != "" {
		if _, err := service.NewSession(s.DAO).Verify(raw); err == nil {
			c.Redirect(http.StatusFound, afterLogin(c))
			return
		}
	}
	t := web.T(reqLang(c)).Admin.Login
	d := web.Data{Title: t.Title}
	if c.Query("changed") == "1" {
		d.Notice = t.Changed
	}
	s.html(c, http.StatusOK, "login", d)
}

func (s *Server) doLogin(c *gin.Context) {
	raw, _, err := service.NewSession(s.DAO).Login(
		c.PostForm("username"), c.PostForm("password"), c.GetHeader("User-Agent"), c.ClientIP())
	if err != nil {
		// 用户名不存在和密码错误给同一句话，不让登录页变成账号枚举器。
		t := web.T(reqLang(c)).Admin.Login
		s.html(c, http.StatusUnauthorized, "login", web.Data{Title: t.Title, Error: t.Bad})
		return
	}
	s.setSessionCookie(c, raw, int(service.SessionTTL/time.Second))
	c.Redirect(http.StatusFound, afterLogin(c))
}

func (s *Server) logout(c *gin.Context) {
	if raw, err := c.Cookie(middleware.SessionCookie); err == nil {
		_ = service.NewSession(s.DAO).Logout(raw)
	}
	s.setSessionCookie(c, "", -1)
	c.Redirect(http.StatusFound, "/login")
}

// setSessionCookie HttpOnly 挡住 JS 读取，SameSite=Lax 挡 CSRF；
// Secure 只在对外地址是 https 时加——自建者可能先用 http 在内网跑通。
func (s *Server) setSessionCookie(c *gin.Context, val string, maxAge int) {
	secure := len(s.ExternalURL) > 5 && s.ExternalURL[:5] == "https"
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(middleware.SessionCookie, val, maxAge, "/", "", secure, true)
}

// afterLogin 登录后去哪。
//
// 带 next 就回他本来要去的页面——被中间件挡下来之后还要自己找回去，是很烦的一件事。
// **只接受站内的绝对路径**：不校验的话这就是个开放重定向，钓鱼链接能拿它做跳板。
func afterLogin(c *gin.Context) string {
	next := c.Query("next")
	if next == "" {
		next = c.PostForm("next")
	}
	if strings.HasPrefix(next, "/") && !strings.HasPrefix(next, "//") {
		return next
	}
	return "/admin"
}
