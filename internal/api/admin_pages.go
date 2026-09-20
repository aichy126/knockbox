package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/library/bytesize"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// ── 消息 ──────────────────────────────────────────────

func (s *Server) adminMessages(c *gin.Context) {
	v := newAdminView(c)
	q := strings.TrimSpace(c.Query("q"))
	userFilter := c.Query("user")
	chFilter := c.Query("channel")
	before, _ := strconv.ParseInt(c.Query("before"), 10, 64)
	const pageSize = 40

	// 不按当前登录者过滤：管理界面的定位是看到这台服务器上的全部消息。
	// 固定成 m.user_id = 登录者的话，公共实例上管理员将看不到任何其他人的消息。
	rows, err := s.admin().Messages(service.MessageFilter{
		Query: q,
		// picker 传的是名字。查不到就【当作没筛选】而不是返回空——
		// 名字打错时给一个空列表，用户会以为「这个人没有消息」。
		UserId:    s.admin().MemberIdByName(strings.TrimSpace(userFilter)),
		ChannelId: chFilter,
		Before:    before,
		Limit:     pageSize + 1,
	})
	if err != nil {
		log.Error("admin: message search failed", log.Any("error", err.Error()))
	}
	more := len(rows) > pageSize
	if more {
		rows = rows[:pageSize]
	}

	var b strings.Builder
	b.WriteString(`<div class="ph"><div><h1>` + web.E(v.t.Nav.Messages) + `</h1>` +
		`<div class="sub">` + web.E(v.t.Msgs.Sub) + `</div></div></div>`)
	b.WriteString(s.searchForm(v, q, userFilter, chFilter))

	if len(rows) == 0 {
		b.WriteString(web.Card("", "", web.Empty(emptyHint(v, q, chFilter))))
	} else {
		var t strings.Builder
		fmt.Fprintf(&t, `<table><thead><tr><th style="padding-left:16px">%s</th><th>%s</th><th>%s</th>`+
			`<th>%s</th><th>%s</th><th>%s</th><th></th></tr></thead><tbody>`,
			web.E(v.t.Common.Time), web.E(v.t.Common.Member), web.E(v.t.Common.Channel),
			web.E(v.t.Common.Type), web.E(v.t.Common.Title), web.E(v.t.Common.Push))
		var last int64
		for _, r := range rows {
			last = r.Id
			readMark := ""
			if r.ReadAt == 0 {
				readMark = `<span class="badge info"><i></i>` + web.E(v.t.Common.Unread) + `</span>`
			}
			fmt.Fprintf(&t, `<tr><td style="padding-left:16px" class="dim num">%s</td>`+
				`<td class="dim"><a href="/admin/users/%d">%s</a></td>`+
				`<td><a href="/admin/channels/%s">%s</a></td>`+
				`<td><span class="badge muted">%s</span></td>`+
				`<td style="font-weight:500"><a href="/admin/messages/%s">%s</a></td>`+
				`<td>%s</td><td style="text-align:right;padding-right:16px">%s</td></tr>`,
				clock(r.Ctime), r.UserId, web.E(r.Owner),
				web.E(r.ChannelId), web.E(channelName(r.Meta, r.ChannelId)),
				web.E(r.Type), web.E(r.UID), web.E(trunc(firstNonEmpty(r.Title, r.Summary), 42)),
				pushBadge(v, r.PushOK, r.PushTotal), readMark)
		}
		t.WriteString(`</tbody></table>`)
		if more {
			fmt.Fprintf(&t, `<div class="pager"><a class="btn outline sm" `+
				`href="?q=%s&user=%s&channel=%s&before=%d">%s%s</a></div>`,
				web.E(q), web.E(userFilter), web.E(chFilter), last,
				web.E(v.t.Common.Older), web.Svg("chev", 14))
		}
		b.WriteString(web.Card("", "", t.String()))
	}
	s.shell(c, "messages", []web.Crumb{web.C(v.t.Nav.Messages)}, b.String())
}

func emptyHint(v adminView, q, ch string) string {
	if q != "" {
		return v.t.Msgs.EmptyQuery
	}
	if ch != "" {
		return v.t.Channel.Empty
	}
	return v.t.Dash.Empty
}

