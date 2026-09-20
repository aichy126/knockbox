package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/aichy126/knockbox/internal/uierr"
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

	// 名字能对上就给那个人，对不上就当是要新建一个——
	// 让用户先去别处建人、再回来选，是没必要的一次往返。
	if uid := s.admin().MemberIdByName(strings.TrimSpace(target)); uid != 0 {
		s.issueFor(c, uid)
		return
	}
	if name == "" {
		name = strings.TrimSpace(target)
	}
	if name == "" {
		s.renderAdminPair(c, nil, s.userText(c, uierr.PairNoTarget))
		return
	}
	u, err := s.admin().CreateMember(name)
	if err != nil {
		s.renderAdminPair(c, nil, fmt.Sprintf(web.T(reqLang(c)).Admin.Pair.CreateFail, err.Error()))
		return
	}
	s.issueFor(c, u.Id)
}

func (s *Server) issueFor(c *gin.Context, uid int64) {
	code, err := service.NewPair(s.DAO).Issue(uid, s.ExternalURL, "admin", s.PairTTL)
	if err != nil {
		s.renderAdminPair(c, nil, fmt.Sprintf(web.T(reqLang(c)).Admin.Pair.IssueFail, err.Error()))
		return
	}
	s.renderAdminPair(c, code, "")
}

func (s *Server) renderAdminPair(c *gin.Context, code *service.PairCode, errMsg string) {
	v := newAdminView(c)
	me := middleware.UserID(c)
	users, err := s.admin().MemberNames()
	if err != nil {
		log.Error("admin: member names failed", log.Any("error", err.Error()))
	}

	var b strings.Builder
	b.WriteString(`<div class="ph"><div><h1>` + web.E(v.t.Nav.Pair) + `</h1>` +
		`<div class="sub">` + web.E(v.t.Pair.Sub) + `</div></div></div>`)
	if errMsg != "" {
		b.WriteString(`<div class="err">` + web.E(errMsg) + `</div>`)
	}

	// 可搜索：公共实例上成员会有几百个，下拉框那时是一条滚不完的列表。
	// 输入的是名字（用户记得住的东西），服务端按名字反查。
	myName := s.admin().MemberName(me)
	var sel strings.Builder
	fmt.Fprintf(&sel, `<span class="picker"><input name="user" list="pOpts" value="%s" `+
		`placeholder="%s" autocomplete="off"><datalist id="pOpts">`,
		web.E(myName), web.E(v.t.Pair.NamePh))
	for _, u := range users {
		fmt.Fprintf(&sel, `<option value="%s">`, web.E(u.Name))
	}
	fmt.Fprintf(&sel, `</datalist><span class="hint">%s</span></span>`,
		web.E(web.Plural(len(users), v.t.Common.PeopleOne, v.t.Common.PeopleCount)))

	form := `<div class="card-b"><form method="post" action="/admin/pair" class="tools">` +
		`<span class="dim">` + web.E(v.t.Pair.SendTo) + `</span>` + sel.String() +
		`<button class="btn" type="submit">` + web.Svg("qr", 15) +
		web.E(v.t.Pair.Issue) + `</button>` +
		`</form>` +
		`<div class="dim" style="font-size:12.5px;margin-top:8px">` +
		web.E(v.t.Pair.Hint) + `</div></div>`
	b.WriteString(web.Card(v.t.Pair.CardWho, "", form))

	if code != nil {
		qr, err := web.QRSVG(code.DeepLink, 240)
		if err != nil {
			qr = `<div class="dim">` + web.E(v.t.Pair.QRFail) + `</div>`
		}
		owner := s.admin().MemberName(code.UserID)
		body := `<div class="card-b" style="display:grid;grid-template-columns:280px 1fr;gap:24px;align-items:start">` +
			`<div style="text-align:center">` + qr +
			`<div class="code" style="margin-top:10px">` + web.E(code.Display) + `</div>` +
			`<div class="host">` + web.E(s.ExternalURL) + `</div></div>` +
			`<div><ol class="steps">` +
			`<li><b>1</b><div>` + web.E(v.t.Pair.Step1) + `</div></li>` +
			`<li><b>2</b><div>` + web.E(v.t.Pair.Step2) + `</div></li>` +
			// Step3 的语料自带 <b>，名字仍然要转义——它是用户起的。
			`<li><b>3</b><div>` + fmt.Sprintf(v.t.Pair.Step3, web.E(owner)) + `</div></li>` +
			`</ol><div class="note" style="margin-top:14px">` + web.Svg("alert", 14) +
			// 有效期跟着 server.pair_ttl 走，不写死——配置改了而文案不改，
			// 这一句就开始骗人。
			`<p>` + web.E(fmt.Sprintf(v.t.Pair.Note, int(s.PairTTL.Minutes()))) +
			`</p></div></div></div>`
		b.WriteString(web.Card(v.t.Pair.CardScan, "", body))
	}

	s.shell(c, "pair", []web.Crumb{web.C(v.t.Nav.Pair)}, b.String())
}

var _ = http.StatusOK
