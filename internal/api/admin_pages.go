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
	b.WriteString(`<div class="ph"><div><h1>搜索消息</h1>` +
		`<div class="sub">这台服务器上的全部消息</div></div></div>`)
	b.WriteString(s.searchForm(q, userFilter, chFilter))

	if len(rows) == 0 {
		b.WriteString(web.Card("", "", web.Empty(emptyHint(q, chFilter))))
	} else {
		var t strings.Builder
		t.WriteString(`<table><thead><tr><th style="padding-left:16px">时间</th><th>成员</th><th>频道</th>` +
			`<th>类型</th><th>标题</th><th>推送</th><th></th></tr></thead><tbody>`)
		var last int64
		for _, r := range rows {
			last = r.Id
			readMark := ""
			if r.ReadAt == 0 {
				readMark = `<span class="badge info"><i></i>未读</span>`
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
				pushBadge(r.PushOK, r.PushTotal), readMark)
		}
		t.WriteString(`</tbody></table>`)
		if more {
			fmt.Fprintf(&t, `<div class="pager"><a class="btn outline sm" `+
				`href="?q=%s&user=%s&channel=%s&before=%d">更早的%s</a></div>`,
				web.E(q), web.E(userFilter), web.E(chFilter), last, web.Svg("chev", 14))
		}
		b.WriteString(web.Card("", "", t.String()))
	}
	s.shell(c, "messages", []web.Crumb{web.C("搜索消息")}, b.String())
}

