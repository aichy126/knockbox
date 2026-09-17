package web

import (
	"fmt"
	"strconv"
	"strings"
)

// Sample 一条示例的展示形态。真正发出去的内容在 api 包里，
// 两边同一份定义——页面上写的和按钮做的必须一致。
type Sample struct {
	ID, Title, Desc, Curl string
	// Lang 代码块左上角那个标签。bash / json 之类。
	Lang string
	// Live 为真时这条旁边有「发这条」按钮。没有 token 的介绍页是 false。
	Live bool
}

type GuideData struct {
	SiteName, Name, Host, Token string
	// 公共模式下的配额。自建时 Public 为 false，整节不出现。
	Public        bool
	MaxChannels   int
	MaxPerDay     int
	RetentionDays int
	Samples       []Sample
	Agent         string
	// MCP 各种客户端的加法。地址里的 token 与 Agent 那段提示词同一个来源。
	MCP  []MCPForm
	Lang Lang
}

// MCPForm 接 MCP 的一种写法：标签页的名字、代码块的语言标签、可复制的内容。
//
// 有三种是因为客户端本来就分三类：会自己读说明去配的 AI 助手、要人手填配置文件的、
// 有命令行的。只给其中一种，另外两类的人得先自己翻译一遍，而翻译是会错的。
type MCPForm struct {
	Tab, Lang, Code string
}

// NoticePage 一句话的独立提示页。错误也要有个体面的落地处，
// 不能给用户一段 JSON 或者一个白屏。
func NoticePage(lang Lang, title, detail string) string {
	return strings.NewReplacer(
		"{{LANG}}", lang.Attr(),
		"{{STYLE}}", Style,
		"{{LANGCSS}}", LangCSS,
		"{{LANGSW}}", LangSwitch(lang),
		"{{TITLE}}", E(title),
		"{{DETAIL}}", E(detail),
	).Replace(`<!doctype html><html lang="{{LANG}}"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{TITLE}}</title><style>{{STYLE}}{{LANGCSS}}
.mid{max-width:520px;margin:0 auto;padding:80px 24px;text-align:center}
.mid h1{font-size:22px;margin:0 0 8px}</style></head>
<body>{{LANGSW}}<div class="app"><div class="mid"><h1>{{TITLE}}</h1>
<p class="dim" style="line-height:1.7">{{DETAIL}}</p></div></div></body></html>`)
}

// GuidePage 使用说明 + 发送页，**一页到底**。
//
// 两种形态共用这一份模板：
//   - 有 token（/s/<token>）：地址是真的，每条示例旁边有「发这条」，按下去手机就响
//   - 没 token（/docs）：地址是占位符，没有按钮，供还没接入的人先了解
//
// 合成一页是因为它们本来就是同一件事。拆成「怎么用」和「我的地址」两页，
// 读者得先判断自己该看哪一份，而他真正需要的那份——带着自己 token 的那份——
// 恰恰只有接入之后才存在。
//
// 模板用 Replacer 而不是 Sprintf：里面有 `92%` 这类字面百分号，
// Sprintf 会把它当占位符。
func GuidePage(d GuideData) string {
	t := T(d.Lang)
	live := d.Token != ""
	url := d.Host + "/api/v1/send/" + d.Token
	if !live {
		url = d.Host + "/api/v1/send/" + t.GuidePlaceholder
	}

	hero := `<div class="hero">
  <div class="label">` + E(t.GuideYourURL) + `</div>
  ` + snip("u", "url", url, t) + `
  <div class="dim" style="font-size:12.5px">` + t.GuideYourURLSub + `</div>
</div>`
	if !live {
		hero = `<div class="hero">
  <div class="label">` + E(t.GuideSampleURL) + `</div>
  ` + snip("u", "url", url, t) + `
  <div class="dim" style="font-size:12.5px">` + t.GuideSampleURLSub + `</div>
  <a class="btn" style="margin-top:14px" href="/">` + E(t.GuideGoJoin) + `</a>
</div>`
	}

	var ex strings.Builder
	for _, s := range d.Samples {
		ex.WriteString(`<section id="ex-` + E(s.ID) + `">`)
		ex.WriteString(`<h2>` + E(s.Title) + `</h2>`)
		ex.WriteString(`<p class="sub">` + E(s.Desc) + `</p>`)
		ex.WriteString(snipLive(s.ID, s.Lang, s.Curl, s.Live, d.Token, t))
		ex.WriteString(`</section>`)
	}

	limits := `<p class="sub">` + E(t.GuideNoLimits) + `</p>`
	if d.Public {
		limits = `<ul class="plain">` +
			`<li>` + fmt.Sprintf(t.GuideLimitCh, d.MaxChannels) + `</li>` +
			`<li>` + fmt.Sprintf(t.GuideLimitDay, d.MaxPerDay) + `</li>` +
			`<li>` + fmt.Sprintf(t.GuideLimitKeep, d.RetentionDays) + `</li>` +
			`<li>` + t.GuideLimit429 + `</li>` +
			`</ul>`
	}

	title := d.Name
	lede := t.GuideChannelLede
	if !live {
		title = fmt.Sprintf(t.GuideDocsTitle, d.SiteName)
		lede = t.GuideDocsLede
	}

	return strings.NewReplacer(
		"{{LANG}}", d.Lang.Attr(),
		"{{STYLE}}", Style,
		"{{LANGCSS}}", LangCSS,
		"{{LANGSW}}", LangSwitch(d.Lang),
		"{{CODECSS}}", CodeCSS,
		"{{TITLE}}", E(title),
		"{{LEDE}}", E(lede),
		"{{HERO}}", hero,
		"{{SAMPLES}}", ex.String(),
		"{{AGENT}}", snip("agent", "prompt", d.Agent, t),
		"{{MCP}}", snipTabs("mcp", d.MCP, t),
		"{{LIMITS}}", limits,
		"{{ALERT}}", Svg("alert", 14),
		"{{H_JOIN}}", E(t.GuideHowToJoin),
		"{{H_INSTALL}}", E(t.GuideInstallH),
		"{{P_INSTALL}}", t.GuideInstallP,
		"{{H_CHANNEL}}", E(t.GuideChannelH),
		"{{P_CHANNEL}}", t.GuideChannelP,
		"{{H_SEND}}", E(t.GuideHowToSend),
		"{{H_AGENT}}", E(t.GuideAgentH),
		"{{P_AGENT}}", t.GuideAgentP,
		"{{H_PROMPT}}", E(t.GuidePromptH),
		"{{H_MCP}}", E(t.GuideMCPH),
		"{{P_MCP}}", t.GuideMCPP,
		"{{H_OTHER}}", E(t.GuideOtherH),
		"{{H_LIMITS}}", E(t.GuideLimitsH),
		"{{H_SELFHOST}}", E(t.GuideSelfHostH),
		"{{P_SELFHOST}}", t.GuideSelfHostP,
		"{{P_TOKENWARN}}", t.GuideTokenWarn,
		"{{JS_SENDING}}", jsStr(t.GuideSending),
		"{{JS_SENT}}", jsStr(t.GuideSent),
		"{{JS_SENDFAIL}}", jsStr(t.GuideSendFail),
		"{{JS_COPIED}}", jsStr(t.GuideCopied),
	).Replace(guideTpl)
}

