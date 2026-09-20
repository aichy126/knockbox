package api

import (
	"net/http"
	"strings"
	"testing"
)

// 管理界面是 embed 进来的 SPA。这里盯三件在路由上容易出错的事。

// /admin 与 /login 交给前端，而且【绝不能】是 404。
//
// 没构建过时 dist 里只有 .gitkeep，服务端回 503 并说清该做什么；构建过就是
// index.html。两种都对，404 不对——那说明 NoRoute 没挂上，或者 spaOwns 漏了一条。
func TestSPAOwnsAdminAndLogin(t *testing.T) {
	_, r := newServerWith(t, false, 0, nil)
	for _, p := range []string{"/login", "/admin", "/admin/users", "/admin/users/1", "/admin/settings?tab=server"} {
		w := plainGet(t, r, p)
		switch w.Code {
		case http.StatusOK:
			if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("%s 回的不是 HTML：%q", p, ct)
			}
		case http.StatusServiceUnavailable:
			// 没构建过。这句话是从源码编译的人第一次撞上的，得说清下一步。
			if !strings.Contains(w.Body.String(), "make admin") {
				t.Errorf("%s 的 503 没说该怎么办：%q", p, w.Body.String())
			}
		default:
			t.Errorf("%s 回了 %d，既不是页面也不是「还没构建」", p, w.Code)
		}
	}
}

// /admin/api/ 下【绝不能】回 HTML。
//
// fetch 会把它当成 JSON 解析然后炸掉，报出来的错和真实原因（路径写错了）
// 毫无关系，而且每个调用点都要各自去猜。
func TestUnknownAPIPathStaysJSON(t *testing.T) {
	_, r := newServerWith(t, false, 0, nil)
	w := plainGet(t, r, "/admin/api/nope")
	if w.Code != http.StatusNotFound {
		t.Errorf("想要 404，得到 %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "json") {
		t.Errorf("Content-Type 是 %q，应当是 JSON", ct)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Error("接口路径回了一张 HTML 页")
	}
}

// 公开页不受影响：它们在 SPA 之前就被匹配掉了。
func TestPublicPagesAreNotSwallowedBySPA(t *testing.T) {
	_, r := newServerWith(t, true, 10, nil)
	for _, p := range []string{"/", "/docs", "/join"} {
		w := plainGet(t, r, p)
		if w.Code != http.StatusOK {
			t.Errorf("%s 回了 %d", p, w.Code)
		}
		// 直出的公开页不引 JS；SPA 的 index.html 会有 <script type="module">
		if strings.Contains(w.Body.String(), `type="module"`) {
			t.Errorf("%s 被 SPA 接管了", p)
		}
	}
	// 既不属于公开页也不属于后台的地址仍然是 404
	if w := plainGet(t, r, "/nope"); w.Code != http.StatusNotFound {
		t.Errorf("/nope 回了 %d，应当是 404", w.Code)
	}
}
