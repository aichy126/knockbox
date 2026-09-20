package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/aichy126/knockbox/admin"
	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/gin-gonic/gin"
)

// 管理界面是一个 SPA，产物 embed 在二进制里（见 admin/embed.go）。
//
// 挂在 NoRoute 上，不是 r.GET("/admin/*path")：gin 的路由树不允许同一层既有
// catch-all 又有别的子节点，而 /admin/api/... 就在那一层——注册了会【启动即 panic】。
//
// 交付物没有变：仍然是一个二进制、一个数据库文件。

// spaRoots SPA 接管的地址。其余（/、/docs、/s/:token、/join）仍是服务端直出的
// 公开页，它们不需要 JavaScript，也不该因为后台换了实现而变。
func spaOwns(p string) bool {
	return p == "/login" || p == "/admin" || strings.HasPrefix(p, "/admin/")
}

// mountSPA 装在 NoRoute 上。顺序：接口 404 → 静态资源 → index.html → 真 404。
func mountSPA(r *gin.Engine) {
	dist := admin.FS()
	index, indexErr := fs.ReadFile(dist, "index.html")

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path

		// /admin/api/ 下【绝不能】回 HTML：fetch 会把它当成 JSON 解析然后炸掉，
		// 报出来的错和真实原因（路径写错了）毫无关系。
		if strings.HasPrefix(p, middleware.AdminAPIPrefix) {
			c.AbortWithStatusJSON(http.StatusNotFound, gin.H{"code": 1, "msg": "no such endpoint", "data": nil})
			return
		}
		if !spaOwns(p) {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}

		// 没构建过时 dist 里只有 .gitkeep。说清楚该做什么，而不是回一片空白——
		// 从源码编译的人第一次撞上的就是这里。
		if indexErr != nil {
			c.String(http.StatusServiceUnavailable,
				"The admin interface has not been built.\n\nRun `make admin` (needs Node 20+), or use a release binary or the Docker image, which ship it built.\n")
			return
		}

		// 带指纹的静态资源：vite 产出的文件名含内容 hash，可以长缓存。
		if name := strings.TrimPrefix(p, "/admin/"); name != p && name != "" {
			if serveAsset(c, dist, name) {
				return
			}
		}

		// 其余都交给前端路由。index.html 不缓存：它引用的是带 hash 的资源，
		// 缓存住了就会在升级后继续加载已经不存在的 js。
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})
}

func serveAsset(c *gin.Context, dist fs.FS, name string) bool {
	// path.Clean 挡住 ../：embed.FS 自己也会拒，但错误信息不如直接不试。
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	f, err := dist.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		return false
	}
	body, err := io.ReadAll(f)
	if err != nil {
		return false
	}
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.Data(http.StatusOK, mimeOf(name), body)
	return true
}

// mimeOf 只认产物里真会出现的几种。用 mime.TypeByExtension 的话，
// 它在某些系统上会去读 /etc/mime.types，同一个二进制在两台机器上行为不同。
func mimeOf(name string) string {
	switch path.Ext(name) {
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".json":
		return "application/json; charset=utf-8"
	case ".woff2":
		return "font/woff2"
	case ".png":
		return "image/png"
	case ".ico":
		return "image/x-icon"
	}
	return "application/octet-stream"
}

// asset 发一个内嵌的图标。图标跟着二进制走，改它要发版，
// 所以缓存可以放心地长——一年，和其它不带指纹的站点图标一样。
func (s *Server) asset(name string) gin.HandlerFunc {
	body, mime := web.Asset(name)
	return func(c *gin.Context) {
		if body == nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Data(http.StatusOK, mime, body)
	}
}
