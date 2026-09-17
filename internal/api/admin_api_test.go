package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// envelope 拆开信封，返回 code / data / error_code。
// 不拆的话每个测试都要写一遍同样的十行。
type envelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

func apiGet(t *testing.T, r http.Handler, path, cookie string) (int, envelope, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
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

// 所有 /admin/api/* 无会话时必须是 401 JSON，【绝不是 302】。
//
// 浏览器 fetch 默认 redirect:"follow"，302 会被跟到 /login 拿回 200 的 HTML，
// 然后在 res.json() 上炸掉——报出来的错和真实原因毫无关系。
// Accept: text/html 那一路要单独跑，因为那正是浏览器自己会带上的头。
func TestAdminAPIRequiresSession(t *testing.T) {
	_, r := newServerWith(t, false, 0, nil)
	paths := []string{
		"/admin/api/me", "/admin/api/overview", "/admin/api/members",
		"/admin/api/members/1", "/admin/api/channels/x", "/admin/api/channels/x/messages",
		"/admin/api/messages", "/admin/api/messages/x",
		"/admin/api/pair/targets", "/admin/api/settings",
	}
	for _, p := range paths {
		for _, accept := range []string{"", "text/html,application/xhtml+xml", "application/json"} {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			if accept != "" {
				req.Header.Set("Accept", accept)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("%s Accept=%q：想要 401，得到 %d（Location=%q）",
					p, accept, w.Code, w.Header().Get("Location"))
				continue
			}
			var e envelope
			if err := json.Unmarshal(w.Body.Bytes(), &e); err != nil {
				t.Errorf("%s Accept=%q：响应不是 JSON", p, accept)
			}
		}
	}
}

// 成功信封恰好是这个形状：老客户端读 msg，新客户端读 data。
func TestAdminAPISuccessEnvelope(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	code, e, _ := apiGet(t, r, "/admin/api/me", cookie)
	if code != http.StatusOK {
		t.Fatalf("想要 200，得到 %d", code)
	}
	if e.Code != 0 || e.Msg != "success" {
		t.Errorf("信封不对：code=%d msg=%q", e.Code, e.Msg)
	}
	var d meData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if d.User.Name != "admin" || d.Server.Name == "" {
		t.Errorf("me 的内容不对：%+v", d)
	}
}

// 失败一律带 data.error_code——客户端按它分支，而不是去匹配句子。
func TestAdminAPIFailureCarriesErrorCode(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	for _, tc := range []struct{ path, want string }{
		{"/admin/api/members/999999", "member.not_found"},
		{"/admin/api/messages/nosuchuid", "message.not_found"},
		{"/admin/api/channels/nosuchchannel", "channel.not_found"},
		{"/admin/api/messages?member=999999", "member.not_found"},
	} {
		code, e, errCode := apiGet(t, r, tc.path, cookie)
		if code != http.StatusOK {
			t.Errorf("%s：业务失败也该是 HTTP 200，得到 %d", tc.path, code)
		}
		if e.Code != 1 {
			t.Errorf("%s：code 应当是 1，得到 %d", tc.path, e.Code)
		}
		if errCode != tc.want {
			t.Errorf("%s：error_code 应当是 %s，得到 %q", tc.path, tc.want, errCode)
		}
	}
}

// 概览是【全站】口径，不按登录的这个管理员过滤。
//
// 公共实例上管理员自己那个 uid 基本没有流量，按他过滤会让首页第一个数字
// 恒显示 0，而服务器实际推了几万条。
func TestOverviewCountsWholeServer(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := service.NewSend(s.DAO).Deliver(ch, service.SendInput{Title: "x"}, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}

	_, e, _ := apiGet(t, r, "/admin/api/overview", cookie)
	var d overviewData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	// 管理员自己一条消息都没发，这三条全是别人的
	if d.Messages.Current != 3 {
		t.Errorf("窗口内消息应当是 3（全站），得到 %d", d.Messages.Current)
	}
	if d.Channels.Total != 1 {
		t.Errorf("频道应当是 1（全站），得到 %d", d.Channels.Total)
	}
	if len(d.Recent) != 3 {
		t.Errorf("最近消息应当有 3 条，得到 %d", len(d.Recent))
	}
}

// 一次推送都没有时，成功率是 null 不是 0：
// 「成功率 0%」和「没推过」在界面上是两句不同的话。
func TestOverviewPushRateIsNullWhenNoPushes(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	_, e, _ := apiGet(t, r, "/admin/api/overview", cookie)
	if !strings.Contains(string(e.Data), `"rate":null`) {
		t.Errorf("没推过时 rate 应当是 null：%s", e.Data)
	}
}

// 游标翻页：两页不重不漏。
//
// 「不相交」那一条抓的是 id < cursor 写成 <= 的差一错——
// 那种错只会让某一条消息重复出现一次，翻页的人看不出来。
func TestMessageSearchPagesWithCursor(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	const n = 45
	for i := 0; i < n; i++ {
		if _, err := service.NewSend(s.DAO).Deliver(ch,
			service.SendInput{Title: fmt.Sprintf("第 %d 条", i)}, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}

	page := func(cursor string) List[messageJSON] {
		p := "/admin/api/messages?limit=40"
		if cursor != "" {
			p += "&cursor=" + cursor
		}
		_, e, _ := apiGet(t, r, p, cookie)
		var l List[messageJSON]
		if err := json.Unmarshal(e.Data, &l); err != nil {
			t.Fatal(err)
		}
		return l
	}

	first := page("")
	if len(first.Items) != 40 || !first.HasMore || first.NextCursor == "" {
		t.Fatalf("第一页应当是 40 条 + has_more + 游标，得到 %d/%v/%q",
			len(first.Items), first.HasMore, first.NextCursor)
	}
	second := page(first.NextCursor)
	if len(second.Items) != 5 || second.HasMore {
		t.Fatalf("第二页应当是 5 条且没有更多，得到 %d/%v", len(second.Items), second.HasMore)
	}

	seen := map[string]bool{}
	for _, m := range first.Items {
		seen[m.UID] = true
	}
	for _, m := range second.Items {
		if seen[m.UID] {
			t.Errorf("两页重复出现了 %s", m.UID)
		}
		seen[m.UID] = true
	}
	if len(seen) != n {
		t.Errorf("两页合起来应当是 %d 条，得到 %d", n, len(seen))
	}
}

// 游标分页在「翻页时有人注册」这件事上不丢行，offset 分页会。
func TestMemberCursorSurvivesInsertion(t *testing.T) {
	s, r, cookie, _ := adminWith(t, "aaa")
	for _, n := range []string{"bbb", "ccc", "ddd"} {
		if _, err := service.NewAdmin(s.DAO).CreateMember(n); err != nil {
			t.Fatal(err)
		}
	}

	page := func(cursor string) List[memberJSON] {
		p := "/admin/api/members?limit=2"
		if cursor != "" {
			p += "&cursor=" + cursor
		}
		_, e, _ := apiGet(t, r, p, cookie)
		var l List[memberJSON]
		if err := json.Unmarshal(e.Data, &l); err != nil {
			t.Fatal(err)
		}
		return l
	}

	first := page("")
	if len(first.Items) != 2 {
		t.Fatalf("第一页应当 2 行，得到 %d", len(first.Items))
	}
	// 两页之间插一个新成员——offset 分页会因此跳过一行
	if _, err := service.NewAdmin(s.DAO).CreateMember("插进来的"); err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	for _, m := range first.Items {
		seen[m.ID] = true
	}
	// 循环要有上界：游标如果不前进（比如退回 offset 分页），
	// 无界的 for 会一直转下去，测试表现成挂起而不是失败——
	// 一个测不出来的测试比没有测试更糟。
	cursor := first.NextCursor
	for i := 0; cursor != ""; i++ {
		if i > 10 {
			t.Fatalf("翻了 10 页还没到头，游标没有前进：%q", cursor)
		}
		p := page(cursor)
		for _, m := range p.Items {
			if seen[m.ID] {
				t.Errorf("成员 %d 出现了两次", m.ID)
			}
			seen[m.ID] = true
		}
		cursor = p.NextCursor
	}
	// admin + aaa + bbb + ccc + ddd + 插进来的 = 6，一个都不能少
	if len(seen) != 6 {
		t.Errorf("翻完应当见到 6 个成员，得到 %d", len(seen))
	}
}

// 频道消息流能翻到第 40 条以外。
// 服务端直出的那一版固定 40 条、没有任何往前翻的办法，这是被补上的功能缺口。
func TestChannelStreamReachesBeyondForty(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	const n = 60
	for i := 0; i < n; i++ {
		if _, err := service.NewSend(s.DAO).Deliver(ch,
			service.SendInput{Title: fmt.Sprintf("第 %d 条", i)}, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}

	seen, cursor := map[string]bool{}, ""
	for i := 0; i < 10; i++ {
		p := "/admin/api/channels/" + ch.Id + "/messages?limit=40"
		if cursor != "" {
			p += "&cursor=" + cursor
		}
		_, e, _ := apiGet(t, r, p, cookie)
		var l List[messageJSON]
		if err := json.Unmarshal(e.Data, &l); err != nil {
			t.Fatal(err)
		}
		// 返回的是时间正序，和 app 一致
		for j := 1; j < len(l.Items); j++ {
			if l.Items[j-1].ID > l.Items[j].ID {
				t.Fatal("频道流应当按时间正序返回")
			}
		}
		for _, m := range l.Items {
			seen[m.UID] = true
		}
		if cursor = l.NextCursor; cursor == "" {
			break
		}
		if i == 9 {
			t.Fatal("翻了 10 页还没到头，游标没有前进")
		}
	}
	if len(seen) != n {
		t.Errorf("顺着游标应当能取到全部 %d 条，得到 %d", n, len(seen))
	}
}

// limit 越界钳到默认值，和 /api/v1/messages 的做法一致：不合法不报错，取默认。
func TestLimitIsClamped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, q := range []string{"limit=0", "limit=-5", "limit=999999", "limit=abc"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/x?"+q, nil)
		if _, limit := pageArgs(c, 40); limit != 40 {
			t.Errorf("%s 应当钳到 40，得到 %d", q, limit)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x?limit=7&cursor=99", nil)
	cursor, limit := pageArgs(c, 40)
	if limit != 7 || cursor != 99 {
		t.Errorf("合法值应当原样通过，得到 limit=%d cursor=%d", limit, cursor)
	}
}

// 设置页那颗小圆点的数据源：哪些键还是 config.toml 的值。
func TestSettingsReportsFromConfig(t *testing.T) {
	s, r, cookie, _ := adminWith(t, "别人")
	_, e, _ := apiGet(t, r, "/admin/api/settings", cookie)
	var d settingsData
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	for _, k := range settingKeys {
		if _, ok := d.FromConfig[k]; !ok {
			t.Errorf("from_config 缺 %s", k)
		}
		if !d.FromConfig[k] {
			t.Errorf("还没在后台改过，%s 应当是 true", k)
		}
	}
	if err := s.Settings.Save(map[string]string{"site_name": "改过了"}); err != nil {
		t.Fatal(err)
	}
	_, e, _ = apiGet(t, r, "/admin/api/settings", cookie)
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if d.FromConfig["site_name"] {
		t.Error("改过之后 site_name 的 from_config 应当是 false")
	}
	if d.Values.SiteName != "改过了" {
		t.Errorf("site_name 应当读到新值，得到 %q", d.Values.SiteName)
	}
}

// 设备列表【不透出 apns_token】。
func TestMemberDetailHidesAPNsToken(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	dev := &models.Device{
		UUID: "u1", UserId: member, Name: "iPhone",
		APNsToken: "SECRETTOKEN", APNsEnv: "production", Status: models.DeviceLive,
	}
	if _, err := s.DAO.Engine().Insert(dev); err != nil {
		t.Fatal(err)
	}
	_, e, _ := apiGet(t, r, "/admin/api/members/"+itoa(member), cookie)
	if strings.Contains(string(e.Data), "SECRETTOKEN") {
		t.Error("设备的 apns_token 被透出去了")
	}
	var d memberDetail
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if len(d.Devices) != 1 || !d.Devices[0].CanPush {
		t.Errorf("应当有一台能收推送的设备，得到 %+v", d.Devices)
	}
}

// 同一个频道的消息数，在两个端点上必须是同一个数。
//
// 原先频道详情读反范式的 channel.msg_count、成员详情走 COUNT(*)，而保留期 GC
// （internal/service/gc.go 那条 DELETE）不维护 msg_count——跑过之后两屏对同一个
// 频道显示两个数，而且都不报错。这里直接模拟 GC 的删法。
func TestChannelMessageCountAgreesAcrossEndpoints(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if _, err := service.NewSend(s.DAO).Deliver(ch, service.SendInput{Title: "x"}, "127.0.0.1"); err != nil {
			t.Fatal(err)
		}
	}

	counts := func() (detail, inMember int64) {
		_, e, _ := apiGet(t, r, "/admin/api/channels/"+ch.Id, cookie)
		var cd channelDetail
		if err := json.Unmarshal(e.Data, &cd); err != nil {
			t.Fatal(err)
		}
		_, e, _ = apiGet(t, r, "/admin/api/members/"+itoa(member), cookie)
		var md memberDetail
		if err := json.Unmarshal(e.Data, &md); err != nil {
			t.Fatal(err)
		}
		for _, c := range md.Channels {
			if c.ID == ch.Id {
				return cd.Channel.Messages, c.Messages
			}
		}
		t.Fatal("成员详情里没有这个频道")
		return 0, 0
	}

	if a, b := counts(); a != 6 || b != 6 {
		t.Fatalf("发完 6 条，两处都应当是 6，得到 频道详情=%d 成员详情=%d", a, b)
	}

	// 模拟保留期 GC：直接删行，不碰 msg_count——这正是 gc.go 的做法
	if _, err := s.DAO.Engine().Exec(
		"DELETE FROM message WHERE channel_id=? AND id IN (SELECT id FROM message WHERE channel_id=? LIMIT 4)",
		ch.Id, ch.Id); err != nil {
		t.Fatal(err)
	}
	a, b := counts()
	if a != b {
		t.Errorf("GC 之后两个端点对同一频道给出不同的数：频道详情=%d 成员详情=%d", a, b)
	}
	if a != 2 {
		t.Errorf("删掉 4 条之后应当剩 2 条，得到 %d", a)
	}
}

// 成员详情里的三个计数不能是 0——列表端点同名字段是真值，
// 同一个类型在两处含义不同的话，前端复用一个卡片组件就会在详情页显示「设备 0」。
func TestMemberDetailFillsCounts(t *testing.T) {
	s, r, cookie, member := adminWith(t, "别人")
	ch, err := service.NewChannel(s.DAO).Create(member, service.ChannelInput{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NewSend(s.DAO).Deliver(ch, service.SendInput{Title: "x"}, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	dev := &models.Device{UUID: "u1", UserId: member, Name: "iPhone",
		APNsToken: "t", APNsEnv: "production", Status: models.DeviceLive}
	if _, err := s.DAO.Engine().Insert(dev); err != nil {
		t.Fatal(err)
	}

	_, e, _ := apiGet(t, r, "/admin/api/members/"+itoa(member), cookie)
	var d memberDetail
	if err := json.Unmarshal(e.Data, &d); err != nil {
		t.Fatal(err)
	}
	if d.Member.Devices != 1 || d.Member.Channels != 1 || d.Member.Messages != 1 {
		t.Errorf("member 里的计数应当是 1/1/1，得到 %d/%d/%d",
			d.Member.Devices, d.Member.Channels, d.Member.Messages)
	}
	// 和同一份响应里的数组长度对得上
	if int(d.Member.Devices) != len(d.Devices) || int(d.Member.Channels) != len(d.Channels) {
		t.Error("member 里的计数和同一份响应里的数组长度对不上")
	}
}

// listOf 拿到 nil 切片时也要给出 []。
//
// 现在三个列表端点都经过 make() 的映射函数，所以这条在端点层面测不出来——
// 它防的是下一个直接 res.Rsucc(listOf(...)) 的调用方：xorm 查不到行时留下的是
// nil 切片，序列化成 items:null，前端一个 data.items.map() 就抛 TypeError。
func TestListOfNilIsEmptyArray(t *testing.T) {
	out := listOf[memberJSON](nil, 10, func(m memberJSON) int64 { return m.ID })
	if out.Items == nil {
		t.Fatal("nil 切片应当变成空切片")
	}
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"items":[]`) {
		t.Errorf("应当序列化成 items:[]，得到 %s", b)
	}
	if out.HasMore || out.NextCursor != "" {
		t.Errorf("空结果不该有下一页：%+v", out)
	}
}

// 端点这一层的同一条契约（走的是映射函数，不是 listOf）。
func TestEmptyListIsArrayNotNull(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	for _, p := range []string{
		"/admin/api/messages?q=一条也匹配不上的词",
		"/admin/api/members?q=没有这个人",
	} {
		_, e, _ := apiGet(t, r, p, cookie)
		if !strings.Contains(string(e.Data), `"items":[]`) {
			t.Errorf("%s：空列表应当是 []，得到 %s", p, e.Data)
		}
	}
}

// 设置页的 tab 用 key 不用中文文案：文案一翻译，中文当路由状态的那条链接就点不亮了。
func TestSettingsTabUsesKeyNotLabel(t *testing.T) {
	_, r, cookie, _ := adminWith(t, "别人")
	body := adminGet(t, r, "/admin/settings?tab=server", cookie).Body.String()
	if !strings.Contains(body, "改密码") {
		t.Error("?tab=server 应当打开「服务器」那一页")
	}
	if strings.Contains(body, "tab=%E6%9C%8D%E5%8A%A1%E5%99%A8") || strings.Contains(body, "tab=服务器") {
		t.Error("链接里还有中文当路由状态")
	}
}
