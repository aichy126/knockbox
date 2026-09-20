package web

import (
	"embed"
	"net/http"
	"strings"
)

// 图标进二进制。这是整个服务端唯一嵌进去的图片：没有它，每一页的标签页
// 都是空白的，而浏览器还会为此去请求一次 /favicon.ico 再拿到 404。
//
// 两个尺寸就够：32 给标签页，180 给「添加到主屏幕」。更大的没有用武之地——
// 这几页不是要被人收藏成书签墙的。
//
//go:embed assets/favicon.png assets/apple-touch-icon.png
var assetFS embed.FS

// IconLinks 放进每一页的 <head>。
//
// 写成常量而不是让每个模板自己拼：五处模板各写一遍的结果，
// 是加一个尺寸时永远漏掉其中一处。
const IconLinks = `<link rel="icon" type="image/png" href="/favicon.png">` +
	`<link rel="apple-touch-icon" href="/apple-touch-icon.png">`

// Asset 按名字取一个内嵌资源，返回内容与它的 Content-Type。
// 取不到就是 nil——调用方据此回 404，而不是给浏览器一个 0 字节的图片。
func Asset(name string) ([]byte, string) {
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return nil, ""
	}
	b, err := assetFS.ReadFile("assets/" + name)
	if err != nil {
		return nil, ""
	}
	return b, http.DetectContentType(b)
}
