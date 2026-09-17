package web

import (
	"fmt"
	"html"
	"strings"
)

// Icon 侧栏与按钮用的线性图标（lucide 风格，24 网格）。
var Icon = map[string]string{
	"dash":    `<rect x="3" y="3" width="7" height="9" rx="2"/><rect x="14" y="3" width="7" height="5" rx="2"/><rect x="14" y="12" width="7" height="9" rx="2"/><rect x="3" y="16" width="7" height="5" rx="2"/>`,
	"msg":     `<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`,
	"hash":    `<line x1="4" y1="9" x2="20" y2="9"/><line x1="4" y1="15" x2="20" y2="15"/><line x1="10" y1="3" x2="8" y2="21"/><line x1="16" y1="3" x2="14" y2="21"/>`,
	"phone":   `<rect x="5" y="2" width="14" height="20" rx="2"/><line x1="12" y1="18" x2="12.01" y2="18"/>`,
	"users":   `<path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M22 21v-2a4 4 0 0 0-3-3.87"/>`,
	"gear":    `<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.6a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"/>`,
	"qr":      `<rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 14h3v3h-3zM18 18h3v3h-3z"/>`,
	"chev":    `<polyline points="9 18 15 12 9 6"/>`,
	"zap":     `<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>`,
	"alert":   `<path d="M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0Z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/>`,
	"logout":  `<path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/>`,
	"trash":   `<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`,
	"refresh": `<path d="M21 12a9 9 0 1 1-3-6.7"/><polyline points="21 3 21 9 15 9"/>`,
	"copy":    `<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`,
}

// Svg 渲染一个图标。
func Svg(name string, size int) string {
	return fmt.Sprintf(`<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" `+
		`stroke-linecap="round" stroke-linejoin="round" width="%d" height="%d" aria-hidden="true">%s</svg>`,
		size, size, Icon[name])
}

// E 转义。所有进模板的用户数据都要过它——消息标题、频道名这些都是外部输入。
func E(s string) string { return html.EscapeString(s) }

type navItem struct{ Key, Href, Icon, Label string }

// Crumb 面包屑一节。Href 为空表示不可点。
type Crumb struct{ Text, Href string }

// C 造一节面包屑。
func C(text string, href ...string) Crumb {
	if len(href) > 0 {
		return Crumb{text, href[0]}
	}
	return Crumb{Text: text}
}

// 导航按【归属关系】而不是按数据库表来分：
// 消息属于频道，频道和设备属于成员。所以顶层只有成员，
// 频道和设备在成员详情里，消息在频道详情里。
//
// 「消息」仍然留一个顶层入口——它是跨频道的全局检索，
// 和「某个频道下的消息」是两种不同的用途，不该只能从钻取路径到达。
var navItems = []navItem{
	{"dash", "/admin", "dash", "概览"},
	{"users", "/admin/users", "users", "成员"},
	{"messages", "/admin/messages", "msg", "搜索消息"},
	{"pair", "/admin/pair", "qr", "配对设备"},
	{"settings", "/admin/settings", "gear", "设置"},
}

// Shell 页面外壳：侧栏 + 顶栏 + 内容。
type Shell struct {
	Nav        string // 当前高亮的导航项
	Crumbs     []Crumb
	ServerName string
	Host       string
	Version    string
	Online     bool
	Body       string // 已经渲染好的 HTML
	// Flat 让内容区自己不滚，由页面内部的某个区域滚（聊天窗那种布局）。
	Flat bool
}

func (s Shell) titleText() string {
	parts := make([]string, 0, len(s.Crumbs))
	for _, c := range s.Crumbs {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, " · ")
}

func (s Shell) Render() string {
	var nav strings.Builder
	for _, it := range navItems {
		on := ""
		if it.Key == s.Nav {
			on = " on"
		}
		fmt.Fprintf(&nav, `<a class="item%s" href="%s">%s%s</a>`, on, it.Href, Svg(it.Icon, 16), it.Label)
	}

	// 面包屑除了最后一项都可以点回去——钻进三层之后没有返回路径是很烦的
	crumbs := make([]string, 0, len(s.Crumbs))
	for i, c := range s.Crumbs {
		if i == len(s.Crumbs)-1 {
			crumbs = append(crumbs, fmt.Sprintf(`<span class="cur">%s</span>`, E(c.Text)))
			continue
		}
		if c.Href == "" {
			crumbs = append(crumbs, fmt.Sprintf(`<span>%s</span>`, E(c.Text)))
			continue
		}
		crumbs = append(crumbs, fmt.Sprintf(`<a href="%s">%s</a>`, E(c.Href), E(c.Text)))
	}

	dot := "var(--success)"
	state := "运行中"
	if !s.Online {
		dot, state = "var(--danger)", "异常"
	}

	return fmt.Sprintf(`<!doctype html><html lang="zh-CN"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>%s · Knockbox</title><style>%s</style></head>
<body><div class="app fixed">
  <aside class="sidebar">
    <div class="brand"><div class="brand-mark">%s</div>
      <div><div class="brand-name">Knockbox</div><div class="brand-sub">%s</div></div></div>
    <nav class="nav">%s</nav>
    <div class="side-foot"><span class="dot" style="background:%s"></span>%s · %s</div>
  </aside>
  <main class="main">
    <div class="topbar"><div class="crumb">%s</div><div class="spacer"></div>
      <a class="btn ghost sm" href="/logout">%s退出登录</a></div>
    <div class="content %s">%s</div>
  </main>
</div></body></html>`,
		E(s.titleText()), Style, Mark, E(s.ServerName),
		nav.String(), dot, state, E(s.Version),
		strings.Join(crumbs, Svg("chev", 13)), Svg("logout", 15), contentClass(s.Flat), s.Body)
}

// Card 一张卡片。title 为空时不画头部。
func Card(title, tools, body string) string {
	head := ""
	if title != "" {
		head = fmt.Sprintf(`<div class="card-h"><div class="t">%s</div><div class="spacer"></div>%s</div>`,
			E(title), tools)
	}
	return fmt.Sprintf(`<div class="card">%s%s</div>`, head, body)
}

// Stat 概览页的数字块。
func Stat(label, value, kind, sub string) string {
	return fmt.Sprintf(`<div class="card stat"><div class="card-b" style="padding:16px">
<div class="dim" style="font-size:12.5px">%s</div>
<div class="num" style="font-size:28px;font-weight:600;letter-spacing:-0.02em;margin:4px 0 2px">%s</div>
<div class="badge %s">%s</div></div></div>`, E(label), E(value), kind, E(sub))
}

// Empty 空状态。**一句话说清为什么空**，别只写「暂无数据」——
// 用户要的是下一步做什么，不是一个确认它坏没坏的谜题。
func Empty(text string) string {
	return fmt.Sprintf(`<div class="empty">%s</div>`, E(text))
}

func contentClass(flat bool) string {
	if flat {
		return "flat"
	}
	return "scroll"
}

// Tabs 页内切换。用链接而不是 JS：地址栏能带着 tab 一起分享和刷新，
// 服务端直出页面也不必为了一个切换引入状态。
func Tabs(current string, items [][2]string) string {
	var b strings.Builder
	b.WriteString(`<div class="tabs">`)
	for _, it := range items {
		on := ""
		if it[0] == current {
			on = " on"
		}
		fmt.Fprintf(&b, `<a class="tab%s" href="%s">%s</a>`, on, E(it[1]), E(it[0]))
	}
	b.WriteString(`</div>`)
	return b.String()
}
