// Package web 服务端直出的 HTML。
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"maps"
	"net/url"
	"sort"
	"strings"
)

// Lang 一门语言。取值就是 locales 下的文件名：en / zh / ja ...
type Lang string

const (
	LangEN Lang = "en"
	LangZH Lang = "zh"
)

//go:embed locales/*.json
var localeFS embed.FS

var (
	// texts 每门语言的全部文案，启动时从 locales/*.json 读进来。
	texts = map[Lang]Texts{}
	// langs 已注册的语言。英文排第一，其余按名字排序——语言开关按这个顺序显示。
	langs []Lang
)

// 加一门语言 = 往 locales/ 放一个 JSON 文件。这里一行都不用改，
// 别处也没有一张需要同步维护的语言清单——那种清单正是「提了 PR 却漏了一处」的来源。
//
// 语料坏了直接 panic：服务起不来，比带着满页空白跑起来好。后者要等有人
// 正好访问到那一页才会被发现。
func init() {
	base := Texts{}
	mustLoadLocale("locales/en.json", &base)
	texts[LangEN] = base
	langs = []Lang{LangEN}

	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		panic("i18n: 读不到 locales 目录: " + err.Error())
	}
	var rest []Lang
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		name := Lang(strings.TrimSuffix(e.Name(), ".json"))
		if name == LangEN {
			continue
		}
		// 先拷一份英文再往上盖：这门语言没翻的 key 自然回落成英文。
		// 半翻完的语料照样能用，不会在页面上留一片空白，
		// 也就不必等「翻完 100%」才敢合一个 PR。
		t := base
		// map 字段要单独复制：结构体赋值只拷指针，不拷的话这门语言的
		// 解码会把值写进英文那一份里——英文页面开始显示中文，而且只在
		// 语言文件的加载顺序变化时才会暴露。
		t.UserErrors = maps.Clone(base.UserErrors)
		mustLoadLocale("locales/"+e.Name(), &t)
		texts[name] = t
		rest = append(rest, name)
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i] < rest[j] })
	langs = append(langs, rest...)
}

func mustLoadLocale(path string, dst *Texts) {
	b, err := localeFS.ReadFile(path)
	if err != nil {
		panic("i18n: 读不到 " + path + ": " + err.Error())
	}
	if err := json.Unmarshal(b, dst); err != nil {
		panic("i18n: " + path + " 不是合法 JSON: " + err.Error())
	}
}

// Langs 已注册的语言，英文在最前。
func Langs() []Lang { return langs }

// KnownLang 这个标记对得上某一门已注册的语言吗。
// 语言开关会把用户给的值交到这里，所以它必须是个白名单判断，
// 而不是「拿过来就往 cookie 里写」。
func KnownLang(tag string) (Lang, bool) { return matchLang(tag) }

// LangCookie 记住语言偏好的 cookie 名。
//
// 只有管理界面写它：那是登录进来长期看的地方，每开一页都回落到浏览器语言
// 会很别扭。公开页仍然只按本次请求判定——一个陌生人点一次语言开关，
// 不该在他的浏览器里留下东西。两边都【读】它：同一个人在后台选了中文，
// 公开页跟着中文才是对的。
const LangCookie = "lang"

// PickLang 决定这次请求用哪种语言。
//
// 顺序是 `?lang=` → cookie → Accept-Language → 英文。查询参数放在最前，
// 是因为公开页上那个语言开关靠它；同时它也让「想看另一种语言」这件事
// 不依赖改浏览器设置。
//
// **默认英文**：这是一个面向全球的开源项目，中文是其中一种，不是基准。
func PickLang(query, cookie, acceptLanguage string) Lang {
	if l, ok := matchLang(query); ok {
		return l
	}
	if l, ok := matchLang(cookie); ok {
		return l
	}
	// 真实的头长这样 `zh-CN,zh;q=0.9,en;q=0.8`：按出现顺序取第一个认识的。
	// 不做 q 值排序——权重最高的几乎总是排在最前，而为此引入一个解析器，
	// 带来的出错机会比它解决的问题多。
	for _, part := range strings.Split(acceptLanguage, ",") {
		tag := part
		if i := strings.Index(tag, ";"); i >= 0 {
			tag = tag[:i]
		}
		if l, ok := matchLang(tag); ok {
			return l
		}
	}
	return LangEN
}