// userPicker 可搜索的成员选择器。
//
// 用 <input list> + <datalist> 而不是 <select>：成员数在公共实例上会到几百，
// 下拉框那时是一条滚不完的列表。datalist 既能打字过滤又能点开看全部，
// 浏览器原生支持，不用引任何组件库、不写一行组件代码。
//
// 值用「名字」而不是 id：用户输入的是名字，让他去记一个数字 id 是荒唐的。
// 服务端按名字反查，查不到就当没筛选——**不能因为名字打错就返回空列表**，
// 那样用户会以为「这个人没有消息」。
func (s *Server) userPicker(v adminView, name, current, placeholder string) string {
	rows, err := s.admin().MemberNames()
	if err != nil {
		log.Error("admin: member names failed", log.Any("error", err.Error()))
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<span class="picker"><input name="%s" list="%s_opts" value="%s" placeholder="%s">`,
		name, name, web.E(current), web.E(placeholder))
	fmt.Fprintf(&b, `<datalist id="%s_opts">`, name)
	for _, r := range rows {
		fmt.Fprintf(&b, `<option value="%s">`, web.E(r.Name))
	}
	b.WriteString(`</datalist>`)
	fmt.Fprintf(&b, `<span class="hint">%s</span></span>`,
		web.E(web.Plural(len(rows), v.t.Common.PeopleOne, v.t.Common.PeopleCount)))
	return b.String()
}

// searchForm 成员 → 频道 的联动筛选。
//
// 频道全部渲染出来、按成员在前端过滤，而不是选完成员再往服务端跑一趟：
// 一台服务器上的频道总数是几十量级，一次带出来远比一次往返便宜，
// 而且切成员时列表立刻就变，不闪。
func (s *Server) searchForm(v adminView, q, userFilter, chFilter string) string {
	chans, err := s.admin().AllChannels()
	if err != nil {
		log.Error("admin: channel list failed", log.Any("error", err.Error()))
	}
	ownerRows, err := s.admin().MemberNames()
	if err != nil {
		log.Error("admin: member names failed", log.Any("error", err.Error()))
	}
	ownerName := map[int64]string{}
	for _, r := range ownerRows {
		ownerName[r.Id] = r.Name
	}

	var b strings.Builder
	b.WriteString(`<form class="tools" method="get" id="mf">`)
	fmt.Fprintf(&b, `<input name="q" placeholder="%s" value="%s" style="min-width:220px">`,
		web.E(v.t.Msgs.SearchPh), web.E(q))

	b.WriteString(s.userPicker(v, "user", userFilter, v.t.Msgs.AllMembers))

	b.WriteString(`<select name="channel" id="cSel"><option value="">` +
		web.E(v.t.Msgs.AllChannels) + `</option>`)
	for _, ch := range chans {
		sel := ""
		if ch.Id == chFilter {
			sel = " selected"
		}
		fmt.Fprintf(&b, `<option value="%s" data-user="%d" data-owner-name="%s"%s>%s</option>`,
			web.E(ch.Id), ch.UserId, web.E(ownerName[ch.UserId]), sel,
			web.E(channelName(ch.Meta, ch.Id)))
	}
	b.WriteString(`</select>`)

	b.WriteString(`<button class="btn" type="submit">` + web.E(v.t.Common.Search) + `</button>`)
	if q != "" || chFilter != "" || userFilter != "" {
		b.WriteString(`<a class="btn ghost" href="/admin/messages">` +
			web.E(v.t.Common.Clear) + `</a>`)
	}
	b.WriteString(`</form>`)

	// 联动：换成员就把不属于他的频道藏掉。若当前选中的频道不属于新成员，回到「全部频道」——
	// 留着一个对不上的组合，搜出来永远是空，而用户看不出是为什么。
	// 联动：按【成员名字】过滤频道。
	// picker 里用户输入的就是名字，绕回 id 再比一遍只是多一层可能出错的映射。
	// 当前选中的频道若不属于新成员，回到「全部频道」——留着一个对不上的组合，
	// 搜出来永远是空，而用户看不出为什么。
	b.WriteString(`<script>(function(){
  var u=document.querySelector('input[name=user]'),c=document.getElementById('cSel');
  if(!u||!c)return;
  function sync(){
    var name=u.value.trim(), keep=false;
    Array.prototype.forEach.call(c.options,function(o){
      if(!o.value)return;
      var show=!name||o.dataset.ownerName===name;
      o.hidden=!show;
      if(o.selected&&show)keep=true;
    });
    if(!keep)c.value='';
  }
  u.addEventListener('change',sync); u.addEventListener('input',sync); sync();
})()</script>`)
	return b.String()
}

// adminMessageDetail 单条消息。管理员能看到正文——这是自持服务器的固有属性，
// 页面上不需要特意声明，但也不遮掩。
func (s *Server) adminMessageDetail(c *gin.Context) {
	v := newAdminView(c)
	msg, channel, err := s.admin().Message(c.Param("uid"))
	if err != nil {
		s.shell(c, "messages", []web.Crumb{web.C(v.t.Msgs.Crumb), web.C(v.t.Common.NotFound)},
			web.Card("", "", web.Empty(s.userText(c, uierr.MessageNotFound))))
		return
	}
	m, ch := *msg, *channel

	var b strings.Builder
	title := m.Title
	if title == "" {
		title = m.Summary
	}
	b.WriteString(`<div class="ph"><div><h1>` + web.E(trunc(title, 60)) + `</h1><div class="sub">` +
		web.E(channelName(ch.Meta, m.ChannelId)) + ` · ` + web.E(m.Type) + ` · ` +
		web.E(time.Unix(m.Ctime, 0).Format("2006-01-02 15:04:05")) + `</div></div></div>`)

	body := m.Body
	if body == "" {
		body = m.Summary
	}
	b.WriteString(web.Card(v.t.Msgs.CardBody, "", `<div class="card-b"><pre class="body">`+web.E(body)+`</pre></div>`))
	if m.Extra != "" {
		b.WriteString(web.Card(v.t.Msgs.CardExtra, "", `<div class="card-b"><pre class="body">`+web.E(m.Extra)+`</pre></div>`))
	}
	b.WriteString(web.Card(v.t.Msgs.CardSummary, "",
		`<div class="card-b"><pre class="body">`+web.E(m.Summary)+`</pre></div>`))
	if m.Replyable() {
		b.WriteString(web.Card(v.t.Msgs.CardReply, "", s.replyPanel(v, &m)))
	}
	b.WriteString(web.Card(v.t.Channel.DeliveryLog, "", s.pushLogTable(v, m.Id)))
	s.shell(c, "messages", []web.Crumb{
		web.C(v.t.Nav.Messages, "/admin/messages"), web.C(trunc(title, 24)),
	}, b.String())
}

// replyPanel 一条可回复消息的回复状态与回调投递结果。
//
// 这一屏是【回调失败时唯一的线索】：用户那边显示「已回复」，发送方什么都没收到，
// 两边都不会自己发现这件事。没有这一屏，排查只能去翻日志。
func (s *Server) replyPanel(v adminView, m *models.Message) string {
	var b strings.Builder
	b.WriteString(`<div class="card-b">`)

	now := time.Now().Unix()
	switch {
	case m.Replied():
		fmt.Fprintf(&b, `<p><span class="badge ok"><i></i>%s</span> <b>%s</b> `+
			`<span class="dim">%s</span></p>`,
			web.E(v.t.Channel.Replied), web.E(m.Reply),
			web.E(time.Unix(m.RepliedAt, 0).Format("2006-01-02 15:04:05")))
	case m.ReplyExpired(now):
		fmt.Fprintf(&b, `<p><span class="badge warn"><i></i>%s</span> `+
			`<span class="dim">%s</span></p>`,
			web.E(v.t.Msgs.ReplyExpired),
			web.E(fmt.Sprintf(v.t.Msgs.ReplyExpSub, time.Unix(m.ReplyUntil, 0).Format("2006-01-02 15:04:05"))))
	case m.ReplyUntil > 0:
		fmt.Fprintf(&b, `<p><span class="badge muted"><i></i>%s</span> `+
			`<span class="dim">%s</span></p>`,
			web.E(v.t.Msgs.ReplyWaiting),
			web.E(fmt.Sprintf(v.t.Msgs.ReplyUntil, time.Unix(m.ReplyUntil, 0).Format("2006-01-02 15:04:05"))))
	default:
		b.WriteString(`<p><span class="badge muted"><i></i>` + web.E(v.t.Msgs.ReplyWaiting) +
			`</span> <span class="dim">` + web.E(v.t.Msgs.ReplyNoLimit) + `</span></p>`)
	}
	// 回调地址只在后台显示。它不下发给任何客户端，但服务器的主人排障时要看得到。
	fmt.Fprintf(&b, `<p class="dim">%s</p>`,
		fmt.Sprintf(web.E(v.t.Msgs.ReplyWebhook), `<span class="mono">`+web.E(m.ReplyWebhook)+`</span>`))
	b.WriteString(`</div>`)

	rows, err := s.admin().ReplyHooks(m.Id)
	if err != nil {
		log.Error("admin: reply hooks failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		if m.Replied() {
			// 回复和入队在同一个事务里，所以这种情况说明数据被写坏了。
			b.WriteString(web.Empty(v.t.Msgs.ReplyNoHooks))
		}
		return b.String()
	}
	fmt.Fprintf(&b, `<table><thead><tr><th style="padding-left:16px">%s</th><th>%s</th>`+
		`<th>HTTP</th><th>%s</th><th>%s</th></tr></thead><tbody>`,
		web.E(v.t.Msgs.ColHook), web.E(v.t.Common.Attempts),
		web.E(v.t.Common.Reason), web.E(v.t.Common.Time))
	for _, r := range rows {
		code := v.t.Common.Dash
		if r.StatusCode != 0 {
			code = strconv.Itoa(r.StatusCode)
		}
		fmt.Fprintf(&b, `<tr><td style="padding-left:16px">%s</td><td class="num">%d</td>`+
			`<td class="num dim">%s</td><td class="dim">%s</td><td class="dim num">%s</td></tr>`,
			hookBadge(v, r.Status), r.Attempt,
			web.E(code), web.E(trunc(r.Error, 60)), clock(r.Utime))
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func hookBadge(v adminView, status int) string {
	switch status {
	case 1:
		return `<span class="badge ok"><i></i>` + web.E(v.t.Common.Delivered) + `</span>`
	case 2:
		return `<span class="badge warn"><i></i>` + web.E(v.t.Common.Retrying) + `</span>`
	case 3:
		// 「已放弃」要显眼：发送方永远收不到这个答案了，而它自己不会知道。
		return `<span class="badge err"><i></i>` + web.E(v.t.Common.GivenUp) + `</span>`
	}
	return `<span class="badge muted"><i></i>` + web.E(v.t.Common.Queued) + `</span>`
}

func (s *Server) pushLogTable(v adminView, msgID int64) string {
	rows, err := s.admin().PushLog(msgID)
	if err != nil {
		log.Error("admin: push log failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		return web.Empty(v.t.Msgs.NoPushLog)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<table><thead><tr><th style="padding-left:16px">%s</th><th>%s</th>`+
		`<th>%s</th><th>%s</th><th>apns-id</th><th>%s</th></tr></thead><tbody>`,
		web.E(v.t.Common.Device), web.E(v.t.Msgs.ColEnv), web.E(v.t.Common.Status),
		web.E(v.t.Common.Attempts), web.E(v.t.Common.Time))
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><td style="padding-left:16px">%s</td><td class="dim">%s</td>`+
			`<td>%s</td><td class="num">%d</td><td class="mono dim">%s</td><td class="dim num">%s</td></tr>`,
			web.E(r.Device), web.E(r.APNsEnv), statusBadge(v, r.Status, r.HTTPStatus, r.Reason),
			r.Attempts, web.E(trunc(r.APNsID, 12)), clock(r.Utime))
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func statusBadge(v adminView, status, httpStatus int, reason string) string {
	switch status {
	case 1:
		return `<span class="badge ok"><i></i>` +
			web.E(fmt.Sprintf(v.t.Msgs.DeliveredCode, strconv.Itoa(httpStatus))) + `</span>`
	case 3:
		return `<span class="badge err"><i></i>` + web.E(firstNonEmpty(reason, v.t.Common.Failed)) + `</span>`
	case 2:
		return `<span class="badge warn"><i></i>` + web.E(v.t.Common.Retrying) + `</span>`
	}
	return `<span class="badge muted"><i></i>` + web.E(v.t.Common.Queued) + `</span>`
}

// ── 频道 ──────────────────────────────────────────────

// ── 设备 ──────────────────────────────────────────────

func (s *Server) adminDeviceDelete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	owner, err := s.admin().RevokeDevice(id)
	if err != nil {
		// 回到点「注销」的那一页。拿不到属主（设备本来就不存在）就回成员列表。
		back := "/admin/users"
		if owner != 0 {
			back = "/admin/users/" + strconv.FormatInt(owner, 10)
		}
		c.Redirect(http.StatusFound, back+"?dev=notfound")
		return
	}
	c.Redirect(http.StatusFound, "/admin/users/"+strconv.FormatInt(owner, 10))
}

// ── 成员 ──────────────────────────────────────────────

// passwordError 把 ?pwd= 上那个短码翻成一句话。
//
// 前三种和 JSON 接口共用同一批 uierr code，所以共用同一份语料。
// err 那种是内部错误，uierr 里没有它——那批 code 是按「用户的下一步」分的，
// 而这里用户没有下一步，只能重试或者去看日志。
func (s *Server) passwordError(c *gin.Context, code string) string {
	switch code {
	case "wrong":
		return s.userText(c, uierr.PasswordWrong)
	case "mismatch":
		return s.userText(c, uierr.PasswordMismatch)
	case "weak":
		return s.userText(c, uierr.PasswordWeak)
	case "err":
		return web.T(reqLang(c)).Admin.Common.PwChangeFail
	}
	return ""
}

// actionError 把一次失败的写操作画成一条红条。
//
// 这些动作是 POST 完 302 走的，失败信息只能挂在 query 上带回来——
// 和设置页的 ?pwd=wrong 是同一套做法。没有这一条，「点了没反应」就只是
// 从「悄悄什么都没做」变成「悄悄跳回列表」，对用户没有任何区别。
func (s *Server) actionError(c *gin.Context, param, notFoundCode string) string {
	var msg string
	switch c.Query(param) {
	case "notfound":
		msg = s.userText(c, notFoundCode)
	case "error":
		// 内部错误不进 uierr：那批 code 是按「用户的下一步」分的，
		// 而这里用户没有下一步，只能重试或者去看日志。
		msg = web.T(reqLang(c)).Admin.Common.ActionFail
	default:
		return ""
	}
	return `<div class="err">` + web.E(msg) + `</div>`
}

func (s *Server) adminUsers(c *gin.Context) {
	v := newAdminView(c)
	q := strings.TrimSpace(c.Query("q"))
	page, _ := strconv.Atoi(c.Query("page"))
	if page < 1 {
		page = 1
	}
	const pageSize = 50

	// 公共实例上成员会有几百上千，一次全渲染出来页面会很重，
	// 而且没有搜索的话找一个人只能靠浏览器的 Ctrl+F。
	rows, total, err := s.admin().Members(service.MemberFilter{
		Query: q, Limit: pageSize, Offset: (page - 1) * pageSize,
	})
	if err != nil {
		log.Error("admin: member list failed", log.Any("error", err.Error()))
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<div class="ph"><div><h1>%s</h1><div class="sub">%s</div></div>`+
		`<div class="spacer"></div><a class="btn" href="/admin/pair">%s%s</a></div>`,
		web.E(v.t.Nav.Users), web.E(fmt.Sprintf(v.t.Users.Sub, total)),
		web.Svg("qr", 15), web.E(v.t.Nav.Pair))
	fmt.Fprintf(&b, `<form class="tools" method="get"><input name="q" placeholder="%s" value="%s" style="min-width:220px">`+
		`<button class="btn" type="submit">%s</button>%s</form>`,
		web.E(v.t.Users.SearchPh), web.E(q), web.E(v.t.Common.Search),
		map[bool]string{
			true:  `<a class="btn ghost" href="/admin/users">` + web.E(v.t.Users.Clear) + `</a>`,
			false: "",
		}[q != ""])
	b.WriteString(s.actionError(c, "member", uierr.MemberNotFound))

	if len(rows) == 0 {
		b.WriteString(web.Card("", "", web.Empty(map[bool]string{
			true:  v.t.Users.EmptyQuery,
			false: v.t.Users.EmptyAll,
		}[q != ""])))
		s.shell(c, "users", []web.Crumb{web.C(v.t.Nav.Users)}, b.String())
		return
	}

	var t strings.Builder
	fmt.Fprintf(&t, `<table><thead><tr><th style="padding-left:16px">%s</th><th>%s</th>`+
		`<th>%s</th><th>%s</th><th>%s</th><th>%s</th><th>%s</th></tr></thead><tbody>`,
		web.E(v.t.Common.Name), web.E(v.t.Common.Role), web.E(v.t.Common.Device),
		web.E(v.t.Common.Channel), web.E(v.t.Common.Messages),
		web.E(v.t.Users.ColLogin), web.E(v.t.Users.ColQuota))
	for _, r := range rows {
		role := `<span class="badge muted">` + web.E(v.t.Common.RoleMember) + `</span>`
		if r.Role == models.RoleAdmin {
			role = `<span class="badge info">` + web.E(v.t.Common.RoleAdmin) + `</span>`
		}
		fmt.Fprintf(&t, `<tr><td style="padding-left:16px;font-weight:500">`+
			`<a href="/admin/users/%d">%s</a></td><td>%s</td>`+
			`<td class="num">%d</td><td class="num">%d</td><td class="num">%d</td>`+
			`<td class="dim">%s</td><td style="text-align:right;padding-right:16px">%s</td></tr>`,
			r.Id, web.E(r.Name), role, r.Devices, r.Channels,
			r.Messages, v.ago(r.LastLoginAt),
			unlimitedToggle(v, strconv.FormatInt(r.Id, 10), r.Unlimited != 0))
	}
	t.WriteString(`</tbody></table>`)
	if int64(page*pageSize) < total {
		fmt.Fprintf(&t, `<div class="pager"><a class="btn outline sm" href="?q=%s&page=%d">%s%s</a>`+
			`<span class="dim">%s</span></div>`,
			web.E(q), page+1, web.E(v.t.Common.NextPage), web.Svg("chev", 14),
			web.E(pagerInfo(v, page, total)))
	}
	b.WriteString(web.Card("", "", t.String()))
	// 这一段带标签（<b>、<code>），语料里就是 HTML，所以不过 E()。
	// 它是我们自己写的文案，不是外部输入。
	b.WriteString(`<div class="note">` + web.Svg("alert", 14) + `<p>` + v.t.Users.Note + `</p></div>`)
	s.shell(c, "users", []web.Crumb{web.C(v.t.Nav.Users)}, b.String())
}

