package web

import (
	"bytes"
	"sync"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

var (
	mdOnce sync.Once
	md     goldmark.Markdown
)

// Markdown 把消息正文渲染成 HTML。
//
// ⚠️ **绝不开 WithUnsafe()**。消息正文是外部输入——任何能往频道发消息的人
// 都能写正文，开了它就等于允许任意 HTML 注入后台页面。goldmark 默认会把裸 HTML
// 当文本转义掉，这正是我们要的：管理员看到的是发送方写了什么，
// 而不是发送方想让浏览器做什么。
//
// 代码块只排版不高亮：高亮要引 chroma，而后台是用来核对内容的，
// 不是阅读器。app 那边才需要高亮。
func Markdown(src string) string {
	mdOnce.Do(func() {
		md = goldmark.New(
			goldmark.WithExtensions(
				extension.GFM, // 表格、删除线、任务列表、自动链接
			),
			goldmark.WithRendererOptions(
				gmhtml.WithHardWraps(), // 告警正文里的换行就是换行，不该被合并成一段
			),
		)
	})
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		// 渲染失败就退回纯文本，不能让一条畸形消息把整页打没
		return `<pre class="body">` + E(src) + `</pre>`
	}
	return `<div class="md">` + buf.String() + `</div>`
}
