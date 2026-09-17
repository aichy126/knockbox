package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// adminPair 后台里的配对页。
//
// 和公共的接入页（/ 与 /join）是**两套东西**，不要混：
//
//	· 公共接入页：任何人打开都能扫，扫完【新建一个成员】。它是对外的门。
//	· 这一页：管理员用的，要能选【这个码发给谁】——给某个已有成员加一台设备，
//	  或者顺手建一个新成员再出码。公共模式关掉时，这是唯一的加人入口。
//
// 出码时的归属由页面上选择的成员决定，而不是固定为当前登录的管理员——
// 否则「给某个已有成员加一台设备」在界面上就没有入口。
func (s *Server) adminPairPage(c *gin.Context) {
	s.renderAdminPair(c, nil, "")
}

func (s *Server) adminPairIssue(c *gin.Context) {
	target := c.PostForm("user")
	name := strings.TrimSpace(c.PostForm("new_name"))

	var uid int64
	// 名字能对上就给那个人，对不上就当是要新建一个——
	// 让用户先去别处建人、再回来选，是没必要的一次往返。
	if uid := s.userIDByName(target); uid != 0 {
		s.issueFor(c, uid)
		return
	}
	if true {
		if name == "" {
			name = strings.TrimSpace(target)
		}
		if name == "" {
			s.renderAdminPair(c, nil, "填一个成员名字：已有的会直接用，没有的会新建。")
			return
		}
		now := time.Now().Unix()
		u := &models.User{Name: name, Role: models.RoleMember,
			Status: models.StatusActive, Ctime: now, Utime: now}
		if _, err := s.DAO.Engine().Insert(u); err != nil {
			s.renderAdminPair(c, nil, "建成员失败："+err.Error())
			return
		}
		uid = u.Id
	}
	s.issueFor(c, uid)
}

func (s *Server) issueFor(c *gin.Context, uid int64) {
	code, err := service.NewPair(s.DAO).Issue(uid, s.ExternalURL, "admin", s.PairTTL)
	if err != nil {
		s.renderAdminPair(c, nil, "签发配对码失败："+err.Error())
		return
	}
	s.renderAdminPair(c, code, "")
}

func (s *Server) renderAdminPair(c *gin.Context, code *service.PairCode, errMsg string) {
	me := middleware.UserID(c)
	users, _ := s.DAO.Engine().QueryString("SELECT id, name FROM user ORDER BY id")

	var b strings.Builder
	b.WriteString(`<div class="ph"><div><h1>配对设备</h1>` +
		`<div class="sub">给某个成员加一台设备。新成员也可以在这里顺手建。</div></div></div>`)
	if errMsg != "" {
		b.WriteString(`<div class="err">` + web.E(errMsg) + `</div>`)
	}

	// 可搜索：公共实例上成员会有几百个，下拉框那时是一条滚不完的列表。
	// 输入的是名字（用户记得住的东西），服务端按名字反查。
	var myName string
	_, _ = s.DAO.Engine().SQL("SELECT name FROM user WHERE id=?", me).Get(&myName)
	var sel strings.Builder
	fmt.Fprintf(&sel, `<span class="picker"><input name="user" list="pOpts" value="%s" `+
		`placeholder="成员名字" autocomplete="off"><datalist id="pOpts">`, web.E(myName))
	for _, u := range users {
		fmt.Fprintf(&sel, `<option value="%s">`, web.E(u["name"]))
	}
	fmt.Fprintf(&sel, `</datalist><span class="hint">%d 人</span></span>`, len(users))

	form := `<div class="card-b"><form method="post" action="/admin/pair" class="tools">` +
		`<span class="dim">发给</span>` + sel.String() +
		`<button class="btn" type="submit">` + web.Svg("qr", 15) + `生成配对码</button>` +
		`</form>` +
		`<div class="dim" style="font-size:12.5px;margin-top:8px">` +
		`填已有成员的名字就是给他加一台设备；填一个没有的名字会顺手建一个新成员。</div></div>`
	b.WriteString(web.Card("给谁", "", form))

	if code != nil {
		qr, err := web.QRSVG(code.DeepLink, 240)
		if err != nil {
			qr = `<div class="dim">二维码生成失败</div>`
		}
		var owner string
		_, _ = s.DAO.Engine().SQL("SELECT name FROM user WHERE id=?", code.UserID).Get(&owner)
		body := `<div class="card-b" style="display:grid;grid-template-columns:280px 1fr;gap:24px;align-items:start">` +
			`<div style="text-align:center">` + qr +
			`<div class="code" style="margin-top:10px">` + web.E(code.Display) + `</div>` +
			`<div class="host">` + web.E(s.ExternalURL) + `</div></div>` +
			`<div><ol class="steps">` +
			`<li><b>1</b><div>在那台设备上装好 Knockbox 并打开</div></li>` +
			`<li><b>2</b><div>点「添加服务器」，扫左边这个码</div></li>` +
			`<li><b>3</b><div>扫完这台设备就属于 <b>` + web.E(owner) + `</b> 了，和他的其它设备收同样的消息</div></li>` +
			`</ol><div class="note" style="margin-top:14px">` + web.Svg("alert", 14) +
			`<p>只能用一次、10 分钟内有效。码里只有配对码，没有长期凭证——截图外泄也换不来什么。</p></div></div></div>`
		b.WriteString(web.Card("扫这个码", "", body))
	}

	s.shell(c, "pair", []web.Crumb{web.C("配对设备")}, b.String())
}

var _ = http.StatusOK