func emptyHint(q, ch string) string {
	if q != "" {
		return "没有匹配的消息。换个词试试，搜索会同时看标题、摘要和正文。"
	}
	if ch != "" {
		return "这个频道还没有消息。"
	}
	return "还没有消息。往任意一个频道 curl 一条试试。"
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
func (s *Server) userPicker(name, current, placeholder string) string {
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
	fmt.Fprintf(&b, `<span class="hint">%d 人</span></span>`, len(rows))
	return b.String()
}

// searchForm 成员 → 频道 的联动筛选。
//
// 频道全部渲染出来、按成员在前端过滤，而不是选完成员再往服务端跑一趟：
// 一台服务器上的频道总数是几十量级，一次带出来远比一次往返便宜，
// 而且切成员时列表立刻就变，不闪。
func (s *Server) searchForm(q, userFilter, chFilter string) string {
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
	fmt.Fprintf(&b, `<input name="q" placeholder="搜标题、正文" value="%s" style="min-width:220px">`, web.E(q))

	b.WriteString(s.userPicker("user", userFilter, "全部成员"))

	b.WriteString(`<select name="channel" id="cSel"><option value="">全部频道</option>`)
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

	b.WriteString(`<button class="btn" type="submit">搜索</button>`)
	if q != "" || chFilter != "" || userFilter != "" {
		b.WriteString(`<a class="btn ghost" href="/admin/messages">清空条件</a>`)
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
	msg, channel, err := s.admin().Message(c.Param("uid"))
	if err != nil {
		s.shell(c, "messages", []web.Crumb{web.C("消息"), web.C("未找到")},
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
	b.WriteString(web.Card("正文", "", `<div class="card-b"><pre class="body">`+web.E(body)+`</pre></div>`))
	if m.Extra != "" {
		b.WriteString(web.Card("附加信息", "", `<div class="card-b"><pre class="body">`+web.E(m.Extra)+`</pre></div>`))
	}
	b.WriteString(web.Card("推送摘要（进 APNs payload 的就是它）", "",
		`<div class="card-b"><pre class="body">`+web.E(m.Summary)+`</pre></div>`))
	if m.Replyable() {
		b.WriteString(web.Card("回复", "", s.replyPanel(&m)))
	}
	b.WriteString(web.Card("投递记录", "", s.pushLogTable(m.Id)))
	s.shell(c, "messages", []web.Crumb{web.C("搜索消息", "/admin/messages"), web.C(trunc(title, 24))}, b.String())
}

// replyPanel 一条可回复消息的回复状态与回调投递结果。
//
// 这一屏是【回调失败时唯一的线索】：用户那边显示「已回复」，发送方什么都没收到，
// 两边都不会自己发现这件事。没有这一屏，排查只能去翻日志。
func (s *Server) replyPanel(m *models.Message) string {
	var b strings.Builder
	b.WriteString(`<div class="card-b">`)

	now := time.Now().Unix()
	switch {
	case m.Replied():
		fmt.Fprintf(&b, `<p><span class="badge ok"><i></i>已回复</span> <b>%s</b> `+
			`<span class="dim">%s</span></p>`,
			web.E(m.Reply), web.E(time.Unix(m.RepliedAt, 0).Format("2006-01-02 15:04:05")))
	case m.ReplyExpired(now):
		fmt.Fprintf(&b, `<p><span class="badge warn"><i></i>时限已过</span> `+
			`<span class="dim">截止 %s，没有回复</span></p>`,
			web.E(time.Unix(m.ReplyUntil, 0).Format("2006-01-02 15:04:05")))
	case m.ReplyUntil > 0:
		fmt.Fprintf(&b, `<p><span class="badge muted"><i></i>等待回复</span> `+
			`<span class="dim">截止 %s</span></p>`,
			web.E(time.Unix(m.ReplyUntil, 0).Format("2006-01-02 15:04:05")))
	default:
		b.WriteString(`<p><span class="badge muted"><i></i>等待回复</span> ` +
			`<span class="dim">没有设时限</span></p>`)
	}
	// 回调地址只在后台显示。它不下发给任何客户端，但服务器的主人排障时要看得到。
	fmt.Fprintf(&b, `<p class="dim">回调地址 <span class="mono">%s</span></p>`, web.E(m.ReplyWebhook))
	b.WriteString(`</div>`)

	rows, err := s.admin().ReplyHooks(m.Id)
	if err != nil {
		log.Error("admin: reply hooks failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		if m.Replied() {
			// 回复和入队在同一个事务里，所以这种情况说明数据被写坏了。
			b.WriteString(web.Empty("已经回复了，却没有回调记录。这不该发生，请检查服务端日志。"))
		}
		return b.String()
	}
	b.WriteString(`<table><thead><tr><th style="padding-left:16px">回调</th><th>尝试</th>` +
		`<th>HTTP</th><th>原因</th><th>时间</th></tr></thead><tbody>`)
	for _, r := range rows {
		code := "—"
		if r.StatusCode != 0 {
			code = strconv.Itoa(r.StatusCode)
		}
		fmt.Fprintf(&b, `<tr><td style="padding-left:16px">%s</td><td class="num">%d</td>`+
			`<td class="num dim">%s</td><td class="dim">%s</td><td class="dim num">%s</td></tr>`,
			hookBadge(r.Status), r.Attempt,
			web.E(code), web.E(trunc(r.Error, 60)), clock(r.Utime))
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func hookBadge(status int) string {
	switch status {
	case 1:
		return `<span class="badge ok"><i></i>已送达</span>`
	case 2:
		return `<span class="badge warn"><i></i>重试中</span>`
	case 3:
		// 「已放弃」要显眼：发送方永远收不到这个答案了，而它自己不会知道。
		return `<span class="badge err"><i></i>已放弃</span>`
	}
	return `<span class="badge muted"><i></i>排队中</span>`
}

func (s *Server) pushLogTable(msgID int64) string {
	rows, err := s.admin().PushLog(msgID)
	if err != nil {
		log.Error("admin: push log failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		return web.Empty("没有投递记录。频道静音时消息照常存档，但不会排推送。")
	}
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th style="padding-left:16px">设备</th><th>环境</th>` +
		`<th>状态</th><th>尝试</th><th>apns-id</th><th>时间</th></tr></thead><tbody>`)
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><td style="padding-left:16px">%s</td><td class="dim">%s</td>`+
			`<td>%s</td><td class="num">%d</td><td class="mono dim">%s</td><td class="dim num">%s</td></tr>`,
			web.E(r.Device), web.E(r.APNsEnv), statusBadge(r.Status, r.HTTPStatus, r.Reason),
			r.Attempts, web.E(trunc(r.APNsID, 12)), clock(r.Utime))
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func statusBadge(status, httpStatus int, reason string) string {
	switch status {
	case 1:
		return `<span class="badge ok"><i></i>已送达 ` + web.E(strconv.Itoa(httpStatus)) + `</span>`
	case 3:
		return `<span class="badge err"><i></i>` + web.E(firstNonEmpty(reason, "失败")) + `</span>`
	case 2:
		return `<span class="badge warn"><i></i>重试中</span>`
	}
	return `<span class="badge muted"><i></i>排队中</span>`
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
		msg = "操作没有成功。服务器日志里有原因。"
	default:
		return ""
	}
	return `<div class="err">` + web.E(msg) + `</div>`
}

func (s *Server) adminUsers(c *gin.Context) {
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
	fmt.Fprintf(&b, `<div class="ph"><div><h1>成员</h1><div class="sub">这台服务器上的收件身份 · 共 %d 人</div></div>`+
		`<div class="spacer"></div><a class="btn" href="/admin/pair">%s配对设备</a></div>`, total, web.Svg("qr", 15))
	fmt.Fprintf(&b, `<form class="tools" method="get"><input name="q" placeholder="搜成员名字" value="%s" style="min-width:220px">`+
		`<button class="btn" type="submit">搜索</button>%s</form>`,
		web.E(q), map[bool]string{true: `<a class="btn ghost" href="/admin/users">清空</a>`, false: ""}[q != ""])
	b.WriteString(s.actionError(c, "member", uierr.MemberNotFound))

	if len(rows) == 0 {
		b.WriteString(web.Card("", "", web.Empty(map[bool]string{
			true:  "没有叫这个名字的成员。",
			false: "还没有成员。点右上角配对一台设备就会建出第一个。",
		}[q != ""])))
		s.shell(c, "users", []web.Crumb{web.C("成员")}, b.String())
		return
	}

	var t strings.Builder
	t.WriteString(`<table><thead><tr><th style="padding-left:16px">名字</th><th>角色</th>` +
		`<th>设备</th><th>频道</th><th>消息</th><th>最后登录</th><th>配额</th></tr></thead><tbody>`)
	for _, r := range rows {
		role := `<span class="badge muted">收件人</span>`
		if r.Role == models.RoleAdmin {
			role = `<span class="badge info">管理员</span>`
		}
		fmt.Fprintf(&t, `<tr><td style="padding-left:16px;font-weight:500">`+
			`<a href="/admin/users/%d">%s</a></td><td>%s</td>`+
			`<td class="num">%d</td><td class="num">%d</td><td class="num">%d</td>`+
			`<td class="dim">%s</td><td style="text-align:right;padding-right:16px">%s</td></tr>`,
			r.Id, web.E(r.Name), role, r.Devices, r.Channels,
			r.Messages, ago(r.LastLoginAt),
			unlimitedToggle(strconv.FormatInt(r.Id, 10), r.Unlimited != 0))
	}
	t.WriteString(`</tbody></table>`)
	if int64(page*pageSize) < total {
		fmt.Fprintf(&t, `<div class="pager"><a class="btn outline sm" href="?q=%s&page=%d">下一页%s</a>`+
			`<span class="dim">第 %d 页 · 共 %d 人</span></div>`,
			web.E(q), page+1, web.Svg("chev", 14), page, total)
	}
	b.WriteString(web.Card("", "", t.String()))
	b.WriteString(`<div class="note">` + web.Svg("alert", 14) +
		`<p>扫码接入的人是<b>收件人</b>：没有用户名密码，登录不了这个后台，身份就是设备上那把凭据。` +
		`第一个管理员在服务器首次启动时自动建好，再加人用命令行 —— <code class="code">knockbox user add &lt;名字&gt;</code>。</p></div>`)
	s.shell(c, "users", []web.Crumb{web.C("成员")}, b.String())
}

// unlimitedToggle 单个用户的配额豁免。
// 运营者自己的账号不该被自己设的限额卡住，而他用的是同一套接口。
func unlimitedToggle(id string, on bool) string {
	label, style := "豁免配额", "ghost"
	if on {
		label, style = "已豁免 · 点击恢复", "outline"
	}
	return fmt.Sprintf(`<form class="inline" method="post" action="/admin/users/%s/unlimited">`+
		`<input type="hidden" name="on" value="%d">`+
		`<button class="btn %s sm" type="submit">%s</button></form>`,
		web.E(id), boolInt(!on), style, label)
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
		return `<span class="dot-set" title="已在后台修改"></span>`
	}
	row := func(label, hint, ctl, key string) string {
		return fmt.Sprintf(`<div class="set-row"><div class="lab"><b>%s</b><span>%s</span></div>`+
			`<div class="ctl">%s%s</div></div>`, web.E(label), web.E(hint), mark(key), ctl)
	}
	num := func(name string, v int) string {
		return fmt.Sprintf(`<input type="number" min="0" name="%s" value="%d">`, name, v)
	}

	tab := c.Query("tab")
	if tab != "服务器" {
		tab = "对外开放"
	}

	var b strings.Builder
	b.WriteString(`<div class="ph"><div><h1>设置</h1>` +
		`<div class="sub">改完立刻生效，不用重启</div></div></div>`)
	b.WriteString(web.Tabs(tab, [][2]string{
		{"对外开放", "/admin/settings"},
		{"服务器", "/admin/settings?tab=服务器"},
	}))
	if c.Query("saved") == "1" {
		b.WriteString(`<div class="narrow"><div class="note">` + web.Svg("zap", 14) +
			`<p>已保存。</p></div></div>`)
	}
	// 改密码失败的三种情形。成功那一条不在这里：密码一改会话就失效了，
	// 人已经被带到登录页，提示也留在那边。
	if msg := map[string]string{
		"wrong":    "当前密码不对，密码没有改动。",
		"mismatch": "两次输入的新密码不一致，密码没有改动。",
		"weak":     "新密码至少 8 位，密码没有改动。",
		"err":      "改密码没有成功，密码没有改动。服务器日志里有原因。",
	}[c.Query("pwd")]; msg != "" {
		b.WriteString(`<div class="narrow"><div class="err">` + web.E(msg) + `</div></div>`)
	}
	if tab == "对外开放" {
		b.WriteString(`<form method="post" action="/admin/settings" class="narrow" ` +
			`style="display:flex;flex-direction:column;gap:16px">`)

		checked := ""
		if st.PublicEnabled() {
			checked = " checked"
		}
		sw := fmt.Sprintf(`<label class="sw"><input type="checkbox" name="public_enabled" value="1"%s><i></i></label>`, checked)
		open := row("公共模式", "任何人打开首页都能扫码接入", sw, "public_enabled") +
			row("每 IP 每小时注册", "挡批量注册", num("register_per_hour", st.RegisterPerHour()), "register_per_hour") +
			fmt.Sprintf(`<div class="set-row"><div class="lab"><b>站点名</b><span>显示在接入页上</span></div>`+
				`<div class="ctl">%s<input type="text" name="site_name" value="%s"></div></div>`,
				mark("site_name"), web.E(st.SiteName()))
		b.WriteString(web.Card("对外开放", "", open))

		quota := row("频道数上限", "每个成员，0 = 不限", num("max_channels", st.MaxChannels()), "max_channels") +
			row("每 24 小时消息", "滚动窗口，不是按自然日清零", num("max_per_day", st.MaxPerDay()), "max_per_day") +
			row("消息保留天数", "超期自动物理删除，0 = 永久", num("retention_days", st.RetentionDays()), "retention_days")
		quota += `<div class="card-foot"><div class="dim" style="font-size:12.5px;flex:1">` +
			`<b>配额只在公共模式下生效。</b>自建模式下自己的数据不该被限；` +
			`单个成员还能在<a href="/admin/users">成员</a>页里单独豁免。</div></div>`
		b.WriteString(web.Card("每个成员的配额", "", quota))

		b.WriteString(`<div style="display:flex;align-items:center;gap:12px">` +
			`<button class="btn" type="submit">保存</button>` +
			`<span class="dim" style="font-size:12.5px">带 <span class="dot-set" ` +
			`style="display:inline-block;vertical-align:middle"></span> 的项已在后台改过，` +
			`其余用 config.toml 里的值。</span></div>`)
		b.WriteString(`</form>`)
	} else {
		// ── 服务器信息（只读） ────────────────────────────────
		mode := `<span class="badge muted">自建模式</span>`
		if st.PublicEnabled() {
			mode = `<span class="badge info">公共模式</span>`
		}
		info := row("名称", "", web.E(s.Name), "") +
			row("对外地址", "", `<span class="mono">`+web.E(s.ExternalURL)+`</span>`, "") +
			row("版本", "", `<span class="mono">`+web.E(s.Version)+`</span>`, "") +
			row("运行模式", "", mode, "")
		b.WriteString(`<div class="narrow">` + web.Card("服务器", "", info) + `</div>`)

		kv := func(k, v string) string {
			return fmt.Sprintf(`<div><div class="k">%s</div><div class="v num">%s</div></div>`,
				web.E(k), web.E(v))
		}
		usage := `<div class="kv-grid">` +
			kv("频道数", fmt.Sprint(u.Channels)) +
			kv("24 小时消息", fmt.Sprint(u.Today)) +
			kv("消息总数", fmt.Sprint(u.Messages)) +
			kv("附件占用", bytesize.Decimal(u.FileBytes)) +
			`</div>`
		b.WriteString(`<div class="narrow">` + web.Card("你自己的用量", "", usage) + `</div>`)

		// 改密码必须能在这里做完。第一个管理员是服务器首次启动时自动建的，
		// 密码随机且只打印那一次——把「改掉它」放在命令行里，等于要求每个自建的人
		// 都会进容器敲命令，而他们当中很多人不会。
		pwRow := func(label, name, autocomplete string) string {
			return fmt.Sprintf(`<div class="set-row"><div class="lab"><b>%s</b></div>`+
				`<div class="ctl"><input type="password" name="%s" autocomplete="%s" required></div></div>`,
				web.E(label), name, autocomplete)
		}
		pw := pwRow("当前密码", "current", "current-password") +
			pwRow("新密码", "new", "new-password") +
			pwRow("再输一次", "confirm", "new-password") +
			`<div class="card-foot"><div class="dim" style="font-size:12.5px;flex:1">` +
			`至少 8 位。改完这个账号的登录状态会全部失效，要用新密码重新登录一次。</div></div>`
		b.WriteString(`<div class="narrow"><form method="post" action="/admin/account/password" ` +
			`style="display:flex;flex-direction:column;gap:16px">` +
			web.Card("修改密码（"+web.E(currentName)+"）", "", pw) +
			`<div><button class="btn" type="submit">修改密码</button></div></form></div>`)

		b.WriteString(`<div class="narrow">` + web.Card("其他账号操作", "", `<div class="card-b" style="padding:16px">`+
			`<p style="margin:0 0 10px;line-height:1.7">新增、停用、重设别人的密码在命令行里做，`+
			`任何操作都不需要直接改数据库：</p>`+
			`<pre class="body">knockbox user add &lt;名字&gt;       新建
knockbox user passwd &lt;名字&gt;    改密码
knockbox user list             列出
knockbox user disable &lt;名字&gt;   停用并踢掉会话</pre></div>`) + `</div>`)
	}
	s.shell(c, "settings", []web.Crumb{web.C("设置")}, b.String())
}

// adminPasswordSave 改当前登录账号的密码。
//
// 只改自己的：别人的密码要么由本人在这里改，要么由服务器上的人用
// `knockbox user passwd` 重设。后台不做「管理员替别人设密码」这件事，
// 它意味着有人知道另一个人的密码。
func (s *Server) adminPasswordSave(c *gin.Context) {
	back := func(code string) {
		c.Redirect(http.StatusFound, "/admin/settings?tab=服务器&pwd="+code)
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
		s.shell(c, "settings", []web.Crumb{web.C("设置")},
			web.Card("", "", web.Empty("保存失败："+err.Error())))
		return
	}
	c.Redirect(http.StatusFound, "/admin/settings?saved=1")
}
