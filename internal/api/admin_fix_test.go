package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
)

// 这一批盯的是「后台点了一下，界面说成功了，实际什么都没发生」。
//
// 三处共同的病根是同一个：这些 handler 是照着「每个人只管自己那一份」写的，
// 而管理后台的权限模型恰恰相反——middleware.AdminAuth 的包注释写着
// 「这是有意的全权限：它就是这台服务器的主人」。

// adminWith 建一个管理员 + 一个收件人，返回登录 cookie 与收件人 id。
func adminWith(t *testing.T, member string) (*Server, http.Handler, string, int64) {
	t.Helper()
	s, r := newServerWith(t, false, 0, nil)
	if _, err := service.NewAccount(s.DAO).Create("admin", "adminpassword1", models.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	form := url.Values{"username": {"admin"}, "password": {"adminpassword1"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	cookie := w.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("管理员登不进去")
	}
	u, err := service.NewAdmin(s.DAO).CreateMember(member)
	if err != nil {
		t.Fatal(err)
	}
	return s, r, cookie, u.Id
}

func adminPost(t *testing.T, r http.Handler, path, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, nil)
	req.Header.Set("Cookie", cookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func adminGet(t *testing.T, r http.Handler, path, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Cookie", cookie)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 注销别人的设备。成员详情页上就有这颗按钮，点了却什么都不会发生。
//
// 注销必须是【软登出】而不是删行，和 app 侧 DELETE /api/v1/devices/:uuid 同一条路：
//   - push_log.device_id 会悬空，那台手机的历史投递记录在界面上变成空名字
//   - DeviceAuth 专门为 DeviceLoggedOut 留的那句「这台设备被登出了，请重新配对」
//     会失效，被删掉的设备再来只拿到泛泛的 invalid token
func TestAdminRevokesAnotherMembersDevice(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")

	dev := &models.Device{
		UUID: "dev-uuid-1", UserId: member, Name: "别人的 iPhone",
		APNsToken: "sometoken", APNsEnv: "production", Status: models.DeviceLive,
	}
	if _, err := s.DAO.Engine().Insert(dev); err != nil {
		t.Fatal(err)
	}

	w := adminPost(t, r, "/admin/devices/"+itoa(dev.Id)+"/delete", cookie)

	var got models.Device
	ok, err := s.DAO.Engine().ID(dev.Id).Get(&got)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("设备行被删掉了：注销应当是软登出，硬删会让 push_log 的历史记录失去设备名")
	}
	if got.Status != models.DeviceLoggedOut {
		t.Errorf("status 应当是 DeviceLoggedOut(%d)，得到 %d", models.DeviceLoggedOut, got.Status)
	}
	if got.APNsToken != "" {
		t.Errorf("apns_token 应当被清空，得到 %q", got.APNsToken)
	}
	// 点「注销」的地方是成员详情页，回去的也该是那一页。
	// 原来跳的 /admin/devices 是一条从未注册过的路由，结果是 404。
	if want := "/admin/users/" + itoa(member); w.Header().Get("Location") != want {
		t.Errorf("应当回到 %s，得到 %q", want, w.Header().Get("Location"))
	}
}

// 注销一个不存在的设备要说出来，不能装作成功。
func TestAdminRevokeUnknownDeviceReportsIt(t *testing.T) {
	_, r, cookie, member := adminWith(t, "别人")
	w := adminPost(t, r, "/admin/devices/999999/delete", cookie)
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "dev=notfound") {
		t.Fatalf("应当带上 dev=notfound，得到 %q", loc)
	}
	// 光有 query 参数不算数：跳回去的那一页必须真的把这句话画出来，
	// 否则只是把「悄悄什么都没做」换成了「悄悄跳回列表」。
	body := adminGet(t, r, "/admin/users/"+itoa(member)+"?dev=notfound", cookie).Body.String()
	if !strings.Contains(body, "no longer there") {
		t.Error("成员详情页没有显示「这台设备已经不在了」")
	}
}

// 打开别人的消息。列表页刻意跨全站（管理界面就是要看到这台服务器上的全部消息），
// 详情页却只认自己的，于是从列表点进去必然落到「这条消息不存在」——一条死链。
func TestAdminReadsAnotherMembersMessage(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")

	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	out, err := service.NewSend(s.DAO).Deliver(ch, service.SendInput{Title: "别人的消息"}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}

	// 列表页确实把它列出来了——这正是详情页必须打得开的理由
	if body := adminGet(t, r, "/admin/messages", cookie).Body.String(); !strings.Contains(body, out.UID) {
		t.Fatal("消息搜索页没有列出别人的消息，前提不成立")
	}

	w := adminGet(t, r, "/admin/messages/"+out.UID, cookie)
	if w.Code != http.StatusOK {
		t.Fatalf("想要 200，得到 %d", w.Code)
	}
	// 断言「打开了」而不是「没看到那句错误」：这个请求没带 Accept-Language，
	// 而 reqLang 默认英文，拿中文原文去比会在一个渲染着英文错误页的响应上通过。
	body := w.Body.String()
	if !strings.Contains(body, "别人的消息") {
		t.Error("详情页没有渲染出这条消息的标题——点开别人的消息落到了「未找到」")
	}
	if strings.Contains(body, "未找到") {
		t.Error("面包屑停在「未找到」")
	}
}

// 配额豁免打在一个不存在的成员上。UPDATE 影响 0 行不是成功：
// 成员可能已经被删了，或者这是一个开着的旧标签页。
func TestUnlimitedRejectsUnknownMember(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	req := httptest.NewRequest(http.MethodPost, "/admin/users/999999/unlimited",
		strings.NewReader("on=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookie)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if loc := w.Header().Get("Location"); !strings.Contains(loc, "member=notfound") {
		t.Fatalf("应当带上 member=notfound，得到 %q", loc)
	}
	body := adminGet(t, r, "/admin/users?member=notfound", cookie).Body.String()
	if !strings.Contains(body, "no longer exists") {
		t.Error("成员列表页没有显示「这个成员已经不存在了」")
	}
}

// 豁免开关本身要真的往返。
func TestUnlimitedRoundTrips(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	set := func(on string) int {
		req := httptest.NewRequest(http.MethodPost, "/admin/users/"+itoa(member)+"/unlimited",
			strings.NewReader("on="+on))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Cookie", cookie)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		var u models.User
		if _, err := s.DAO.Engine().ID(member).Get(&u); err != nil {
			t.Fatal(err)
		}
		return u.Unlimited
	}
	if got := set("1"); got != 1 {
		t.Errorf("开启后应当是 1，得到 %d", got)
	}
	if got := set("0"); got != 0 {
		t.Errorf("关闭后应当是 0，得到 %d", got)
	}
}

// DB 真的出错时不能装作成功。
//
// `_, _ = Exec(...)` 只在磁盘或连接层面才会出错，没有 mock 的测试栈里唯一能真正
// 制造它的手段就是把引擎关掉。这一条落在 service 层而不是走 HTTP：AdminAuth 每个
// 请求都要读库验会话，引擎一关，请求在中间件就被挡下，压根到不了这个 handler——
// 那样测的就不是「写操作吞错」而是「会话查不动」了。
func TestUnlimitedSurfacesDBFailure(t *testing.T) {
	s, _ := newServerWith(t, false, 0, nil)
	u, err := service.NewAdmin(s.DAO).CreateMember("别人")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.DAO.Engine().Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.NewAdmin(s.DAO).SetUnlimited(u.Id, true); err == nil {
		t.Error("库都关了，SetUnlimited 还报成功")
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