// matchLang 把一个语言标记对到已注册的语言：先精确匹配，再退到主语言。
// 这样 ja-JP 找得到 ja.json；只有 zh.json 时，zh-CN 与 zh-Hant 都落到中文。
func matchLang(tag string) (Lang, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return "", false
	}
	if _, ok := texts[Lang(tag)]; ok {
		return Lang(tag), true
	}
	if i := strings.Index(tag, "-"); i > 0 {
		if base := Lang(tag[:i]); textsHas(base) {
			return base, true
		}
	}
	return "", false
}

func textsHas(l Lang) bool { _, ok := texts[l]; return ok }

// Attr 放进 <html lang="…">。屏幕阅读器和浏览器的翻译提示都读它。
func (l Lang) Attr() string {
	if v := T(l).HTMLLang; v != "" {
		return v
	}
	return string(l)
}

// T 取这一语的全部文案。未知语言回落英文，不 panic——
// 一个拼错的 ?lang= 不该把页面打没。
func T(l Lang) Texts {
	if t, ok := texts[l]; ok {
		return t
	}
	return texts[LangEN]
}

// Texts 页面上的每一句话。
//
// 分两部分：公共接入这条路上的字（接入页、发送说明页、错误落地页）直接摊在
// 这里，管理界面的字在 Admin 下面分页放。分开是因为两边的读者不同——
// 前者是收到链接的陌生人，后者是运行这台服务器的人。
type Texts struct {
	// 语言开关上显示的是【别的语言】的名字，所以这里写的是这门语言自己的名字
	// —— 一个中文用户看不懂英文的「Language」，反过来也一样。
	LangName string `json:"lang_name"`
	// HTMLLang 放进 <html lang="…">。留空就用文件名（en / ja），
	// 中文这类需要分简繁的才写全（zh-CN）。
	HTMLLang string `json:"html_lang"`

	// UserErrors 用户会读到的错误，key 是 internal/uierr 里的 code。
	//
	// 用一张表而不是一个个字段：加一条错误只改 JSON，不改 Go——和「加一门语言
	// 只加一个文件」是同一条原则。代价是 code 拼错不会编译失败，所以有测试
	// 拿 uierr.All 核对这张表一条不少。
	UserErrors map[string]string `json:"user_errors"`

	// ── 接入页 ──────────────────────────────────────────────
	JoinTitle      string `json:"join_title"` // <title>
	JoinHeading    string `json:"join_heading"`
	JoinLede       string `json:"join_lede"`
	JoinValidFor   string `json:"join_valid_for"` // 「%d 分钟内有效 · 只能用一次」
	JoinOpenHere   string `json:"join_open_here"`
	JoinOpenHint   string `json:"join_open_hint"`
	JoinAfterScan  string `json:"join_after_scan"`
	JoinStep1      string `json:"join_step1"`
	JoinStep2      string `json:"join_step2"`
	JoinStep3      string `json:"join_step3"`
	JoinNoAccount  string `json:"join_no_account"`
	JoinHowTo      string `json:"join_how_to"`
	JoinCopy       string `json:"join_copy"`
	JoinCopied     string `json:"join_copied"`
	JoinPaired     string `json:"join_paired"`
	JoinPairedHint string `json:"join_paired_hint"`
	JoinYourURL    string `json:"join_your_u_r_l"`
	JoinYourURLSub string `json:"join_your_u_r_l_sub"`
	JoinOpenSend   string `json:"join_open_send"`
	JoinKeepSecret string `json:"join_keep_secret"`
	JoinAddDevice  string `json:"join_add_device"`
	// JoinQRAlt 二维码的无障碍标签。它读给屏幕阅读器，和界面上其余的字一样要翻。
	JoinQRAlt string `json:"join_qr_alt"`

	// ── 发送说明页 ──────────────────────────────────────────
	GuideDocsTitle    string `json:"guide_docs_title"` // /docs 形态的大标题，会接上服务器名
	GuideDocsLede     string `json:"guide_docs_lede"`
	GuideChannelLede  string `json:"guide_channel_lede"`
	GuideYourURL      string `json:"guide_your_u_r_l"`
	GuideYourURLSub   string `json:"guide_your_u_r_l_sub"`
	GuideSampleURL    string `json:"guide_sample_u_r_l"`
	GuideSampleURLSub string `json:"guide_sample_u_r_l_sub"`
	GuideGoJoin       string `json:"guide_go_join"`
	GuidePlaceholder  string `json:"guide_placeholder"` // 发送地址里的 <你的TOKEN>
	GuideHowToJoin    string `json:"guide_how_to_join"`
	GuideInstallH     string `json:"guide_install_h"`
	GuideInstallP     string `json:"guide_install_p"`
	GuideChannelH     string `json:"guide_channel_h"`
	GuideChannelP     string `json:"guide_channel_p"`
	GuideHowToSend    string `json:"guide_how_to_send"`
	GuideAgentH       string `json:"guide_agent_h"`
	GuideAgentP       string `json:"guide_agent_p"`
	GuidePromptH      string `json:"guide_prompt_h"`
	GuideMCPH         string `json:"guide_m_c_p_h"`
	GuideMCPP         string `json:"guide_m_c_p_p"`
	GuideMCPTabAsk    string `json:"guide_m_c_p_tab_ask"` // 「发给 AI 助手」那一页
	GuideMCPTabJSON   string `json:"guide_m_c_p_tab_j_s_o_n"`
	GuideMCPTabCLI    string `json:"guide_m_c_p_tab_c_l_i"`
	GuideMCPAsk       string `json:"guide_m_c_p_ask"` // 发给 AI 助手的那段话，地址已经填好
	GuideOtherH       string `json:"guide_other_h"`
	GuideLimitsH      string `json:"guide_limits_h"`
	GuideNoLimits     string `json:"guide_no_limits"`
	GuideLimitCh      string `json:"guide_limit_ch"`
	GuideLimitDay     string `json:"guide_limit_day"`
	GuideLimitKeep    string `json:"guide_limit_keep"`
	GuideLimit429     string `json:"guide_limit429"`
	GuideSelfHostH    string `json:"guide_self_host_h"`
	GuideSelfHostP    string `json:"guide_self_host_p"`
	GuideTokenWarn    string `json:"guide_token_warn"`
	GuideSend         string `json:"guide_send"`
	GuideSending      string `json:"guide_sending"`
	GuideSent         string `json:"guide_sent"`
	GuideSendFail     string `json:"guide_send_fail"`
	GuideCopy         string `json:"guide_copy"`
	GuideCopied       string `json:"guide_copied"`

	// ── 错误与提示 ──────────────────────────────────────────
	ErrBadLinkTitle  string `json:"err_bad_link_title"`
	ErrBadLinkDetail string `json:"err_bad_link_detail"`
	ErrGone          string `json:"err_gone"`
	ErrNoSample      string `json:"err_no_sample"`
	ErrJoinTooFast   string `json:"err_join_too_fast"`
	ErrIssueFailed   string `json:"err_issue_failed"`
	ErrQRFailed      string `json:"err_q_r_failed"`

	// Admin 管理界面（/login 与 /admin/*）。嵌套而不是摊平：它有两百多条，
	// 摊进来会把上面这张表淹掉。嵌套结构体在 json.Unmarshal 时是逐字段覆盖，
	// 所以「先拷一份英文再往上盖」的回落照样成立，不必像 UserErrors 那样手动 Clone。
	Admin AdminTexts `json:"admin"`
}