// snip 一个只能复制的代码块。
func snip(id, lang, code string, t Texts) string {
	return `<div class="snip">
  <div class="snip-bar"><span class="snip-lang">` + E(lang) + `</span>
    <span class="snip-act"><button class="btn outline sm" onclick="cp(this,'` + id + `')">` +
		Svg("copy", 13) + E(t.GuideCopy) + `</button></span></div>
  <div data-raw="` + E(code) + `" id="` + id + `">` + CodeBlock(code) + `</div>
</div>`
}

// snipTabs 同一件事的几种写法，共用一个代码块。
//
// 复制按钮只有一颗，复制的是当前这一页——三颗按钮并排会让人先想「该按哪颗」，
// 而他要的只是眼前看到的这段。
func snipTabs(id string, forms []MCPForm, t Texts) string {
	if len(forms) == 0 {
		return ""
	}
	var tabs, panes strings.Builder
	for i, f := range forms {
		on, hidden := "", " hidden"
		if i == 0 {
			on, hidden = " on", ""
		}
		idx := strconv.Itoa(i)
		tabs.WriteString(`<button class="snip-tab` + on + `" onclick="tab(this,'` + id + `',` + idx + `)">` +
			E(f.Tab) + `</button>`)
		panes.WriteString(`<div class="snip-pane" id="` + id + `-` + idx + `"` + hidden +
			` data-lang="` + E(f.Lang) + `" data-raw="` + E(f.Code) + `">` + CodeBlock(f.Code) + `</div>`)
	}
	return `<div class="snip">
  <div class="snip-bar tabbed"><span class="snip-tabs">` + tabs.String() + `</span>
    <span class="snip-act"><button class="btn outline sm" onclick="cpTab(this,'` + id + `')">` +
		Svg("copy", 13) + E(t.GuideCopy) + `</button></span></div>
  ` + panes.String() + `
</div>`
}

// snipLive 代码块 + 一颗真把它发出去的按钮。
//
// 「发这条」走的是和展示的命令**同一份定义**（api 包里的 samples），
// 所以按下去看到的效果，就是照着抄能得到的效果。
func snipLive(id, lang, code string, live bool, token string, t Texts) string {
	send := ""
	if live {
		// 不用 form 提交：那会刷新页面并滚到这一节，按一下页面就自己动一下。
		send = `<button class="btn sm" onclick="tryOne(this,'` + E(token) + `','` + E(id) + `')">` +
			Svg("zap", 13) + E(t.GuideSend) + `</button>`
	}
	return `<div class="snip">
  <div class="snip-bar"><span class="snip-lang">` + E(lang) + `</span>
    <span class="snip-act">` + send +
		`<button class="btn outline sm" onclick="cp(this,'ex` + id + `')">` +
		Svg("copy", 13) + E(t.GuideCopy) + `</button></span></div>
  <div data-raw="` + E(code) + `" id="ex` + id + `">` + CodeBlock(code) + `</div>
</div>`
}

