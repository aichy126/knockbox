package api

import (
	"net/http"
	"strings"

	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/gin-gonic/gin"
)

// langCookieMaxAge 语言偏好留多久。一年——它是个偏好，不是会话状态。
const langCookieMaxAge = 365 * 24 * 3600

// setLang 管理界面的语言开关落在这里：记下选择，再跳回原来那一页。
//
// 公开页不走这条路（它只按本次请求判定），所以这个 handler 存在的唯一理由
// 就是「后台要记住」。它不需要登录态：cookie 只影响渲染语言，换一门语言
// 看不到任何多余的东西。
func (s *Server) setLang(c *gin.Context) {
	if l, ok := web.KnownLang(c.Query("to")); ok {
		c.SetSameSite(http.SameSiteLaxMode)
		// HttpOnly：页面是服务端直出的，没有哪段脚本需要读它。
		c.SetCookie(web.LangCookie, string(l), langCookieMaxAge, "/", "", false, true)
	}
	c.Redirect(http.StatusFound, safeNext(c.Query("next")))
}

// safeNext 把 next 限制成本站的一个路径。
//
// 这是个开放重定向面：不校验的话，`/lang?to=en&next=https://evil.example`
// 就成了一条挂在你域名下的跳转链接。规则只认「以单个 / 开头」——
// `//evil.example` 是协议相对 URL，浏览器会当成外站；反斜杠有的浏览器
// 也按斜杠解析，所以一并挡掉。
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") ||
		strings.HasPrefix(next, "//") ||
		strings.HasPrefix(next, "/\\") ||
		strings.Contains(next, "\\") {
		return "/admin"
	}
	return next
}

// asset 发一个内嵌的静态文件。图标跟着二进制走，改它要发版，
// 所以缓存可以放心地长——一年，和其它不带指纹的站点图标一样。
func (s *Server) asset(name string) gin.HandlerFunc {
	body, mime := web.Asset(name)
	return func(c *gin.Context) {
		if body == nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Data(http.StatusOK, mime, body)
	}
}
