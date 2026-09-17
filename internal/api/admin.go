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
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// shell 填好每页都一样的那部分。
func (s *Server) shell(c *gin.Context, nav string, crumbs []web.Crumb, body string) {
	s.shellFlat(c, nav, crumbs, body, false)
}

// shellFlat flat=true 时内容区不滚，由页面内部的滚动区负责（频道那种聊天窗布局）。
func (s *Server) shellFlat(c *gin.Context, nav string, crumbs []web.Crumb, body string, flat bool) {
	host := s.ExternalURL
	if u := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(host, "https://"), "http://"), "/", 2); len(u) > 0 {
		host = u[0]
	}
	page := web.Shell{
		Nav: nav, Crumbs: crumbs, ServerName: s.Name, Host: host,
		Version: s.Version, Online: true, Body: body, Flat: flat,
	}.Render()
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(page))
}

func ago(ts int64) string {
	if ts <= 0 {
		return "—"
	}
	d := time.Since(time.Unix(ts, 0))
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
	return time.Unix(ts, 0).Format("2006-01-02")
}

func clock(ts int64) string {
	if ts <= 0 {
		return "—"
	}
	t := time.Unix(ts, 0)
	if time.Since(t) < 24*time.Hour {
		return t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

// channelName 从不透明 meta 里取名字。服务端平时不解析 meta，
// 只有管理界面例外——给人看的列表里显示一串 ULID 是没法用的。
//
// 必须真的按 JSON 解析。用字符串查找 "name" 的话，值里出现这几个字符
// （或者值本身带转义引号）就会取到错误的内容。
func channelName(meta, id string) string {
	var m struct {
		Name string `json:"name"`
	}
	if json.Unmarshal([]byte(meta), &m) == nil {
		if n := strings.TrimSpace(m.Name); n != "" {
			return n
		}
	}
	if len(id) > 6 {
		return id[:6]
	}
	return id
}

// ── 概览 ──────────────────────────────────────────────

func (s *Server) adminDash(c *gin.Context) {
	const window = int64(24 * 3600)
	since := time.Now().Unix() - window

	// 全站口径，不按登录的这个管理员过滤。理由见 service.Admin.Overview：
	// 公共实例上管理员自己那个 uid 基本没有流量，按他过滤会让首页第一个数字
	// 恒显示 0，而服务器实际推了几万条。
	o, err := s.admin().Overview(0, since, window)
	if err != nil {
		log.Error("admin: overview stats failed", log.Any("error", err.Error()))
	}

	rate, rateKind := "—", "muted"
	if v, ok := o.Rate(); ok {
		rate = fmt.Sprintf("%.1f%%", v)
		switch {
		case v >= 99:
			rateKind = "ok"
		case v >= 90:
			rateKind = "warn"
		default:
			rateKind = "err"
		}
	}

	var b strings.Builder
	b.WriteString(`<div class="ph"><div><h1>概览</h1><div class="sub">` +
		web.E(s.Name) + ` · 最近 24 小时</div></div></div>`)
	b.WriteString(`<div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:16px">`)
	b.WriteString(web.Stat("24 小时消息", strconv.FormatInt(o.Messages, 10), "muted",
		fmt.Sprintf("前一天 %d", o.MessagesPrev)))
	b.WriteString(web.Stat("推送成功率", rate, rateKind,
		fmt.Sprintf("%d 条失败 · %d 条重试中", o.PushFailed, o.PushRetrying)))
	b.WriteString(web.Stat("可推送设备", strconv.FormatInt(o.DevicesPushable, 10), "info",
		fmt.Sprintf("%d 台走沙箱", o.DevicesSandbox)))
	b.WriteString(web.Stat("频道", strconv.FormatInt(o.Channels, 10), "muted",
		fmt.Sprintf("%d 个静音", o.ChannelsMuted)))
	b.WriteString(`</div>`)

	b.WriteString(`<div style="display:grid;grid-template-columns:2fr 1fr;gap:16px;align-items:start">`)
	b.WriteString(web.Card("最近消息",
		`<a class="btn ghost sm" href="/admin/messages">查看全部`+web.Svg("chev", 14)+`</a>`,
		s.recentMessages(0, 8)))
	b.WriteString(web.Card("推送失败 · 24 小时", "", s.failureBreakdown(0, since)))
	b.WriteString(`</div>`)

	s.shell(c, "dash", []web.Crumb{web.C("概览")}, b.String())
}

func (s *Server) recentMessages(uid int64, limit int) string {
	rows, err := s.admin().RecentMessages(uid, limit)
	if err != nil {
		log.Error("admin: recent messages failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		return web.Empty("还没有消息。往任意一个频道 curl 一条试试。")
	}
	var b strings.Builder
	b.WriteString(`<table><thead><tr><th style="padding-left:16px">时间</th><th>频道</th>` +
		`<th>类型</th><th>标题</th><th>推送</th></tr></thead><tbody>`)
	for _, r := range rows {
		fmt.Fprintf(&b, `<tr><td style="padding-left:16px" class="dim num">%s</td><td>%s</td>`+
			`<td><span class="badge muted">%s</span></td>`+
			`<td style="font-weight:500"><a href="/admin/messages/%s">%s</a></td><td>%s</td></tr>`,
			clock(r.Ctime), web.E(channelName(r.Meta, r.ChannelId)), web.E(r.Type),
			web.E(r.UID), web.E(trunc(firstNonEmpty(r.Title, r.Summary), 42)),
			pushBadge(r.PushOK, r.PushTotal))
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

func pushBadge(ok, total int64) string {
	switch {
	case total == 0:
		return `<span class="badge muted"><i></i>未推送</span>`
	case ok == total:
		return fmt.Sprintf(`<span class="badge ok"><i></i>%d/%d</span>`, ok, total)
	case ok == 0:
		return fmt.Sprintf(`<span class="badge err"><i></i>%d/%d</span>`, ok, total)
	}
	return fmt.Sprintf(`<span class="badge warn"><i></i>%d/%d</span>`, ok, total)
}

func (s *Server) failureBreakdown(uid, since int64) string {
	rows, err := s.admin().Failures(uid, since, 6)
	if err != nil {
		log.Error("admin: failure breakdown failed", log.Any("error", err.Error()))
	}
	if len(rows) == 0 {
		return `<div class="card-b"><div class="badge ok"><i></i>24 小时内没有失败</div></div>`
	}
	var b strings.Builder
	b.WriteString(`<div class="card-b" style="display:flex;flex-direction:column;gap:10px">`)
	for _, r := range rows {
		fmt.Fprintf(&b, `<div style="display:flex;align-items:center;gap:10px">`+
			`<span class="mono">%s</span><span class="badge muted">%d</span>`+
			`<div class="spacer"></div><span class="num" style="font-weight:600">%d</span></div>`,
			web.E(r.Reason), r.HTTPStatus, r.Count)
	}
	b.WriteString(`<div class="note">` + web.Svg("alert", 14) +
		`<div>收到 <b>410 Unregistered</b> 只在 Apple 给的时间戳晚于设备最后更新时才注销 —— ` +
		`重装后延迟到达的旧 410 不会误杀刚配对的设备。</div></div></div>`)
	return b.String()
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// admin 后台的查询与写操作。无状态，每次现造。
func (s *Server) admin() *service.Admin { return service.NewAdmin(s.DAO) }

// quota 每次用当前设置构造——设置在后台改完要立刻生效，不能等重启。
func (s *Server) quota() *service.Quota {
	return service.NewQuota(s.DAO, s.Settings.Limits())
}