const guideTpl = `<!doctype html><html lang="{{LANG}}"><head>
<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{TITLE}}</title><style>{{STYLE}}{{CODECSS}}{{LANGCSS}}
.doc{max-width:780px;margin:0 auto;padding:40px 24px 96px}
.doc h1{font-size:28px;margin:0 0 6px;letter-spacing:-0.02em}
.doc .lede{color:var(--muted-fg);font-size:14.5px;line-height:1.7;margin:0 0 26px}
.doc section{margin-top:32px}
.doc h2{font-size:17px;margin:0 0 6px;letter-spacing:-0.01em}
.doc h3{font-size:13px;margin:40px 0 2px;letter-spacing:.08em;text-transform:uppercase;
  color:var(--muted-fg);font-weight:600}
.doc .sub{color:var(--muted-fg);font-size:13.5px;line-height:1.7;margin:0 0 12px}
.doc p{line-height:1.75;margin:0 0 12px;font-size:14.5px}
.doc ul.plain{margin:0 0 12px;padding-left:20px;line-height:1.9;font-size:14.5px}

.hero{border:1px solid var(--line);border-radius:16px;background:var(--card);padding:18px}
.hero .label{font-size:12px;color:var(--muted-fg);margin-bottom:8px;letter-spacing:.02em}
.toc{display:flex;flex-wrap:wrap;gap:8px;margin:14px 0 0}
.toc a{font-size:12.5px;padding:5px 10px;border:1px solid var(--line);border-radius:999px;
  color:var(--muted-fg);text-decoration:none}
.toc a:hover{color:var(--fg);border-color:var(--fg)}
</style></head><body>{{LANGSW}}<div class="app"><div class="doc">

<h1>{{TITLE}}</h1>
<p class="lede">{{LEDE}}</p>

{{HERO}}

<h3>{{H_JOIN}}</h3>
<section style="margin-top:10px">
  <h2>{{H_INSTALL}}</h2>
  <p class="sub">{{P_INSTALL}}</p>
</section>
<section>
  <h2>{{H_CHANNEL}}</h2>
  <p class="sub">{{P_CHANNEL}}</p>
</section>

<h3>{{H_SEND}}</h3>
{{SAMPLES}}

<h3>{{H_AGENT}}</h3>
<section style="margin-top:10px">
  <h2>{{H_PROMPT}}</h2>
  <p class="sub">{{P_AGENT}}</p>
  {{AGENT}}
</section>
<section>
  <h2>{{H_MCP}}</h2>
  <p class="sub">{{P_MCP}}</p>
  {{MCP}}
</section>

<h3>{{H_OTHER}}</h3>
<section style="margin-top:10px">
  <h2>{{H_LIMITS}}</h2>
  {{LIMITS}}
</section>
<section>
  <h2>{{H_SELFHOST}}</h2>
  <p class="sub">{{P_SELFHOST}}</p>
</section>

<div class="note" style="margin-top:34px">{{ALERT}}<div>{{P_TOKENWARN}}</div></div>

</div></div>
<script>
var TXT = {sending:{{JS_SENDING}}, sent:{{JS_SENT}}, fail:{{JS_SENDFAIL}}, copied:{{JS_COPIED}}};
// 发一条示例。**不刷新页面**：反馈直接落在按钮上，视口一动不动。
function tryOne(b, token, id){
  if (b.disabled) return;
  var t = b.innerHTML; b.disabled = true; b.textContent = TXT.sending;
  var body = new URLSearchParams(); body.set('sample', id);
  fetch('/s/' + token + '/try', {method:'POST', body: body})
    .then(function(r){ return r.json() })
    .then(function(j){
      b.textContent = j.ok ? TXT.sent : (j.msg || TXT.fail);
      setTimeout(function(){ b.innerHTML = t; b.disabled = false; }, 2200);
    })
    .catch(function(){
      b.textContent = TXT.fail;
      setTimeout(function(){ b.innerHTML = t; b.disabled = false; }, 2200);
    });
}
// 切换写法。面板用 hidden 属性而不是 display:none，省一条样式规则。
function tab(b,id,i){
  var bar=b.parentNode, k;
  for (k=0;k<bar.children.length;k++) bar.children[k].className='snip-tab'+(k===i?' on':'');
  for (k=0;;k++){ var p=document.getElementById(id+'-'+k); if(!p) break; p.hidden=(k!==i); }
}
// 复制当前显示的那一页。
function cpTab(b,id){
  for (var k=0;;k++){
    var p=document.getElementById(id+'-'+k); if(!p) break;
    if (!p.hidden) { cp(b, id+'-'+k); return; }
  }
}
// 复制的是 data-raw 里的原文，不是渲染后的 HTML——
// 着色往里面塞了 span，取 innerText 在换行和空白上会走样。
function cp(b,id){
  var el=document.getElementById(id);
  navigator.clipboard.writeText(el.getAttribute('data-raw')||el.innerText).then(function(){
    var t=b.textContent; b.textContent=TXT.copied;
    setTimeout(function(){b.textContent=t},1500);
  });
}
</script></body></html>`