// UserError 把一个错误 code 渲染成这一语的句子。
//
// 找不到就原样返回 code：那是一个能被搜索、能被报告的字符串，
// 比给人看一片空白强，也让「语料漏了一条」在第一次出现时就被认出来。
func UserError(l Lang, code string, args ...any) string {
	f, ok := T(l).UserErrors[code]
	if !ok || f == "" {
		return code
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}

// LangSwitch 页面右上角那个开关，列出除当前语言之外的每一种。
// 显示的是语言自己的名字——一个中文用户看不懂英文的「Language」，
// 反过来也一样，而语言自己的名字两边都认得。
func LangSwitch(cur Lang) string {
	var b strings.Builder
	b.WriteString(`<nav class="langsw">`)
	for _, l := range langs {
		if l == cur {
			continue
		}
		b.WriteString(`<a href="?lang=` + string(l) + `" hreflang="` + l.Attr() + `">` +
			E(T(l).LangName) + `</a>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// AdminLangSwitch 管理界面顶栏里的语言开关。
//
// 和公开页那个不一样：它指向 /lang，由服务端落一个 cookie 再跳回来。
// 后台是登录进来一待就是几十页的地方，语言得记住；公开页只看一次，不留痕迹。
// next 是切换后回到的地址，服务端会校验它只能是本站的相对路径。
func AdminLangSwitch(cur Lang, next string) string {
	return langSwitchTo(cur, next, "langsw-admin", "btn ghost sm")
}

// LoginLangSwitch 登录页的语言开关。
//
// 链接和后台一样会落 cookie —— 在这里切了语言，登录进去的后台就是那门语言，
// 否则一进门又跳回浏览器语言，等于白切。样式借公开页那套固定在右上角：
// 登录页没有顶栏可以挂。
func LoginLangSwitch(cur Lang) string {
	return langSwitchTo(cur, "/login", "langsw", "")
}

func langSwitchTo(cur Lang, next, wrapClass, btnClass string) string {
	var b strings.Builder
	b.WriteString(`<nav class="` + wrapClass + `">`)
	for _, l := range langs {
		if l == cur {
			continue
		}
		cls := ""
		if btnClass != "" {
			cls = ` class="` + btnClass + `"`
		}
		b.WriteString(`<a` + cls + ` href="/lang?to=` + string(l) +
			`&next=` + url.QueryEscape(next) + `" hreflang="` + l.Attr() + `">` +
			E(T(l).LangName) + `</a>`)
	}
	b.WriteString(`</nav>`)
	return b.String()
}

// LangCSS 开关的样式。固定在右上角，不参与各页自己的栅格。
const LangCSS = `
.langsw{position:fixed;top:14px;right:16px;z-index:9;display:flex;gap:6px}
.langsw a{font-size:12.5px;padding:5px 10px;
  border:1px solid var(--line);border-radius:999px;background:var(--card);
  color:var(--muted-fg);text-decoration:none}
.langsw a:hover{color:var(--fg);border-color:var(--fg)}
`

// AdminLangCSS 后台顶栏那个开关的样式。它跟在退出登录旁边，
// 用同一套 btn ghost sm，所以这里只管间距。
const AdminLangCSS = `
.langsw-admin{display:inline-flex;gap:6px;margin-right:8px}
`

// Plural 按数量选一条，再填进数字。
//
// 英语把 1 和其余分开，中文不分——中文那两条写成一样的就行。
// 这是个刻意简化的规则：够用于英语和中文，而不够的语言（俄语的
// 少数/复数、阿拉伯语的六档）要的是完整的 CLDR 复数规则，那时再换。
// 在此之前，写一个假装通用的实现只会让人以为它已经通用了。
func Plural(n int, one, other string) string {
	if n == 1 {
		return fmt.Sprintf(one, n)
	}
	return fmt.Sprintf(other, n)
}
