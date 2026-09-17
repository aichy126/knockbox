package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/library/bytesize"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// adminUser 成员详情：他的设备和频道都在这一页。
//
// 按归属关系组织而不是按表：设备和频道都属于成员，
// 把它们做成三个平级列表，等于让用户自己在脑子里做关联查询。
func (s *Server) adminUser(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	member, err := s.admin().Member(id)
	if err != nil {
		s.shell(c, "users", []web.Crumb{web.C("成员", "/admin/users"), web.C("未找到")},
			web.Card("", "", web.Empty(s.userText(c, uierr.MemberNotFound))))
		return
	}
	u := *member
	usage := s.quota().Usage(u.Id)

	var b strings.Builder
	role := `<span class="badge muted">收件人</span>`
	if u.Role == models.RoleAdmin {
		role = `<span class="badge info">管理员</span>`
	}
	b.WriteString(`<div class="ph"><div><h1>` + web.E(u.Name) + `</h1><div class="sub">` +
		`创建于 ` + time.Unix(u.Ctime, 0).Format("2006-01-02") + `</div></div>` +
		`<div class="spacer"></div>` + role + unlimitedToggle(fmt.Sprint(u.Id), u.Unlimited != 0) + `</div>`)

	// 用量
	lim := func(n, max int) string {
		if max <= 0 {
			return fmt.Sprintf("%d", n)
		}
		return fmt.Sprintf("%d / %d", n, max)
	}
	b.WriteString(`<div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(180px,1fr));gap:16px">`)
	b.WriteString(web.Stat("频道", lim(usage.Channels, usage.ChannelLimit), "muted", "当前"))
	b.WriteString(web.Stat("24 小时消息", lim(usage.Today, usage.DailyLimit), "muted", "滚动窗口"))
	b.WriteString(web.Stat("消息总数", fmt.Sprint(usage.Messages), "muted", "未删除的"))
	b.WriteString(web.Stat("附件占用", bytesize.Decimal(usage.FileBytes), "muted", "派生图，原图不留"))
	b.WriteString(`</div>`)

	b.WriteString(s.actionError(c, "dev", uierr.DeviceNotFound))
	b.WriteString(web.Card("频道", "", s.channelTable(u.Id)))
	b.WriteString(web.Card("设备", `<a class="btn ghost sm" href="/admin/pair">`+web.Svg("qr", 14)+`配对新设备</a>`,
		s.deviceTable(u.Id)))

	s.shell(c, "users", []web.Crumb{web.C("成员", "/admin/users"), web.C(u.Name)}, b.String())
}

