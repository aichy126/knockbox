package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
//
// 入口从表单页换成了 JSON 接口（界面改成 SPA），要盯的行为一条没变。

// adminWith 建一个管理员 + 一个收件人，返回登录 cookie 与收件人 id。
func adminWith(t *testing.T, member string) (*Server, http.Handler, string, int64) {
	t.Helper()
	s, r := newServerWith(t, false, 0, nil)
	if _, err := service.NewAccount(s.DAO).Create("admin", "adminpassword1", models.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/api/login",
		strings.NewReader(`{"username":"admin","password":"adminpassword1"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	cookie := w.Header().Get("Set-Cookie")
	if cookie == "" {
		t.Fatalf("管理员登不进去：%d %s", w.Code, w.Body.String())
	}
	u, err := service.NewAdmin(s.DAO).CreateMember(member)
	if err != nil {
		t.Fatal(err)
	}
	return s, r, cookie, u.Id
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

	_, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/devices/"+itoa(dev.Id)+"/revoke", cookie, "")
	if e.Code != 0 {
		t.Fatalf("注销别人的设备失败了：%s", e.Msg)
	}

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
	// 回的是属主：前端据此刷新那一页，不用自己记从哪来。
	var out struct {
		Owner int64 `json:"owner"`
	}
	_ = json.Unmarshal(e.Data, &out)
	if out.Owner != member {
		t.Errorf("owner 应当是 %d，得到 %d", member, out.Owner)
	}
}

// 注销一个不存在的设备要说出来，不能装作成功。
func TestAdminRevokeUnknownDeviceReportsIt(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	_, e, code := apiSend(t, r, http.MethodPost, "/admin/api/devices/999999/revoke", cookie, "")
	if e.Code == 0 {
		t.Fatal("注销一个不存在的设备报了成功")
	}
	// 带上 code 而不只是一句话：前端据此决定刷新哪一页（可能别人已经注销过了）。
	if code != "device.not_found" {
		t.Errorf("error_code 应当是 device.not_found，得到 %q", code)
	}
}

// 打开别人的消息。列表刻意跨全站（管理界面就是要看到这台服务器上的全部消息），
// 详情却只认自己的，于是从列表点进去必然落到「这条消息不存在」——一条死链。
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

	// 列表确实把它列出来了——这正是详情必须打得开的理由
	_, list, _ := apiGet(t, r, "/admin/api/messages", cookie)
	if !strings.Contains(string(list.Data), out.UID) {
		t.Fatal("消息列表没有列出别人的消息，前提不成立")
	}

	_, e, code := apiGet(t, r, "/admin/api/messages/"+out.UID, cookie)
	if e.Code != 0 {
		t.Fatalf("打开别人的消息失败了：%s（%s）", e.Msg, code)
	}
	if !strings.Contains(string(e.Data), "别人的消息") {
		t.Error("详情里没有这条消息的标题")
	}
}

// 配额豁免打在一个不存在的成员上。UPDATE 影响 0 行不是成功：
// 成员可能已经被删了，或者这是一个开着的旧标签页。
func TestUnlimitedRejectsUnknownMember(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	_, e, code := apiSend(t, r, http.MethodPost, "/admin/api/members/999999/unlimited", cookie, `{"on":true}`)
	if e.Code == 0 {
		t.Fatal("给一个不存在的成员设豁免报了成功")
	}
	if code != "member.not_found" {
		t.Errorf("error_code 应当是 member.not_found，得到 %q", code)
	}
}

// 豁免开关本身要真的往返。
func TestUnlimitedRoundTrips(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	set := func(on bool) int {
		body := `{"on":false}`
		if on {
			body = `{"on":true}`
		}
		if _, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/members/"+itoa(member)+"/unlimited", cookie, body); e.Code != 0 {
			t.Fatalf("设豁免失败：%s", e.Msg)
		}
		var u models.User
		if _, err := s.DAO.Engine().ID(member).Get(&u); err != nil {
			t.Fatal(err)
		}
		return u.Unlimited
	}
	if got := set(true); got != 1 {
		t.Errorf("开启后应当是 1，得到 %d", got)
	}
	if got := set(false); got != 0 {
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
