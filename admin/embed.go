// Package admin 把构建好的管理界面（admin/dist）嵌进二进制。
//
// 交付物没有变：仍然是一个二进制、一个数据库文件。docker pull 或下载二进制的人
// 看不出任何区别；从源码编译的人多一步 `make admin`（npm ci && npm run build），
// 那是贡献者的正常成本，不是使用者的。
package admin

import (
	"embed"
	"io/fs"
)

// dist 前端产物。
//
// `all:` 前缀是为了把 .gitkeep 也收进来：go:embed 在目录不存在时是编译错误，
// 会连锁打挂 CodeQL 的 autobuild、go install，以及任何只想编 Go 的贡献者。
// 所以 dist/.gitkeep 必须入库——没构建过时目录里只有它，服务端据此回 503
// 说「前端没构建」，而不是编不过。
//
//go:embed all:dist
var dist embed.FS

// FS 产物根目录：index.html 与 assets/。
func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic("admin: dist is not embedded: " + err.Error())
	}
	return sub
}
