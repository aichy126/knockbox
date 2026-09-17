package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
)

func apiSend(t *testing.T, r http.Handler, method, path, cookie, body string) (int, envelope, string) {
	t.Helper()
	var rd *strings.Reader
	if body == "" {
		rd = strings.NewReader("")
	} else {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var e envelope
	_ = json.Unmarshal(w.Body.Bytes(), &e)
	var d struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.Unmarshal(e.Data, &d)
	return w.Code, e, d.ErrorCode
}

// 改密码的三种拒绝各自带自己的 code——要重填的不是同一个输入框。
//
// 每一种拒绝之后【原密码必须还能用】。「报了个错，密码却已经变了」
// 是这里最坏的失败，而它从界面上完全看不出来。
func TestAPIPasswordChangeErrorCodes(t *testing.T) {
	s, r, cookie, _ := adminWith(t, "别人")
	acc := service.NewAccount(s.DAO)

	for _, tc := range []struct{ name, body, want string }{
		{"当前密码不对", `{"current":"wrongpassword","new":"newpassword1","confirm":"newpassword1"}`, "password.wrong"},
		{"两次输入不一致", `{"current":"adminpassword1","new":"newpassword1","confirm":"newpassword2"}`, "password.mismatch"},
		{"新密码太短", `{"current":"adminpassword1","new":"short1","confirm":"short1"}`, "password.weak"},
		// 当前密码错 + 两次输入也不一致：报的必须是 password.wrong。
		//
		// 这一条钉的是【顺序】：先验当前密码，再比两次输入。理由不是「哪个更
		// 重要」，是安全——会话 cookie 被偷走的人不该能拿这个接口试探，
		// 先回 mismatch 等于告诉他「当前密码那栏你蒙对了」。
		{"都不对时先报当前密码", `{"current":"wrongpassword","new":"newpassword1","confirm":"newpassword2"}`, "password.wrong"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e, code := apiSend(t, r, http.MethodPost, "/admin/api/account/password", cookie, tc.body)
			if e.Code != 1 || code != tc.want {
				t.Errorf("应当是 %s，得到 code=%d error_code=%q", tc.want, e.Code, code)
			}
			if _, err := acc.Verify("admin", "adminpassword1"); err != nil {
				t.Fatalf("被拒之后原密码应当仍然有效：%v", err)
			}
		})
	}
}

// 改成功之后旧会话立刻失效——SetPassword 清掉这个账号的全部会话。
func TestAPIPasswordChangeInvalidatesSessions(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	_, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/account/password", cookie,
		`{"current":"adminpassword1","new":"newpassword1","confirm":"newpassword1"}`)
	if e.Code != 0 {
		t.Fatalf("改密码应当成功，得到 %+v", e)
	}
	var d struct {
		SignedOut bool `json:"signed_out"`
	}
	if err := json.Unmarshal(e.Data, &d); err != nil || !d.SignedOut {
		t.Errorf("应当回报 signed_out=true，得到 %s", e.Data)
	}
	if code, _, _ := apiGet(t, r, "/admin/api/me", cookie); code != http.StatusUnauthorized {
		t.Errorf("旧 cookie 应当已经失效，得到 %d", code)
	}
}

// 登出之后当前会话作废。
func TestAPILogoutEndsSession(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	if _, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/logout", cookie, ""); e.Code != 0 {
		t.Fatalf("登出应当成功，得到 %+v", e)
	}
	if code, _, _ := apiGet(t, r, "/admin/api/me", cookie); code != http.StatusUnauthorized {
		t.Errorf("登出之后 cookie 应当失效，得到 %d", code)
	}
}

// 设置里的非法值要挡住，【并且一个字都不能写进去】。
func TestAPISettingsRejectsBadValue(t *testing.T) {
	s, r, cookie, _ := adminWith(t, "别人")
	before := s.Settings.MaxPerDay()

	for _, tc := range []struct{ name, body, field string }{
		{"负数", `{"max_per_day":-1}`, "max_per_day"},
		{"负的保留期", `{"retention_days":-7}`, "retention_days"},
		{"站点名过长", `{"site_name":"` + strings.Repeat("长", siteNameMax+1) + `"}`, "site_name"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e, code := apiSend(t, r, http.MethodPut, "/admin/api/settings", cookie, tc.body)
			if e.Code != 1 || code != "setting.bad_value" {
				t.Errorf("应当是 setting.bad_value，得到 code=%d error_code=%q", e.Code, code)
			}
			// 句子里要带上是哪个字段，否则用户不知道去改哪一栏
			if !strings.Contains(e.Msg, tc.field) {
				t.Errorf("提示里应当带上字段名 %s，得到 %q", tc.field, e.Msg)
			}
		})
	}
	if got := s.Settings.MaxPerDay(); got != before {
		t.Errorf("被拒之后存储值不该变：%d → %d", before, got)
	}
}