func (s *Server) channelTable(uid int64) string {
	rows, err := s.admin().MemberChannels(uid)
	if err != nil {
		log.Error("admin: member channels failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		return web.Empty("还没有频道。频道在 app 里创建。")
	}
	now := time.Now().Unix()
	var t strings.Builder
	t.WriteString(`<table><thead><tr><th style="padding-left:16px">名字</th><th>消息</th><th>最近</th>` +
		`<th>铃声</th><th>打扰级别</th><th>状态</th></tr></thead><tbody>`)
	for _, r := range rows {
		state := `<span class="badge ok"><i></i>正常</span>`
		if r.Muted != 0 {
			state = `<span class="badge muted"><i></i>一直静音</span>`
		} else if r.MuteUntil > now {
			state = `<span class="badge warn"><i></i>静音至 ` + time.Unix(r.MuteUntil, 0).Format("15:04") + `</span>`
		}
		fmt.Fprintf(&t, `<tr><td style="padding-left:16px;font-weight:500">`+
			`<a href="/admin/channels/%s">%s</a></td>`+
			`<td class="num">%d</td><td class="dim num">%s</td><td class="dim">%s</td>`+
			`<td class="dim">%s</td><td>%s</td></tr>`,
			web.E(r.Id), web.E(channelName(r.Meta, r.Id)), r.Messages, ago(r.LastMsgAt),
			web.E(r.Sound), web.E(r.Level), state)
	}
	t.WriteString(`</tbody></table>`)
	return t.String()
}

func (s *Server) deviceTable(uid int64) string {
	rows, err := s.admin().MemberDevices(uid)
	if err != nil {
		log.Error("admin: member devices failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		return web.Empty("还没有设备。点右上角配对一台。")
	}
	var t strings.Builder
	t.WriteString(`<table><thead><tr><th style="padding-left:16px">设备</th><th>系统</th><th>app</th>` +
		`<th>APNs</th><th>同步到</th><th>最后活跃</th><th></th></tr></thead><tbody>`)
	for _, r := range rows {
		push := `<span class="badge err"><i></i>收不到推送</span>`
		if r.CanPush() {
			kind := "ok"
			if r.APNsEnv == "sandbox" {
				kind = "info"
			}
			push = fmt.Sprintf(`<span class="badge %s"><i></i>%s</span>`, kind, web.E(r.APNsEnv))
		}
		fmt.Fprintf(&t, `<tr><td style="padding-left:16px;font-weight:500">%s`+
			`<div class="dim" style="font-weight:400;font-size:12px">%s</div></td>`+
			`<td class="dim">%s %s</td><td class="dim">%s</td><td>%s</td>`+
			`<td class="dim num">%d</td><td class="dim">%s</td>`+
			`<td style="text-align:right;padding-right:16px">`+
			`<form class="inline" method="post" action="/admin/devices/%d/delete" `+
			`onsubmit="return confirm('注销这台设备？它将不再收到推送，需要重新配对。')">`+
			`<button class="btn ghost sm" type="submit">注销</button></form></td></tr>`,
			web.E(r.Name), web.E(r.Model), web.E(r.Platform), web.E(r.OSVersion),
			web.E(r.AppVersion), push, r.SyncRev, ago(r.LastSeenAt), r.Id)
	}
	t.WriteString(`</tbody></table>`)
	return t.String()
}

// adminChannel 频道详情。
//
// 直接展示消息内容，不做标题列表：app 端已经是展开的消息流，
// 后台若停在「列表 → 点进去看」，同一份内容就有了两套操作方式。
// 这里按时间正序排列、自动滚到底，与 app 保持一致。
func (s *Server) adminChannel(c *gin.Context) {
	channel, ownerUser, err := s.admin().Channel(c.Param("id"))
	if err != nil {
		s.shell(c, "users", []web.Crumb{web.C("成员", "/admin/users"), web.C("未找到")},
			web.Card("", "", web.Empty(s.userText(c, uierr.ChannelNotFound))))
		return
	}
	ch, owner := *channel, *ownerUser
	name := channelName(ch.Meta, ch.Id)

	var b strings.Builder
	// 操作都在右上角这一行：清空是频道级动作，和「发送说明」同级，
	// 不该被埋在页面最底下一个叫「危险操作」的卡片里——那个位置反而没人看见。
	b.WriteString(`<div class="ph"><div><h1>` + web.E(name) + `</h1><div class="sub">属于 ` +
		web.E(owner.Name) + ` · ` + web.E(ch.Sound) + ` · ` + web.E(ch.Level) + muteSuffix(&ch) +
		`</div></div><div class="spacer"></div>` +
		`<a class="btn outline" href="/s/` + web.E(ch.Token) + `">` + web.Svg("zap", 15) + `发送说明</a>` +
		// 频道属性平时就是一颗按钮，点开才弹出来——它不常用，不该一直占着一整块版面。
		// 用 <details> 而不是 JS：没有交互状态要管，页面刷新也不会错乱。
		`<details class="pop"><summary class="btn outline">` + web.Svg("dash", 15) +
		`频道属性</summary><div class="pop-body">` + s.channelProps(&ch) + `</div></details>` +
		`<form class="inline" method="post" action="/admin/channels/` + web.E(ch.Id) + `/purge" ` +
		`onsubmit="return confirm('清空这个频道的全部消息？物理删除，无法恢复。')">` +
		`<button class="btn outline" type="submit" style="color:var(--danger)">` +
		web.Svg("trash", 15) + `清空消息</button></form></div>`)

	b.WriteString(s.channelStream(&ch))
	s.shellFlat(c, "users", []web.Crumb{
		web.C("成员", "/admin/users"),
		web.C(owner.Name, "/admin/users/"+fmt.Sprint(owner.Id)),
		web.C(name),
	}, b.String(), true)
}

func muteSuffix(ch *models.Channel) string {
	now := time.Now().Unix()
	if ch.Muted != 0 {
		return ` · <span style="color:var(--warning)">一直静音</span>`
	}
	if ch.MuteUntil > now {
		return ` · <span style="color:var(--warning)">静音至 ` +
			time.Unix(ch.MuteUntil, 0).Format("01-02 15:04") + `</span>`
	}
	return ""
}

func (s *Server) channelProps(ch *models.Channel) string {
	kv := func(k, v string) string {
		return fmt.Sprintf(`<div style="display:flex;gap:12px;padding:8px 0;border-bottom:1px solid var(--border)">`+
			`<div class="dim" style="width:120px;flex-shrink:0">%s</div><div class="wrap-any">%s</div></div>`,
			web.E(k), v)
	}
	return kv("频道 id", `<span class="mono">`+web.E(ch.Id)+`</span>`) +
		kv("发送 token", `<span class="mono">`+web.E(ch.Token)+`</span>`) +
		kv("最后使用", ago(ch.LastUsedAt)+`　<span class="dim mono">`+web.E(ch.LastUsedIP)+`</span>`)
}

// channelStream 消息流。正序排、自动滚到底，和 app 里一样。
func (s *Server) channelStream(ch *models.Channel) string {
	const limit = 40
	rows, err := s.admin().Messages(service.MessageFilter{
		ChannelId: ch.Id, Limit: limit, WithBody: true,
	})
	if err != nil {
		log.Error("admin: channel stream failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		return web.Card("", "", web.Empty("这个频道还没有消息。"))
	}
	// 查的时候倒序（要最近的 N 条），显示的时候正序（和 app 一致）
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}

	var b strings.Builder
	b.WriteString(`<div class="stream" id="stream">`)
	b.WriteString(`<div style="text-align:center"><a class="btn ghost sm" href="/admin/messages?channel=` +
		web.E(ch.Id) + `">看更早的 / 搜索</a></div>`)
	lastDay := ""
	for _, r := range rows {
		if d := time.Unix(r.Ctime, 0).Format("2006-01-02"); d != lastDay {
			lastDay = d
			b.WriteString(`<div style="text-align:center;padding:6px 0">` +
				`<span class="badge muted">` + web.E(dayLabel(r.Ctime)) + `</span></div>`)
		}
		b.WriteString(s.messageBubble(r))
	}
	b.WriteString(`</div>`)
	// 落地就停在最新那条上，和 app 的 defaultScrollAnchor(.bottom) 一个意思。
	// 滚的是流自己而不是整个页面——页面滚的话，频道名和操作按钮会跟着滚走。
	b.WriteString(`<script>(function(){var e=document.getElementById('stream');` +
		`if(e)e.scrollTop=e.scrollHeight})()</script>`)
	return b.String()
}

func dayLabel(ts int64) string {
	t := time.Unix(ts, 0)
	y, m, d := time.Now().Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	switch {
	case !t.Before(today):
		return "今天"
	case !t.Before(today.AddDate(0, 0, -1)):
		return "昨天"
	}
	return t.Format("1 月 2 日")
}

// messageBubble 一条消息，展开显示内容。
func (s *Server) messageBubble(r service.MessageRow) string {
	title := firstNonEmpty(r.Title, r.Summary)
	var body strings.Builder
	switch r.Type {
	case models.TypeCard:
		body.WriteString(cardItems(r.Extra, r.Summary))
	case models.TypeImage, models.TypeFile:
		if r.Summary != "" && r.Summary != title {
			body.WriteString(`<div class="dim" style="margin-bottom:8px">` + web.E(r.Summary) + `</div>`)
		}
		if uid := extraFileUID(r.Extra); uid != "" && s.Files != nil {
			body.WriteString(`<img src="` + web.E(s.Files.URL(uid, true)) +
				`" alt="" style="max-width:100%;max-height:320px;border-radius:10px;display:block">`)
		}
	default:
		txt := firstNonEmpty(r.Body, r.Summary)
		if txt != "" {
			// markdown 类型才渲染；纯文本按原样保留换行，
			// 不能把一段 shell 输出当成 markdown 去解释。
			if r.Type == models.TypeMarkdown {
				body.WriteString(web.Markdown(txt))
			} else {
				body.WriteString(`<pre class="body">` + web.E(txt) + `</pre>`)
			}
		}
	}
	// 可回复的消息把选项也画出来。
	//
	// **只读，一律置灰不可点**：后台是服务器主人查看用的，不是他替用户作答的地方。
	// 这里能点的话，一次误触就会把一个答案送进发送方的回调，
	// 而那一端可能真的去开一扇窗——它分不出这是谁点的。
	body.WriteString(replyReadonly(r))

	unread := ""
	if r.ReadAt == 0 {
		unread = `<span class="badge info"><i></i>未读</span>`
	}
	return fmt.Sprintf(`<div class="card"><div class="card-b" style="padding:14px 16px">
<div style="display:flex;align-items:baseline;gap:10px;margin-bottom:6px">
  <div style="font-weight:600;flex:1;min-width:0">%s</div>
  <span class="badge muted">%s</span>%s%s
  <span class="dim num" style="font-size:12px">%s</span>
</div>%s
<div style="margin-top:8px"><a class="btn ghost sm" href="/admin/messages/%s">投递记录</a></div>
</div></div>`,
		web.E(title), web.E(r.Type), unread, pushBadge(r.PushOK, r.PushTotal),
		time.Unix(r.Ctime, 0).Format("15:04"), body.String(), web.E(r.UID))
}

// replyReadonly 消息列表里的回复区，只读。
//
// **发了什么就画什么**：单选画按钮、多选画勾选框、数值画滑块、文本画输入框，
// 和 app 里那一屏是同一套东西。后台要看得出这条消息【长什么样】，
// 而不只是「回了什么」——只显示答案的话，管理员看不出用户当时面对的是几个选项、
// 范围是多少，也就无从判断这条消息设计得对不对。
//
// 已回复时控件反映答案：选中的那一项高亮、勾选框打勾、滑块停在那个值上、
// 输入框里是原文。没回复就是空的初始态。
//
// **一律不可点、不发任何请求。** 用 disabled 加 pointer-events:none 两道，
// 而且整块没有 form、没有 JS。后台是服务器主人查看用的，不是他替用户作答的地方：
// 能点的话一次误触就会把一个答案送进发送方的回调，而那一端分不出这是谁点的。
func replyReadonly(r service.MessageRow) string {
	if r.ReplyWebhook == "" {
		return ""
	}
	var e struct {
		Reply *struct {
			Type    string   `json:"type"`
			Options []string `json:"options"`
			Min     *float64 `json:"min"`
			Max     *float64 `json:"max"`
			Step    *float64 `json:"step"`
			Unit    string   `json:"unit"`
		} `json:"reply"`
	}
	if err := json.Unmarshal([]byte(r.Extra), &e); err != nil || e.Reply == nil {
		return ""
	}
	repliedAt, until := r.RepliedAt, r.ReplyUntil
	replied := repliedAt != 0
	raw := r.Reply

	var b strings.Builder
	// pointer-events:none 罩住整块——比逐个控件加 disabled 更难漏掉一个。
	b.WriteString(`<div style="margin-top:10px;padding-top:10px;border-top:1px solid var(--border);` +
		`pointer-events:none;user-select:none">`)

	switch e.Reply.Type {
	case models.ReplyChoice:
		picked := map[string]bool{}
		if replied {
			picked[raw] = true
		}
		b.WriteString(choiceRow(e.Reply.Options, picked))
	case models.ReplyMulti:
		picked := map[string]bool{}
		if replied {
			var xs []string
			_ = json.Unmarshal([]byte(raw), &xs)
			for _, x := range xs {
				picked[x] = true
			}
		}
		b.WriteString(checkList(e.Reply.Options, picked, replied))
	case models.ReplyNumber:
		b.WriteString(sliderRow(e.Reply.Min, e.Reply.Max, e.Reply.Step, e.Reply.Unit, raw, replied))
	case models.ReplyText:
		b.WriteString(textRow(raw, replied))
	default:
		b.WriteString(`<div class="dim" style="font-size:12px">这个版本的后台不认识的回复形态：` +
			web.E(e.Reply.Type) + `</div>`)
	}

	b.WriteString(replyStatusLine(replied, repliedAt, until))
	b.WriteString(`</div>`)
	return b.String()
}

// choiceRow 单选：选项并排。已回复时选中那个描边高亮，其余保持灰。
func choiceRow(options []string, picked map[string]bool) string {
	var b strings.Builder
	b.WriteString(`<div style="display:flex;gap:8px;flex-wrap:wrap">`)
	for _, o := range options {
		style := `flex:1;min-width:96px;text-align:center;padding:10px 14px;border-radius:10px;` +
			`font-size:14px;background:var(--muted);color:var(--muted-fg);border:1px solid transparent`
		if picked[o] {
			// 选中的那个用强调色描边：一眼看得出用户点的是哪个。
			style = `flex:1;min-width:96px;text-align:center;padding:10px 14px;border-radius:10px;` +
				`font-size:14px;font-weight:600;background:var(--accent-soft);color:var(--primary);` +
				`border:1px solid var(--primary)`
		}
		b.WriteString(`<div style="` + style + `">` + web.E(o) + `</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// checkList 多选：每项一行，选中的打勾。
func checkList(options []string, picked map[string]bool, replied bool) string {
	var b strings.Builder
	b.WriteString(`<div style="border:1px solid var(--border);border-radius:10px;padding:2px 12px">`)
	for i, o := range options {
		sep := ""
		if i < len(options)-1 {
			sep = `border-bottom:1px solid var(--border);`
		}
		checked := ""
		if picked[o] {
			checked = " checked"
		}
		weight := "400"
		color := "var(--fg)"
		if picked[o] {
			weight = "600"
		} else if replied {
			// 已回复但没选中的，淡下去——对照之下看得出「这个没被选」。
			color = "var(--muted-fg)"
		}
		b.WriteString(`<label style="display:flex;align-items:center;gap:10px;` +
			`min-height:40px;` + sep + `">` +
			`<input type="checkbox" disabled` + checked + `>` +
			`<span style="font-size:14px;font-weight:` + weight + `;color:` + color + `">` +
			web.E(o) + `</span></label>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// sliderRow 数值：滑块停在回复的那个值上；没回复就停在中间，和 app 的初值一致。
func sliderRow(min, max, step *float64, unit, raw string, replied bool) string {
	lo, hi, st := 0.0, 100.0, 1.0
	if min != nil {
		lo = *min
	}
	if max != nil {
		hi = *max
	}
	if step != nil && *step > 0 {
		st = *step
	}
	val := lo + (hi-lo)/2
	if replied {
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			val = v
		}
	}
	big := `<span style="font-size:24px;font-weight:700;font-variant-numeric:tabular-nums">` +
		web.E(trimNum(val)) + `</span><span class="dim" style="font-size:14px;font-weight:600">` +
		web.E(unit) + `</span>`
	if !replied {
		// 没回复时不该把一个具体数字摆成事实，弱化成灰的。
		big = `<span class="dim" style="font-size:24px;font-weight:700;` +
			`font-variant-numeric:tabular-nums;opacity:.5">` + web.E(trimNum(val)) +
			web.E(unit) + `</span>`
	}
	return `<div style="display:flex;flex-direction:column;gap:6px">` + big +
		fmt.Sprintf(`<input type="range" disabled min="%g" max="%g" step="%g" value="%g" `+
			`style="width:100%%;accent-color:var(--primary)">`, lo, hi, st, val) +
		`<div class="dim" style="display:flex;justify-content:space-between;font-size:11px">` +
		`<span>` + web.E(trimNum(lo)+unit) + `</span>` +
		`<span>` + web.E(trimNum(hi)+unit) + `</span></div></div>`
}

// textRow 文本：输入框里是回复原文；没回复就是占位提示。
func textRow(raw string, replied bool) string {
	if replied {
		return `<input type="text" disabled value="` + web.E(raw) + `" ` +
			`style="width:100%;padding:10px 12px;border-radius:10px;border:1px solid var(--border);` +
			`background:var(--card);color:var(--fg);font-size:14px">`
	}
	return `<input type="text" disabled placeholder="等待文字回复" ` +
		`style="width:100%;padding:10px 12px;border-radius:10px;border:1px solid var(--border);` +
		`background:var(--muted);color:var(--muted-fg);font-size:14px">`
}

// replyStatusLine 控件下面那一行状态：回了什么、什么时候，或者还在等、等到几点。
func replyStatusLine(replied bool, repliedAt, until int64) string {
	if replied {
		return `<div style="display:flex;align-items:center;gap:8px;margin-top:8px">` +
			`<span class="badge ok"><i></i>已回复</span>` +
			`<span class="dim" style="font-size:12px">` +
			web.E(time.Unix(repliedAt, 0).Format("2006-01-02 15:04:05")) + `</span></div>`
	}
	if until > 0 && time.Now().Unix() > until {
		return `<div style="margin-top:8px;font-size:12px;color:var(--warning)">` +
			`时限已过（` + web.E(time.Unix(until, 0).Format("15:04:05")) + `），没有回复</div>`
	}
	if until > 0 {
		return `<div class="dim" style="margin-top:8px;font-size:12px">等待回复 · 截止 ` +
			web.E(time.Unix(until, 0).Format("15:04:05")) + `</div>`
	}
	return `<div class="dim" style="margin-top:8px;font-size:12px">等待回复 · 没有设时限</div>`
}

// trimNum 去掉没意义的小数尾巴：24.0 显示成 24，24.5 还是 24.5。
func trimNum(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// cardItems 把 card 的键值对画成行。解析失败就退回摘要——
// 一条畸形消息不该让整页空掉。
func cardItems(extra, summary string) string {
	var e struct {
		Items []struct{ K, V, Style string } `json:"items"`
	}
	if json.Unmarshal([]byte(extra), &e) != nil || len(e.Items) == 0 {
		return `<pre class="body">` + web.E(summary) + `</pre>`
	}
	var b strings.Builder
	b.WriteString(`<div style="display:flex;flex-wrap:wrap;gap:6px">`)
	for _, it := range e.Items {
		kind := "muted"
		switch it.Style {
		case "ok":
			kind = "ok"
		case "warn":
			kind = "warn"
		case "error":
			kind = "err"
		}
		fmt.Fprintf(&b, `<span class="badge %s">%s <b style="margin-left:4px">%s</b></span>`,
			kind, web.E(it.K), web.E(it.V))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// extraFileUID 从 extra 里取附件 uid。不引 json 解码器是因为这里只要一个字段，
// 而 extra 的形状由服务端自己写入，不是外部输入。
func extraFileUID(extra string) string {
	i := strings.Index(extra, `"file"`)
	if i < 0 {
		return ""
	}
	rest := extra[i+6:]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	rest = rest[j+1:]
	k := strings.Index(rest, `"`)
	if k <= 0 {
		return ""
	}
	return rest[:k]
}

func (s *Server) adminChannelPurge(c *gin.Context) {
	id := c.Param("id")
	ch, _, err := s.admin().Channel(id)
	if err != nil {
		c.Redirect(http.StatusFound, "/admin/users")
		return
	}
	maxID, err := s.admin().ChannelMaxMsgId(id)
	if err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := service.NewSync(s.DAO).PurgeChannel(ch.UserId, id, maxID); err != nil {
		c.String(http.StatusInternalServerError, err.Error())
		return
	}
	c.Redirect(http.StatusFound, "/admin/channels/"+id)
}