// pagerInfo 「第 N 页 · 共 M 人」。两个数字，所以不走 Plural，
// 自己按 M 选那一条。
func pagerInfo(v adminView, page int, total int64) string {
	f := v.t.Users.PagerInfo
	if total == 1 {
		f = v.t.Users.PagerOne
	}
	return fmt.Sprintf(f, page, total)
}

// unlimitedToggle 单个用户的配额豁免。
// 运营者自己的账号不该被自己设的限额卡住，而他用的是同一套接口。
func unlimitedToggle(v adminView, id string, on bool) string {
	label, style := v.t.Users.Unlimited, "ghost"
	if on {
		label, style = v.t.Users.UnlimitedOn, "outline"
	}
	return fmt.Sprintf(`<form class="inline" method="post" action="/admin/users/%s/unlimited">`+
		`<input type="hidden" name="on" value="%d">`+
		`<button class="btn %s sm" type="submit">%s</button></form>`,
		web.E(id), boolInt(!on), style, web.E(label))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (s *Server) adminUserUnlimited(c *gin.Context) {
	on := 0
	if c.PostForm("on") == "1" {
		on = 1
	}
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := s.admin().SetUnlimited(id, on == 1); err != nil {
		if e, ok := uierr.As(err); ok && e.Code == uierr.MemberNotFound {
			c.Redirect(http.StatusFound, "/admin/users?member=notfound")
			return
		}
		log.Error("admin: set unlimited failed", log.Any("error", err.Error()))
		c.Redirect(http.StatusFound, "/admin/users?member=error")
		return
	}
	c.Redirect(http.StatusFound, "/admin/users")
}

// ── 设置 ──────────────────────────────────────────────

func (s *Server) adminSettings(c *gin.Context) {
	v := newAdminView(c)
	uid := middleware.UserID(c)
	u := s.quota().Usage(uid)
	st := s.Settings
	currentName := ""
	if me := middleware.Admin(c); me != nil {
		currentName = me.Username
	}

	// 改过的项打一个点，卡片底部统一说明它的含义。
	// 每行挂一个「后台已设置」徽标是纯噪音——重复六次，而这个信息很少被需要。
	mark := func(key string) string {
		if st.FromConfig(key) {
			return ""
		}
		return `<span class="dot-set" title="` + web.E(v.t.Settings.DotTitle) + `"></span>`
	}
	row := func(label, hint, ctl, key string) string {
		return fmt.Sprintf(`<div class="set-row"><div class="lab"><b>%s</b><span>%s</span></div>`+
			`<div class="ctl">%s%s</div></div>`, web.E(label), web.E(hint), mark(key), ctl)
	}
	num := func(name string, v int) string {
		return fmt.Sprintf(`<input type="number" min="0" name="%s" value="%d">`, name, v)
	}

	// tab 的值是 key 不是文案。中文当路由状态的话，后台文案一翻译
	// 这条链接就再也点不亮了，而那是第二步（前端 i18n）必然会撞上的。
	tab := c.Query("tab")
	if tab != "server" {
		tab = "public"
	}

	var b strings.Builder
	b.WriteString(`<div class="ph"><div><h1>` + web.E(v.t.Nav.Settings) + `</h1>` +
		`<div class="sub">` + web.E(v.t.Settings.Sub) + `</div></div></div>`)
	b.WriteString(web.Tabs(tab, [][3]string{
		{"public", v.t.Settings.TabPublic, "/admin/settings"},
		{"server", v.t.Settings.TabServer, "/admin/settings?tab=server"},
	}))
	if c.Query("saved") == "1" {
		b.WriteString(`<div class="narrow"><div class="note">` + web.Svg("zap", 14) +
			`<p>` + web.E(v.t.Settings.Saved) + `</p></div></div>`)
	}
	// 改密码失败的三种情形。成功那一条不在这里：密码一改会话就失效了，
	// 人已经被带到登录页，提示也留在那边。
	//
	// 句子走语料，不在这里再写一份：同样这三句 JSON 接口也要回，
	// 硬编码两份的结果是以后只改到其中一处，两个界面对同一次失败说两句话。
	if msg := s.passwordError(c, c.Query("pwd")); msg != "" {
		b.WriteString(`<div class="narrow"><div class="err">` + web.E(msg) + `</div></div>`)
	}
	if tab == "public" {
		b.WriteString(`<form method="post" action="/admin/settings" class="narrow" ` +
			`style="display:flex;flex-direction:column;gap:16px">`)

		checked := ""
		if st.PublicEnabled() {
			checked = " checked"
		}
		sw := fmt.Sprintf(`<label class="sw"><input type="checkbox" name="public_enabled" value="1"%s><i></i></label>`, checked)
		open := row(v.t.Settings.PublicMode, v.t.Settings.PublicHint, sw, "public_enabled") +
			row(v.t.Settings.RegPerHour, v.t.Settings.RegHint,
				num("register_per_hour", st.RegisterPerHour()), "register_per_hour") +
			fmt.Sprintf(`<div class="set-row"><div class="lab"><b>%s</b><span>%s</span></div>`+
				`<div class="ctl">%s<input type="text" name="site_name" value="%s"></div></div>`,
				web.E(v.t.Settings.SiteName), web.E(v.t.Settings.SiteHint),
				mark("site_name"), web.E(st.SiteName()))
		b.WriteString(web.Card(v.t.Settings.TabPublic, "", open))

		quota := row(v.t.Settings.MaxChannels, v.t.Settings.MaxChHint,
			num("max_channels", st.MaxChannels()), "max_channels") +
			row(v.t.Settings.MaxPerDay, v.t.Settings.MaxDayHint,
				num("max_per_day", st.MaxPerDay()), "max_per_day") +
			row(v.t.Settings.Retention, v.t.Settings.RetHint,
				num("retention_days", st.RetentionDays()), "retention_days")
		// 语料里带 <b> 和一个链接，所以不过 E()——它是我们自己写的文案。
		quota += `<div class="card-foot"><div class="dim" style="font-size:12.5px;flex:1">` +
			v.t.Settings.QuotaNote + `</div></div>`
		b.WriteString(web.Card(v.t.Settings.CardQuota, "", quota))

		dot := `<span class="dot-set" style="display:inline-block;vertical-align:middle"></span>`
		b.WriteString(`<div style="display:flex;align-items:center;gap:12px">` +
			`<button class="btn" type="submit">` + web.E(v.t.Common.Save) + `</button>` +
			`<span class="dim" style="font-size:12.5px">` +
			fmt.Sprintf(web.E(v.t.Settings.SaveHint), dot) + `</span></div>`)
		b.WriteString(`</form>`)
	} else {
		// ── 服务器信息（只读） ────────────────────────────────
		mode := `<span class="badge muted">` + web.E(v.t.Settings.ModeSelf) + `</span>`
		if st.PublicEnabled() {
			mode = `<span class="badge info">` + web.E(v.t.Settings.ModePublic) + `</span>`
		}
		info := row(v.t.Settings.InfoName, "", web.E(s.Name), "") +
			row(v.t.Settings.InfoURL, "", `<span class="mono">`+web.E(s.ExternalURL)+`</span>`, "") +
			row(v.t.Settings.InfoVersion, "", `<span class="mono">`+web.E(s.Version)+`</span>`, "") +
			row(v.t.Settings.InfoMode, "", mode, "")
		b.WriteString(`<div class="narrow">` + web.Card(v.t.Settings.CardServer, "", info) + `</div>`)

		kv := func(k, v string) string {
			return fmt.Sprintf(`<div><div class="k">%s</div><div class="v num">%s</div></div>`,
				web.E(k), web.E(v))
		}
		usage := `<div class="kv-grid">` +
			kv(v.t.Settings.UsageChannels, fmt.Sprint(u.Channels)) +
			kv(v.t.Settings.UsageToday, fmt.Sprint(u.Today)) +
			kv(v.t.Settings.UsageTotal, fmt.Sprint(u.Messages)) +
			kv(v.t.Settings.UsageFiles, bytesize.Decimal(u.FileBytes)) +
			`</div>`
		b.WriteString(`<div class="narrow">` + web.Card(v.t.Settings.CardUsage, "", usage) + `</div>`)

		// 改密码必须能在这里做完。第一个管理员是服务器首次启动时自动建的，
		// 密码随机且只打印那一次——把「改掉它」放在命令行里，等于要求每个自建的人
		// 都会进容器敲命令，而他们当中很多人不会。
		pwRow := func(label, name, autocomplete string) string {
			return fmt.Sprintf(`<div class="set-row"><div class="lab"><b>%s</b></div>`+
				`<div class="ctl"><input type="password" name="%s" autocomplete="%s" required></div></div>`,
				web.E(label), name, autocomplete)
		}
		pw := pwRow(v.t.Settings.PwCurrent, "current", "current-password") +
			pwRow(v.t.Settings.PwNew, "new", "new-password") +
			pwRow(v.t.Settings.PwConfirm, "confirm", "new-password") +
			`<div class="card-foot"><div class="dim" style="font-size:12.5px;flex:1">` +
			web.E(v.t.Settings.PwNote) + `</div></div>`
		b.WriteString(`<div class="narrow"><form method="post" action="/admin/account/password" ` +
			`style="display:flex;flex-direction:column;gap:16px">` +
			web.Card(fmt.Sprintf(v.t.Settings.PwCard, currentName), "", pw) +
			`<div><button class="btn" type="submit">` + web.E(v.t.Settings.PwSubmit) +
			`</button></div></form></div>`)

		// 命令本身不翻译，只翻译后面那句它做什么。对齐用的是等宽字体里的空格，
		// 所以宽度按各语言自己的长度算，不能写死。
		cli := []struct{ cmd, note string }{
			{"knockbox user add &lt;name&gt;", v.t.Settings.CLIAdd},
			{"knockbox user passwd &lt;name&gt;", v.t.Settings.CLIPasswd},
			{"knockbox user list", v.t.Settings.CLIList},
			{"knockbox user disable &lt;name&gt;", v.t.Settings.CLIDisable},
		}
		wide := 0
		for _, it := range cli {
			if n := len([]rune(it.cmd)); n > wide {
				wide = n
			}
		}
		var cmds strings.Builder
		for _, it := range cli {
			cmds.WriteString(it.cmd + strings.Repeat(" ", wide-len([]rune(it.cmd))+4) +
				web.E(it.note) + "\n")
		}
		b.WriteString(`<div class="narrow">` + web.Card(v.t.Settings.CardCLI, "",
			`<div class="card-b" style="padding:16px">`+
				`<p style="margin:0 0 10px;line-height:1.7">`+web.E(v.t.Settings.CLIIntro)+`</p>`+
				`<pre class="body">`+strings.TrimRight(cmds.String(), "\n")+`</pre></div>`) + `</div>`)
	}
	s.shell(c, "settings", []web.Crumb{web.C(v.t.Nav.Settings)}, b.String())
}

// adminPasswordSave 改当前登录账号的密码。
//
// 只改自己的：别人的密码要么由本人在这里改，要么由服务器上的人用
// `knockbox user passwd` 重设。后台不做「管理员替别人设密码」这件事，
// 它意味着有人知道另一个人的密码。
func (s *Server) adminPasswordSave(c *gin.Context) {
	back := func(code string) {
		c.Redirect(http.StatusFound, "/admin/settings?tab=server&pwd="+code)
	}
	me := middleware.Admin(c)
	if me == nil {
		back("err")
		return
	}
	acc := service.NewAccount(s.DAO)
	// 先验当前密码：会话 cookie 被偷走的人不该顺手就能把密码换掉。
	if _, err := acc.Verify(me.Username, c.PostForm("current")); err != nil {
		back("wrong")
		return
	}
	pw := c.PostForm("new")
	if pw != c.PostForm("confirm") {
		back("mismatch")
		return
	}
	if err := service.ValidatePassword(pw); err != nil {
		back("weak")
		return
	}
	if err := acc.SetPassword(me.Username, pw); err != nil {
		log.Error("changing the password failed", log.Any("user", me.Username), log.Any("error", err.Error()))
		back("err")
		return
	}
	// SetPassword 清掉了这个账号的全部会话，当前这条也在内。
	// 把 cookie 一并清掉，否则浏览器会带着一个已经没用的 cookie 再来一次。
	s.setSessionCookie(c, "", -1)
	c.Redirect(http.StatusFound, "/login?changed=1")
}

func (s *Server) adminSettingsSave(c *gin.Context) {
	vals := map[string]string{
		"public_enabled": "0",
		"site_name":      c.PostForm("site_name"),
	}
	if c.PostForm("public_enabled") == "1" {
		vals["public_enabled"] = "1"
	}
	// 数字字段留空就当没改：把空串存成 0 会悄悄变成「不限」，
	// 而用户以为自己什么都没做。
	for _, k := range []string{"max_channels", "max_per_day", "retention_days", "register_per_hour"} {
		if v := strings.TrimSpace(c.PostForm(k)); v != "" {
			if _, err := strconv.Atoi(v); err == nil {
				vals[k] = v
			}
		}
	}
	if err := s.Settings.Save(vals); err != nil {
		v := newAdminView(c)
		s.shell(c, "settings", []web.Crumb{web.C(v.t.Nav.Settings)},
			web.Card("", "", web.Empty(fmt.Sprintf(v.t.Settings.SaveFailed, err.Error()))))
		return
	}
	c.Redirect(http.StatusFound, "/admin/settings?saved=1")
}
