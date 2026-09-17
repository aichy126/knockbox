package api

import (
	"net/http"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// fail 把一个错误变成响应。
//
// 归过类的（uierr）按请求方的 Accept-Language 渲染成一句话放进 msg，
// 同时把 code 放进 data.error_code：老客户端照旧显示 msg——而且拿到的是
// 它自己那门语言的那一份；新客户端读 code 自己组织措辞，那才是真正的本地化。
//
// 没归过类的原样透出。那批是发送 API 的契约校验，读它的是写脚本的人，
// 英文成句本来就对；而真正的内部错误不该走到这里来。
func (s *Server) fail(c *gin.Context, err error) {
	ue, ok := uierr.As(err)
	if !ok {
		res.Rfail(c, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": res.CodeFail,
		"msg":  s.userText(c, ue.Code, ue.Args...),
		"data": gin.H{"error_code": ue.Code},
	})
}

// userText 按这次请求的语言渲染一句给人看的话。语言判定复用 reqLang，
// 免得同一件事在两处各写一遍、日后只改了一处。
func (s *Server) userText(c *gin.Context, code string, args ...any) string {
	return web.UserError(reqLang(c), code, args...)
}

// notFoundText 附件那条路返回的是文件不是 JSON，所以失败时也只给一行字。
func (s *Server) notFoundText(c *gin.Context, code string) {
	c.String(http.StatusNotFound, s.userText(c, code))
}
