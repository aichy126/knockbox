package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "github.com/aichy126/igo/db"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/migrate"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
	"xorm.io/xorm"
)

func newJoinServer(t *testing.T, public bool, registerPerHour int) (*Server, *gin.Engine) {
	t.Helper()
	return newServerWith(t, public, registerPerHour, nil)
}

// newServerWith tune 在 Router 装配【之前】跑，用于改那些只在启动时读一次的值
// （限流上限就是在 Router 里按 PairPerMin 构造的）。
func newServerWith(t *testing.T, public bool, registerPerHour int, tune func(*Server)) (*Server, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dir := t.TempDir()
	e, err := xorm.NewEngine("sqlite3", filepath.Join(dir, "t.db")+"?"+migrate.DSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	if err := migrate.Run(e); err != nil {
		t.Fatal(err)
	}
	d := dao.New(e)
	s := &Server{
		Name:        "test",
		ExternalURL: "http://localhost:8080",
		DAO:         d,
		Settings: service.NewSettings(d, service.Defaults{
			PublicEnabled: public, RegisterPerHour: registerPerHour, SiteName: "Knockbox",
		}),
		Files: service.NewFile(d, filepath.Join(dir, "blobs"), 4, 1600, 600, 70, "http://x", 3600),
	}
	if tune != nil {
		tune(s)
	}
	r := gin.New()
	Router(r, s)
	return s, r
}

func pairCodeCount(t *testing.T, s *Server) int64 {
	t.Helper()
	var n int64
	if _, err := s.DAO.Engine().SQL("SELECT COUNT(*) FROM pair_code").Get(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func get(r *gin.Engine, path string, cookie *http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func joinCookie(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == joinCodeCookie {
			return c
		}
	}
	return nil
}

// 接入页刷新一次就签一张新码的话，pair_code 的行数等于首页被打开的次数，
// 而其中绝大多数永远不会被用到。刷新应当继续用上一张。
func TestJoinPageReusesCodeAcrossReloads(t *testing.T) {
	s, r := newJoinServer(t, true, 100)

	first := get(r, "/", nil)
	if first.Code != http.StatusOK {
		t.Fatalf("接入页应当返回 200，得到 %d", first.Code)
	}
	ck := joinCookie(first)
	if ck == nil || ck.Value == "" {
		t.Fatal("首次打开应当把这次用的配对码写进 cookie")
	}
	if !ck.HttpOnly {
		t.Error("这个 cookie 不需要给 JS 读")
	}
	if n := pairCodeCount(t, s); n != 1 {
		t.Fatalf("首次打开应当签发 1 张码，得到 %d", n)
	}

	for i := 0; i < 5; i++ {
		w := get(r, "/", ck)
		if w.Code != http.StatusOK {
			t.Fatalf("第 %d 次刷新返回 %d", i+2, w.Code)
		}
	}
	if n := pairCodeCount(t, s); n != 1 {
		t.Fatalf("带着 cookie 刷新 5 次之后配对码应当还是 1 张，得到 %d", n)
	}
}

// 码被用掉之后，再打开页面必须换一张新的——否则第二个人扫到的是一张废码。
func TestJoinPageIssuesNewCodeOnceUsed(t *testing.T) {
	s, r := newJoinServer(t, true, 100)
	first := get(r, "/", nil)
	ck := joinCookie(first)
	if ck == nil {
		t.Fatal("没拿到 cookie")
	}
	if _, err := s.DAO.Engine().Exec(
		"UPDATE pair_code SET used_at = 1 WHERE code = ?", ck.Value); err != nil {
		t.Fatal(err)
	}

	w := get(r, "/", ck)
	if w.Code != http.StatusOK {
		t.Fatalf("返回 %d", w.Code)
	}
	if n := pairCodeCount(t, s); n != 2 {
		t.Fatalf("原来那张已被用掉，应当另签一张，pair_code 现在 %d 行", n)
	}
	if got := joinCookie(w); got == nil || got.Value == ck.Value {
		t.Error("cookie 没有换成新签的那张码")
	}
}

// 限流挂在【签发】那一步。没有 cookie 的访问（爬虫）才会反复签发，
// 超过上限要被挡住，而且不能留下新的行。
func TestJoinPageLimitsCodeIssuance(t *testing.T) {
	s, r := newJoinServer(t, true, 2)

	for i := 0; i < 2; i++ {
		if w := get(r, "/", nil); w.Code != http.StatusOK {
			t.Fatalf("第 %d 次应当放行，得到 %d", i+1, w.Code)
		}
	}
	w := get(r, "/", nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("超过上限应当返回 429，得到 %d", w.Code)
	}
	if n := pairCodeCount(t, s); n != 2 {
		t.Fatalf("被挡下的请求不该签发配对码，pair_code 现在 %d 行", n)
	}
}

// 公共模式关闭时，陌生人既不该看到接入页，也不该问得到配对状态。
//
// 这两条路由现在常驻，判断放在处理函数里：公共模式能在后台开关，
// 而路由只在启动时注册一次——按启动时的配置决定挂不挂的话，
// 后台打开公共模式之后接入页会去轮询一个并不存在的 /join/status。
func TestJoinClosedWhenPublicDisabled(t *testing.T) {
	s, r := newJoinServer(t, false, 100)

	for _, p := range []string{"/", "/join"} {
		w := get(r, p, nil)
		if w.Code != http.StatusFound {
			t.Errorf("%s 在自建模式下应当跳登录页，得到 %d", p, w.Code)
		}
		if loc := w.Header().Get("Location"); loc != "/login" {
			t.Errorf("%s 跳到了 %q", p, loc)
		}
	}
	if n := pairCodeCount(t, s); n != 0 {
		t.Errorf("自建模式下不该签发任何配对码，却有 %d 行", n)
	}

	if w := get(r, "/join/status?c=WHATEVER", nil); w.Code != http.StatusNotFound {
		t.Errorf("/join/status 在自建模式下应当 404，得到 %d", w.Code)
	}
}

// 公共模式在后台打开之后，两条路由都要立刻可用，不需要重启。
func TestJoinRoutesFollowRuntimeSetting(t *testing.T) {
	s, r := newJoinServer(t, false, 100)
	if w := get(r, "/join/status?c=X", nil); w.Code != http.StatusNotFound {
		t.Fatalf("起始状态应当是关闭，得到 %d", w.Code)
	}

	if err := s.Settings.Save(map[string]string{"public_enabled": "1"}); err != nil {
		t.Fatal(err)
	}
	if w := get(r, "/join", nil); w.Code != http.StatusOK {
		t.Errorf("后台打开公共模式后接入页应当可用，得到 %d", w.Code)
	}
	if w := get(r, "/join/status?c=X", nil); w.Code != http.StatusOK {
		t.Errorf("接入页轮询的 /join/status 必须同时可用，得到 %d", w.Code)
	}
}
