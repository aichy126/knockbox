package web

import (
	"regexp"
	"strconv"
	"strings"
)

// 命令行和 JSON 的轻量着色。
//
// 不引语法高亮库：这一页要显示的东西是我们自己写死的那几条 curl，
// 词法就那么几类（命令、开关、地址、JSON 键、字符串）。为此拖一个通用高亮器进来，
// 体积和维护面都不划算。
var codeToken = regexp.MustCompile(
	`(^curl\b)` + // 1 命令
		`|(\s-[A-Za-z]\b)` + // 2 开关
		`|(https?://[^\s"'<]+)` + // 3 地址
		`|("[A-Za-z_][\w-]*"\s*:)` + // 4 JSON 键
		`|("(?:[^"\\]|\\.)*")`, // 5 字符串
)

var codeClass = map[int]string{1: "t-cmd", 2: "t-flag", 3: "t-url", 4: "t-key", 5: "t-str"}

// highlightLine 给一行上色。逐行处理，所以正则里的 ^ 就是行首。
func highlightLine(src string) string {
	var b strings.Builder
	last := 0
	for _, m := range codeToken.FindAllStringSubmatchIndex(src, -1) {
		b.WriteString(E(src[last:m[0]]))
		cls := ""
		for g := 1; g <= 5; g++ {
			if m[2*g] >= 0 {
				cls = codeClass[g]
				break
			}
		}
		seg := src[m[0]:m[1]]
		// 开关那一组把前导空白也吃进来了，空白不着色
		lead := len(seg) - len(strings.TrimLeft(seg, " \t"))
		if lead > 0 {
			b.WriteString(E(seg[:lead]))
			seg = seg[lead:]
		}
		b.WriteString(`<span class="` + cls + `">` + E(seg) + `</span>`)
		last = m[1]
	}
	b.WriteString(E(src[last:]))
	return b.String()
}

// CodeBlock 带行号、可折行、带着色的代码块正文。
//
// 每一行是一个 flex 行：左边一个不参与换行的行号，右边的代码自己折。
// **必须折行不能横向滚**——这里最长的那几行是带 token 的完整地址，
// 横着滚的话用户得先发现它能滚，再滑到头才看得全，而他要的就是「看全」。
// 行号做成独立单元格，折下来的续行就不会再顶出一个号。
func CodeBlock(src string) string {
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	w := len(strconv.Itoa(len(lines)))
	var b strings.Builder
	b.WriteString(`<div class="snip-body" style="--ln-w:` + strconv.Itoa(w) + `ch">`)
	for i, ln := range lines {
		b.WriteString(`<div class="cl"><span class="ln">` + strconv.Itoa(i+1) + `</span>`)
		b.WriteString(`<span class="lc">` + highlightLine(ln) + `</span></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// CodeCSS 代码块的样式。
const CodeCSS = `
.t-cmd{color:#7DD3FC;font-weight:600}
.t-flag{color:#C4B5FD}
.t-url{color:#6EE7B7}
.t-key{color:#FCA5A5}
.t-str{color:#FCD34D}

.snip{margin:0 0 14px;border:1px solid var(--line);border-radius:12px;overflow:hidden;background:#0C0E13}
.snip-bar{display:flex;align-items:center;justify-content:space-between;gap:8px;
  padding:7px 8px 7px 14px;border-bottom:1px solid var(--line);background:var(--muted)}
.snip-lang{font-family:ui-monospace,Menlo,monospace;font-size:12px;color:var(--muted-fg)}
.snip-act{display:flex;gap:8px}
.snip-body{padding:12px 14px 14px;font-family:ui-monospace,Menlo,monospace;
  font-size:12.5px;line-height:1.8}
/* 一个代码块的多种写法：标签页占掉 snip-bar 左边那格语言标签的位置 */
.snip-bar.tabbed{padding-left:6px}
.snip-tabs{display:flex;flex-wrap:wrap;gap:2px;min-width:0}
.snip-tab{font:inherit;font-size:12.5px;line-height:1.45;padding:4px 9px;border:0;border-radius:8px;
  background:transparent;color:var(--muted-fg);cursor:pointer;white-space:nowrap}
.snip-tab.on{background:var(--card);color:var(--fg);font-weight:600}
/* 窄屏放不下「标签 + 复制」一行：让标签占满一行，复制按钮退到下一行右对齐。
   不这么写的话复制按钮会和第一行标签并排、剩下的标签折到它下面，看着像没对齐 */
@media (max-width:560px){
  .snip-bar.tabbed{flex-wrap:wrap;row-gap:2px}
  .snip-bar.tabbed .snip-tabs{width:100%}
  .snip-bar.tabbed .snip-act{margin-left:auto}
}
.cl{display:flex;gap:14px}
.ln{width:var(--ln-w,1ch);flex:0 0 auto;text-align:right;color:#4B5262;
  user-select:none;-webkit-user-select:none}
/* overflow-wrap:anywhere 让没有空格的长地址也能折；min-width:0 是 flex 子项能收缩的前提 */
.lc{flex:1 1 auto;min-width:0;white-space:pre-wrap;overflow-wrap:anywhere}
`
