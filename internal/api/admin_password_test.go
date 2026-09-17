package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
)

// 后台改密码。第一个管理员是服务器自动建的、密码随机，所以「在后台把它改掉」
// 是每一个自建的人都会走一次的路径。
func TestAdminPasswordChange(t *testing.T) {
	s, r := newServerWith(t, false, 0, nil)
	acc := service.NewAccount(s.DAO)
	if _, err := acc.Create("admin", "oldpassword1", models.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	login := func(pw string) string {
		form := url.Values{"username": {"admin"}, "password": {pw}}
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusFound {
			return ""
		}
		return w.Header().Get("Set-Cookie")
	}

	cookie := login("oldpassword1")
	if cookie == "" {
		t.Fatal("初始密码登不进去")
	}

	change := func(cookie, current, pw, confirm string) string {
		form := url.Values{"current": {current}, "new": {pw}, "confirm": {confirm}}
		req := httptest.NewRequest(http.MethodPost, "/admin/account/password", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", cookie)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Header().Get("Location")
	}

	// 三种拒绝。每一种之后原密码都必须还能用——「报了个错，密码却已经变了」
	// 是这里最坏的失败，而它从界面上完全看不出来。
	for _, tc := range []struct {
		name                       string
		current, pw, confirm, want string
	}{
		{"当前密码不对", "wrongpassword", "newpassword1", "newpassword1", "pwd=wrong"},
		{"两次输入不一致", "oldpassword1", "newpassword1", "newpassword2", "pwd=mismatch"},
		{"新密码太短", "oldpassword1", "short1", "short1", "pwd=weak"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := change(cookie, tc.current, tc.pw, tc.confirm)
			if !strings.Contains(got, tc.want) {
				t.Errorf("应当重定向到带 %s 的页面，得到 %q", tc.want, got)
			}
			if _, err := acc.Verify("admin", "oldpassword1"); err != nil {
				t.Fatalf("被拒之后原密码应当仍然有效: %v", err)
			}
		})
	}

	// 改成功：人被带去登录页，旧密码与旧会话同时作废。
	if got := change(cookie, "oldpassword1", "newpassword1", "newpassword1"); got != "/login?changed=1" {
		t.Fatalf("改成功后应当去 /login?changed=1，得到 %q", got)
	}
	if _, err := acc.Verify("admin", "oldpassword1"); err == nil {
		t.Error("旧密码改完还能用")
	}
	if _, err := acc.Verify("admin", "newpassword1"); err != nil {
		t.Errorf("新密码不能用: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code == http.StatusOK {
		t.Error("改完密码后旧会话还能进后台")
	}

	if login("newpassword1") == "" {
		t.Error("新密码登不进去")
	}
}
