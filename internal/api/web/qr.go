// Package web 服务端直出的管理页面。
//
// 二维码在服务端生成，当数据下发：管理界面因此不需要任何外部脚本，
// 公开的接入页更是连 JavaScript 都不用。
// 样式是内联的 CSS 令牌，与设计稿（docs/design/knockbox-admin）同一套。
package web

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// QRSVG 把内容画成内联 SVG。
//
// 服务端生成而不是让浏览器用 JS 画：这一页在没有外网、CSP 很严的内网也要能用，
// 而且二维码是这一页存在的唯一理由，不能依赖任何会加载失败的东西。
// label 是给屏幕阅读器的，由调用方按请求语言传进来——写死在这里的话，
// 英文页面上的二维码会被读成中文。
func QRSVG(text string, px int, label string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	n := code.Size
	var d strings.Builder
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&d, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	// ⚠️ 颜色写死黑白，**不要用 currentColor**。
	// 二维码的可扫描性依赖「深色码 + 浅色底」这个物理事实，它不该跟着主题走——
	// 深色模式下 currentColor 是白的，画在白底上就是白底白码，肉眼看得见轮廓、
	// 相机完全扫不出来。自带白底也是为了这个：页面底色再怎么变都不影响它。
	return fmt.Sprintf(
		`<svg viewBox="-2 -2 %d %d" width="%d" height="%d" shape-rendering="crispEdges" `+
			`xmlns="http://www.w3.org/2000/svg" role="img" aria-label="`+E(label)+`">`+
			`<rect x="-2" y="-2" width="%d" height="%d" fill="#fff"/>`+
			`<path fill="#000" d="%s"/></svg>`,
		n+4, n+4, px, px, n+4, n+4, d.String()), nil
}