// 没提到的字段保持不变。
//
// 这是表单版「空串当没改」那条注释的意图，JSON 版能做得更准：
// 缺省和显式给值现在是两回事。
func TestAPISettingsAbsentFieldStaysPut(t *testing.T) {
	s, r, cookie, _ := adminWith(t, "别人")
	if err := s.Settings.Save(map[string]string{"max_per_day": "123"}); err != nil {
		t.Fatal(err)
	}
	_, e, _ := apiSend(t, r, http.MethodPut, "/admin/api/settings", cookie, `{"site_name":"只改这个"}`)
	if e.Code != 0 {
		t.Fatalf("应当成功，得到 %+v", e)
	}
	var d settingsData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if d.Values.MaxPerDay != 123 {
		t.Errorf("没提到的 max_per_day 应当保持 123，得到 %d", d.Values.MaxPerDay)
	}
	if d.Values.SiteName != "只改这个" {
		t.Errorf("site_name 应当已改，得到 %q", d.Values.SiteName)
	}
	// 响应体就是写后的状态，前端不用再 GET 一次
	if d.FromConfig["site_name"] {
		t.Error("改过的键 from_config 应当是 false")
	}
	if !d.FromConfig["max_channels"] {
		t.Error("没改过的键 from_config 应当还是 true")
	}
}

// 显式把一个数字设成 0 与「没提这个字段」必须是两回事。
func TestAPISettingsZeroIsNotAbsent(t *testing.T) {
	s, r, cookie, _ := adminWith(t, "别人")
	if err := s.Settings.Save(map[string]string{"max_per_day": "50"}); err != nil {
		t.Fatal(err)
	}
	if _, e, _ := apiSend(t, r, http.MethodPut, "/admin/api/settings", cookie,
		`{"max_per_day":0}`); e.Code != 0 {
		t.Fatalf("0 是合法值，得到 %+v", e)
	}
	if got := s.Settings.MaxPerDay(); got != 0 {
		t.Errorf("显式给的 0 应当写进去，得到 %d", got)
	}
}

// 清空不存在的频道要报错，不能当成功。
// 服务端直出那版是 302 到 /admin/users，JSON 客户端会把它读成成功。
func TestAPIPurgeUnknownChannel(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	_, e, code := apiSend(t, r, http.MethodPost, "/admin/api/channels/nosuch/purge", cookie, "")
	if e.Code != 1 || code != "channel.not_found" {
		t.Errorf("应当是 channel.not_found，得到 code=%d error_code=%q", e.Code, code)
	}
}

func TestAPIPurgeReturnsDeletedCount(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, err := service.NewSend(s.DAO).Deliver(ch, service.SendInput{Title: "x"}, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}
	_, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/channels/"+ch.Id+"/purge", cookie, "")
	if e.Code != 0 {
		t.Fatalf("清空应当成功，得到 %+v", e)
	}
	var d service.PurgeResult
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if d.Deleted != 5 {
		t.Errorf("应当删掉 5 条，得到 %d", d.Deleted)
	}
}

// 给已有成员签发，不能顺手多建一个人。
func TestAPIPairIssueForExistingMemberDoesNotCreateOne(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	before := pairCodeCount(t, s)
	var users int64
	if _, err := s.DAO.Engine().SQL("SELECT COUNT(*) FROM user").Get(&users); err != nil {
		t.Fatal(err)
	}

	_, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/pair", cookie,
		`{"member_id":`+itoa(member)+`}`)
	if e.Code != 0 {
		t.Fatalf("签发应当成功，得到 %+v", e)
	}
	var d pairIssued
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if d.Member.Created {
		t.Error("给已有成员签发不该报 created=true")
	}
	if d.Member.ID != member {
		t.Errorf("归属应当是 %d，得到 %d", member, d.Member.ID)
	}
	// 二维码留在服务端生成：页面因此不需要任何外部脚本
	if !strings.HasPrefix(d.QRSVG, "<svg") {
		t.Errorf("应当带一段服务端生成的 SVG，得到 %.40q", d.QRSVG)
	}
	if d.DeepLink == "" || d.Code == "" {
		t.Error("deep link 与配对码都要给")
	}
	if got := pairCodeCount(t, s); got != before+1 {
		t.Errorf("配对码应当多一个，%d → %d", before, got)
	}
	var after int64
	if _, err := s.DAO.Engine().SQL("SELECT COUNT(*) FROM user").Get(&after); err != nil {
		t.Fatal(err)
	}
	if after != users {
		t.Errorf("成员数不该变，%d → %d", users, after)
	}
}

