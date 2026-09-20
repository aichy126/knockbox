package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// home 首页。
//
// 公共模式下它是**对外宣传的落地页**：打开就是二维码，扫了就有自己的身份。
// 自建模式下服务器不该被陌生人扫码开账号，所以直接去登录页。
func (s *Server) home(c *gin.Context) {
	if !s.Settings.PublicEnabled() {
		c.Redirect(http.StatusFound, "/login")
		return
	}
	s.joinPage(c, "")
}

// joinCodeCookie 记住本次接入用的配对码，供刷新页面时复用。
const joinCodeCookie = "kb_join"

// joinPage 扫码接入页。开放注册码 user_id = 0，兑换那一刻才建用户。
func (s *Server) joinPage(c *gin.Context, notice string) {
	lang := reqLang(c)
	t := web.T(lang)
	pair := service.NewPair(s.DAO)

	// 刷新页面不该再签一张码：上一次那张存在 cookie 里，只要还没被用掉、
	// 剩余有效期也还够，就接着用。签发才是要限流的动作，页面加载不是。
	var code *service.PairCode
	if raw, err := c.Cookie(joinCodeCookie); err == nil {
		if rec, ok := pair.Pending(raw, s.ExternalURL, time.Minute); ok {
			code = rec
		}
	}
	if code == nil {
		if s.joinLimit != nil && !s.joinLimit.Allow(c.ClientIP()) {
			c.String(http.StatusTooManyRequests, t.ErrJoinTooFast)
			return
		}
		issued, err := pair.Issue(0, s.ExternalURL, "public", s.PairTTL)
		if err != nil {
			c.String(http.StatusInternalServerError, t.ErrIssueFailed+err.Error())
			return
		}
		code = issued
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie(joinCodeCookie, code.Code, int(s.PairTTL/time.Second), "/", "",
			strings.HasPrefix(s.ExternalURL, "https"), true)
	}
	link := code.DeepLink
	qr, err := web.QRSVG(link, 248, t.JoinQRAlt)
	if err != nil {
		c.String(http.StatusInternalServerError, t.ErrQRFailed+err.Error())
		return
	}
	page := web.JoinPage(web.JoinData{
		SiteName: s.Settings.SiteName(),
		Host:     s.ExternalURL,
		Code:     code.Code,
		QR:       qr,
		Link:     link,
		DocsURL:  s.Public.DocsURL,
		Notice:   notice,
		// 有效期不足一分钟时显示 1，不要写成「0 分钟内有效」。
		TTLMinutes: max(1, int(s.PairTTL/time.Minute)),
		// 这个状态接口让页面能自己发现「扫上了」，不用用户手动刷新
		StatusURL: "/join/status?c=" + code.Code,
		Lang:      lang,
	})
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

// joinStatus 页面轮询它来发现配对完成。
//
// 只回「用没用过」这一个比特，**不回任何凭据**——这个接口没有鉴权，
// 谁都能拿一个码来问；能问出的最多是「这个码被用掉了」。
func (s *Server) joinStatus(c *gin.Context) {
	// 公共模式关闭时这个接口不存在，与接入页的可见性保持一致。
	if !s.Settings.PublicEnabled() {
		c.Status(http.StatusNotFound)
		return
	}
	code := c.Query("c")
	if code == "" {
		c.JSON(http.StatusOK, gin.H{"paired": false})
		return
	}
	var rec models.PairCode
	ok, err := s.DAO.Engine().Where("code = ?", code).Get(&rec)
	if err != nil || !ok || rec.UsedAt == 0 {
		c.JSON(http.StatusOK, gin.H{"paired": false})
		return
	}

	out := gin.H{"paired": true}

	// 配对成功后把【这个人的专属地址】交出去——那一页有他自己的发送示例和测试按钮。
	//
	// ⚠️ 只在刚刚配对完的短窗口内给，而且这个接口没有鉴权：
	// 谁拿着那个配对码都能来问。合法的页面每 10 秒轮询一次，一定落在窗口内；
	// 而一张被截图流传出去的二维码，隔几分钟再来就什么也拿不到。
	const window = 5 * 60
	if time.Now().Unix()-rec.UsedAt <= window {
		var token string
		// app 配对后会自己建一个默认频道，取最早的那个
		_, _ = s.DAO.Engine().SQL(
			"SELECT token FROM channel WHERE user_id=? ORDER BY created_at LIMIT 1",
			rec.UserId).Get(&token)
		if token != "" {
			out["send_url"] = s.ExternalURL + "/s/" + token
		}
	}
	c.JSON(http.StatusOK, out)
}