func TestAPIPairIssueCreatesMemberByName(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	_, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/pair", cookie, `{"new_name":"新来的"}`)
	if e.Code != 0 {
		t.Fatalf("应当成功，得到 %+v", e)
	}
	var d pairIssued
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if !d.Member.Created || d.Member.Name != "新来的" {
		t.Errorf("应当顺手建一个叫「新来的」的成员，得到 %+v", d.Member)
	}
}

// 两个都不给：选一个人或起个名字，这是用户的下一步。
func TestAPIPairIssueWithoutTarget(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	_, e, code := apiSend(t, r, http.MethodPost, "/admin/api/pair", cookie, `{}`)
	if e.Code != 1 || code != "pair.no_target" {
		t.Errorf("应当是 pair.no_target，得到 code=%d error_code=%q", e.Code, code)
	}
}

// 注销设备走软登出，并回报它属于谁。
func TestAPIRevokeDevice(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	dev := &models.Device{
		UUID: "u1", UserId: member, Name: "iPhone",
		APNsToken: "tok", APNsEnv: "production", Status: models.DeviceLive,
	}
	if _, err := s.DAO.Engine().Insert(dev); err != nil {
		t.Fatal(err)
	}
	_, e, _ := apiSend(t, r, http.MethodPost, "/admin/api/devices/"+itoa(dev.Id)+"/revoke", cookie, "")
	if e.Code != 0 {
		t.Fatalf("注销应当成功，得到 %+v", e)
	}
	var got models.Device
	ok, err := s.DAO.Engine().ID(dev.Id).Get(&got)
	if err != nil || !ok {
		t.Fatalf("设备行应当还在（软登出）：ok=%v err=%v", ok, err)
	}
	if got.Status != models.DeviceLoggedOut || got.APNsToken != "" {
		t.Errorf("应当是软登出且清空 token，得到 status=%d token=%q", got.Status, got.APNsToken)
	}

	_, e, code := apiSend(t, r, http.MethodPost, "/admin/api/devices/999999/revoke", cookie, "")
	if e.Code != 1 || code != "device.not_found" {
		t.Errorf("不存在的设备应当报 device.not_found，得到 %q", code)
	}
}

// 配额豁免回写真实状态，并且不存在的成员要报出来。
func TestAPIUnlimited(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	if _, e, _ := apiSend(t, r, http.MethodPost,
		"/admin/api/members/"+itoa(member)+"/unlimited", cookie, `{"on":true}`); e.Code != 0 {
		t.Fatalf("应当成功，得到 %+v", e)
	}
	var u models.User
	if _, err := s.DAO.Engine().ID(member).Get(&u); err != nil {
		t.Fatal(err)
	}
	if u.Unlimited != 1 {
		t.Errorf("库里应当是 1，得到 %d", u.Unlimited)
	}
	_, e, code := apiSend(t, r, http.MethodPost, "/admin/api/members/999999/unlimited", cookie, `{"on":true}`)
	if e.Code != 1 || code != "member.not_found" {
		t.Errorf("应当是 member.not_found，得到 %q", code)
	}
}

// 写操作同样受会话保护，且同样不能 302。
func TestAdminAPIWritesRequireSession(t *testing.T) {
	_, r := newServerWith(t, false, 0, nil)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/admin/api/logout"},
		{http.MethodPost, "/admin/api/members/1/unlimited"},
		{http.MethodPost, "/admin/api/devices/1/revoke"},
		{http.MethodPost, "/admin/api/channels/x/purge"},
		{http.MethodPost, "/admin/api/pair"},
		{http.MethodPut, "/admin/api/settings"},
		{http.MethodPost, "/admin/api/account/password"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s：想要 401，得到 %d（Location=%q）",
				tc.method, tc.path, w.Code, w.Header().Get("Location"))
		}
	}
}
