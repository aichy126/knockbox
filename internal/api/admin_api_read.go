package api

import (
	"strconv"
	"strings"
	"time"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// 后台的只读接口。读写分成两个文件，好让「哪些 handler 会改数据」
// 在文件树上一眼可见。

// ── 会话与外壳 ──────────────────────────────────────────

type meUser struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	Unlimited bool   `json:"unlimited"`
}

type meServer struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	ExternalURL   string `json:"external_url"`
	Host          string `json:"host"`
	PublicEnabled bool   `json:"public_enabled"`
}

type meData struct {
	User   meUser        `json:"user"`
	Usage  service.Usage `json:"usage"`
	Server meServer      `json:"server"`
}

// adminMe 当前登录者 + 这台服务器。
//
// 把 server 并进来是全案唯一一处偏离「一个资源一个端点」的地方，值得：
// 前端每次启动渲染外壳（站点名、版本、公共/自建徽标）需要的东西一次拿全，
// 省掉一次瀑布请求。
func (s *Server) adminMe(c *gin.Context) {
	u := middleware.Admin(c)
	res.Rsucc(c, meData{
		User: meUser{
			ID: u.Id, Name: u.Name, Username: u.Username,
			Role: u.Role, Unlimited: u.Unlimited != 0,
		},
		Usage: s.quota().Usage(u.Id),
		Server: meServer{
			Name: s.Name, Version: s.Version, ExternalURL: s.ExternalURL,
			Host: hostOf(s.ExternalURL), PublicEnabled: s.Settings.PublicEnabled(),
		},
	})
}

// hostOf 剥掉 scheme 与路径，只留主机名。外壳左下角显示的是它。
func hostOf(external string) string {
	h := strings.TrimPrefix(strings.TrimPrefix(external, "https://"), "http://")
	if i := strings.IndexByte(h, '/'); i >= 0 {
		h = h[:i]
	}
	return h
}

// ── 概览 ──────────────────────────────────────────────

type overviewData struct {
	Window   int64 `json:"window"`
	Messages struct {
		Current int64 `json:"current"`
		Prev    int64 `json:"prev"`
	} `json:"messages"`
	Push struct {
		OK       int64 `json:"ok"`
		Failed   int64 `json:"failed"`
		Retrying int64 `json:"retrying"`
		// Rate 一次推送都没有时是 null，不是 0：
		// 「成功率 0%」和「没推过」在界面上是两句不同的话。
		Rate *float64 `json:"rate"`
	} `json:"push"`
	Devices struct {
		Pushable int64 `json:"pushable"`
		Sandbox  int64 `json:"sandbox"`
	} `json:"devices"`
	Channels struct {
		Total int64 `json:"total"`
		Muted int64 `json:"muted"`
	} `json:"channels"`
	Failures []service.FailureRow `json:"failures"`
	Recent   []messageJSON        `json:"recent"`
}

func (s *Server) adminAPIOverview(c *gin.Context) {
	window, _ := strconv.ParseInt(c.Query("window"), 10, 64)
	if window <= 0 || window > 7*86400 {
		window = 86400
	}
	since := time.Now().Unix() - window

	// uid=0 = 全站。见 service.Admin.Overview 上的注释。
	o, err := s.admin().Overview(0, since, window)
	if err != nil {
		s.adminFail(c, "overview", err)
		return
	}
	var d overviewData
	d.Window = window
	d.Messages.Current, d.Messages.Prev = o.Messages, o.MessagesPrev
	d.Push.OK, d.Push.Failed, d.Push.Retrying = o.PushOK, o.PushFailed, o.PushRetrying
	if v, ok := o.Rate(); ok {
		d.Push.Rate = &v
	}
	d.Devices.Pushable, d.Devices.Sandbox = o.DevicesPushable, o.DevicesSandbox
	d.Channels.Total, d.Channels.Muted = o.Channels, o.ChannelsMuted

	if d.Failures, err = s.admin().Failures(0, since, 6); err != nil {
		s.adminFail(c, "overview failures", err)
		return
	}
	if d.Failures == nil {
		d.Failures = []service.FailureRow{}
	}
	recent, err := s.admin().RecentMessages(0, 8)
	if err != nil {
		s.adminFail(c, "overview recent", err)
		return
	}
	d.Recent = s.messagesJSONWith(recent)
	res.Rsucc(c, d)
}

// ── 成员 ──────────────────────────────────────────────

type memberJSON struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Status      int    `json:"status"`
	Unlimited   bool   `json:"unlimited"`
	Devices     int64  `json:"devices"`
	Channels    int64  `json:"channels"`
	Messages    int64  `json:"messages"`
	LastLoginAt int64  `json:"last_login_at"`
	CreatedAt   int64  `json:"created_at"`
}

func memberJSONOf(r service.MemberRow) memberJSON {
	return memberJSON{
		ID: r.Id, Name: r.Name, Role: r.Role, Status: r.Status,
		Unlimited: r.Unlimited != 0, Devices: r.Devices, Channels: r.Channels,
		Messages: r.Messages, LastLoginAt: r.LastLoginAt, CreatedAt: r.Ctime,
	}
}

func (s *Server) adminAPIMembers(c *gin.Context) {
	cursor, limit := pageArgs(c, 50)
	rows, total, err := s.admin().Members(service.MemberFilter{
		Query: strings.TrimSpace(c.Query("q")), Cursor: cursor, Limit: limit + 1,
	})
	if err != nil {
		s.adminFail(c, "member list", err)
		return
	}
	out := listOf(rows, limit, func(r service.MemberRow) int64 { return r.Id })
	items := make([]memberJSON, 0, len(out.Items))
	for _, r := range out.Items {
		items = append(items, memberJSONOf(r))
	}
	// 成员表是几百行量级，COUNT(*) 便宜，而界面上「共 N 人」要用它。
	res.Rsucc(c, List[memberJSON]{
		Items: items, HasMore: out.HasMore, NextCursor: out.NextCursor, Total: &total,
	})
}

type deviceJSON struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Model      string `json:"model"`
	OSVersion  string `json:"os_version"`
	AppVersion string `json:"app_version"`
	APNsEnv    string `json:"apns_env"`
	// CanPush 只说有没有 token，【不透出 token 本身】。
	CanPush    bool  `json:"can_push"`
	Status     int   `json:"status"`
	SyncRev    int64 `json:"sync_rev"`
	LastSeenAt int64 `json:"last_seen_at"`
}

type channelJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Meta      string `json:"meta"` // 原样透出：图标与颜色在里面，服务端不解析
	Muted     bool   `json:"muted"`
	MuteUntil int64  `json:"mute_until"`
	Sound     string `json:"sound"`
	Level     string `json:"level"`
	Messages  int64  `json:"messages"`
	LastMsgAt int64  `json:"last_msg_at"`
}

type memberDetail struct {
	Member   memberJSON    `json:"member"`
	Usage    service.Usage `json:"usage"`
	Channels []channelJSON `json:"channels"`
	Devices  []deviceJSON  `json:"devices"`
}

// adminAPIMember 成员详情。
//
// 频道与设备不分页，是有意的：前者被 max_channels 约束，后者被「一个人有几台
// 手机」约束。给它们加游标是纯仪式。
func (s *Server) adminAPIMember(c *gin.Context) {
	id := pathID(c)
	u, err := s.admin().Member(id)
	if err != nil {
		s.adminFail(c, "member", err)
		return
	}
	chans, err := s.admin().MemberChannels(id)
	if err != nil {
		s.adminFail(c, "member channels", err)
		return
	}
	devs, err := s.admin().MemberDevices(id)
	if err != nil {
		s.adminFail(c, "member devices", err)
		return
	}
	d := memberDetail{
		Member: memberJSON{
			ID: u.Id, Name: u.Name, Role: u.Role, Status: u.Status,
			Unlimited: u.Unlimited != 0, LastLoginAt: u.LastLoginAt, CreatedAt: u.Ctime,
			// 三个计数必须填：memberJSON 在列表端点里是真值，
			// 详情端点留零的话，同一个类型在两处含义不同，
			// 前端复用同一个卡片组件就会在详情页显示「设备 0」。
			Devices: int64(len(devs)), Channels: int64(len(chans)),
		},
		Usage:    s.quota().Usage(u.Id),
		Channels: make([]channelJSON, 0, len(chans)),
		Devices:  make([]deviceJSON, 0, len(devs)),
	}
	for _, ch := range chans {
		d.Channels = append(d.Channels, channelJSON{
			ID: ch.Id, Name: channelName(ch.Meta, ch.Id), Meta: ch.Meta,
			Muted: ch.Muted != 0, MuteUntil: ch.MuteUntil, Sound: ch.Sound,
			Level: ch.Level, Messages: ch.Messages, LastMsgAt: ch.LastMsgAt,
		})
	}
	d.Member.Messages = d.Usage.Messages
	for _, dv := range devs {
		d.Devices = append(d.Devices, deviceJSON{
			ID: dv.Id, Name: dv.Name, Platform: dv.Platform, Model: dv.Model,
			OSVersion: dv.OSVersion, AppVersion: dv.AppVersion, APNsEnv: dv.APNsEnv,
			CanPush: dv.CanPush(), Status: dv.Status, SyncRev: dv.SyncRev,
			LastSeenAt: dv.LastSeenAt,
		})
	}
	res.Rsucc(c, d)
}

// ── 频道 ──────────────────────────────────────────────

type channelDetail struct {
	Channel channelJSON `json:"channel"`
	Owner   struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	} `json:"owner"`
	// Token 后台专属。发送说明页要用，而它本来就显示在频道属性里。
	Token      string `json:"token"`
	LastUsedAt int64  `json:"last_used_at"`
	LastUsedIP string `json:"last_used_ip"`
	SendURL    string `json:"send_url"`
}

func (s *Server) adminAPIChannel(c *gin.Context) {
	ch, owner, err := s.admin().Channel(c.Param("id"))
	if err != nil {
		s.adminFail(c, "channel", err)
		return
	}
	// 【不用 ch.MsgCount】：那是反范式计数器，保留期 GC 不维护它。
	// 成员详情里同一个频道走的是 COUNT(*)，两处必须同一个口径，
	// 否则 GC 跑过之后同一个频道在两屏上是两个数，而且都不报错。
	n, err := s.admin().ChannelMessageCount(ch.Id)
	if err != nil {
		s.adminFail(c, "channel message count", err)
		return
	}
	var d channelDetail
	d.Channel = channelJSON{
		ID: ch.Id, Name: channelName(ch.Meta, ch.Id), Meta: ch.Meta,
		Muted: ch.Muted != 0, MuteUntil: ch.MuteUntil, Sound: ch.Sound,
		Level: ch.Level, Messages: n, LastMsgAt: ch.LastMsgAt,
	}
	d.Owner.ID, d.Owner.Name = owner.Id, owner.Name
	d.Token, d.LastUsedAt, d.LastUsedIP = ch.Token, ch.LastUsedAt, ch.LastUsedIP
	d.SendURL = s.ExternalURL + "/s/" + ch.Token
	res.Rsucc(c, d)
}

// adminAPIChannelMessages 频道消息流。
//
// 按 id 倒序【查】、正序【返回】，和服务端直出的那一版一致：要的是最近 N 条，
// 而显示顺序跟 app 一样是时间正序。
//
// 顺带补上一个功能缺口：直出的那版固定 40 条、没有任何往前翻的办法，
// 页面上用一个「看更早的」链接跳到搜索页糊过去。
func (s *Server) adminAPIChannelMessages(c *gin.Context) {
	if _, _, err := s.admin().Channel(c.Param("id")); err != nil {
		s.adminFail(c, "channel", err)
		return
	}
	cursor, limit := pageArgs(c, 40)
	rows, err := s.admin().Messages(service.MessageFilter{
		ChannelId: c.Param("id"), Before: cursor, Limit: limit + 1, WithBody: true,
	})
	if err != nil {
		s.adminFail(c, "channel messages", err)
		return
	}
	out := listOf(rows, limit, func(r service.MessageRow) int64 { return r.Id })
	for i, j := 0, len(out.Items)-1; i < j; i, j = i+1, j-1 {
		out.Items[i], out.Items[j] = out.Items[j], out.Items[i]
	}
	res.Rsucc(c, List[messageJSON]{
		Items: s.messagesJSONWith(out.Items), HasMore: out.HasMore, NextCursor: out.NextCursor,
	})
}

// ── 消息 ──────────────────────────────────────────────

type messageJSON struct {
	ID        int64  `json:"id"`
	UID       string `json:"uid"`
	UserID    int64  `json:"user_id"`
	Owner     string `json:"owner"`
	Channel   string `json:"channel"`
	ChannelNm string `json:"channel_name"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Body      string `json:"body,omitempty"`
	Extra     string `json:"extra,omitempty"`
	FileURL   string `json:"file_url,omitempty"`
	CreatedAt int64  `json:"created_at"`
	Read      bool   `json:"read"`
	Push      struct {
		OK    int64 `json:"ok"`
		Total int64 `json:"total"`
	} `json:"push"`
	Replyable  bool   `json:"replyable"`
	Reply      string `json:"reply,omitempty"`
	RepliedAt  int64  `json:"replied_at,omitempty"`
	ReplyUntil int64  `json:"reply_until,omitempty"`
}

// messagesJSON 列表形态。
//
// 【reply_webhook 不在这里】：它只在消息详情里出现。后台排障要看得到，
// 但列表里没有理由带上它——和「回调地址不下发给客户端」是同一条纪律。
func (s *Server) messageJSONOf(r service.MessageRow) messageJSON {
	m := messageJSON{
		ID: r.Id, UID: r.UID, UserID: r.UserId, Owner: r.Owner,
		Channel: r.ChannelId, ChannelNm: channelName(r.Meta, r.ChannelId),
		Type: r.Type, Title: r.Title, Summary: r.Summary, Body: r.Body, Extra: r.Extra,
		CreatedAt: r.Ctime, Read: r.ReadAt != 0,
		Replyable: r.ReplyWebhook != "", Reply: r.Reply,
		RepliedAt: r.RepliedAt, ReplyUntil: r.ReplyUntil,
	}
	m.Push.OK, m.Push.Total = r.PushOK, r.PushTotal
	if r.Type == models.TypeImage || r.Type == models.TypeFile {
		if uid := extraFileUID(r.Extra); uid != "" && s.Files != nil {
			m.FileURL = s.Files.URL(uid, true)
		}
	}
	return m
}

func (s *Server) adminAPIMessages(c *gin.Context) {
	cursor, limit := pageArgs(c, 40)

	// 【按成员 id 筛，不是名字】。服务端直出的那版收名字，是因为 <input list>
	// 里用户打的就是名字，「查不到当作没筛选」是给打字用的容错；前端手里有 id，
	// 传一个查不到的 id 应当明确报错，而不是静默返回全量。
	var member int64
	if v := strings.TrimSpace(c.Query("member")); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			s.fail(c, uierr.New(uierr.MemberNotFound))
			return
		}
		if _, err := s.admin().Member(id); err != nil {
			s.adminFail(c, "member", err)
			return
		}
		member = id
	}

	rows, err := s.admin().Messages(service.MessageFilter{
		Query:     strings.TrimSpace(c.Query("q")),
		UserId:    member,
		ChannelId: c.Query("channel"),
		Before:    cursor,
		Limit:     limit + 1,
		// 【必须带 WithBody】：它同时控制 body、extra 和 reply_*。不带的话
		// replyable 恒为 false——一个会骗人的字段，界面据此判断要不要画回复区。
		// 搜索本来就是在正文里搜（LIKE m.body），把命中的那段带回去也是应当的。
		WithBody: true,
	})
	if err != nil {
		s.adminFail(c, "message search", err)
		return
	}
	out := listOf(rows, limit, func(r service.MessageRow) int64 { return r.Id })
	// 【不给 total】：COUNT(*) 叠三个 LIKE '%..%' 是每翻一页一次全表扫。
	res.Rsucc(c, List[messageJSON]{
		Items: s.messagesJSONWith(out.Items), HasMore: out.HasMore, NextCursor: out.NextCursor,
	})
}

type messageDetail struct {
	Message messageJSON `json:"message"`
	// ReplyWebhook 只在详情里出现，不下发给任何客户端。
	ReplyWebhook string                 `json:"reply_webhook,omitempty"`
	PushLog      []service.PushLogRow   `json:"push_log"`
	ReplyHooks   []service.ReplyHookRow `json:"reply_hooks"`
}

func (s *Server) adminAPIMessage(c *gin.Context) {
	m, ch, err := s.admin().Message(c.Param("uid"))
	if err != nil {
		s.adminFail(c, "message", err)
		return
	}
	row := service.MessageRow{
		Id: m.Id, UID: m.UID, UserId: m.UserId, ChannelId: m.ChannelId, Meta: ch.Meta,
		Type: m.Type, Title: m.Title, Summary: m.Summary, Body: m.Body, Extra: m.Extra,
		Ctime: m.Ctime, ReadAt: m.ReadAt, ReplyWebhook: m.ReplyWebhook, Reply: m.Reply,
		RepliedAt: m.RepliedAt, ReplyUntil: m.ReplyUntil,
	}
	row.Owner = s.admin().MemberName(m.UserId)
	d := messageDetail{Message: s.messageJSONOf(row), ReplyWebhook: m.ReplyWebhook}
	if d.PushLog, err = s.admin().PushLog(m.Id); err != nil {
		s.adminFail(c, "push log", err)
		return
	}
	if d.ReplyHooks, err = s.admin().ReplyHooks(m.Id); err != nil {
		s.adminFail(c, "reply hooks", err)
		return
	}
	if d.PushLog == nil {
		d.PushLog = []service.PushLogRow{}
	}
	if d.ReplyHooks == nil {
		d.ReplyHooks = []service.ReplyHookRow{}
	}
	res.Rsucc(c, d)
}

// ── 配对 ──────────────────────────────────────────────

type pairTarget struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// IsMe 让前端把当前登录者预选上，和服务端直出那版的行为一致。
	IsMe bool `json:"is_me"`
}

func (s *Server) adminAPIPairTargets(c *gin.Context) {
	rows, err := s.admin().MemberNames()
	if err != nil {
		s.adminFail(c, "pair targets", err)
		return
	}
	me := middleware.UserID(c)
	items := make([]pairTarget, 0, len(rows))
	for _, r := range rows {
		items = append(items, pairTarget{ID: r.Id, Name: r.Name, IsMe: r.Id == me})
	}
	res.Rsucc(c, gin.H{"items": items, "count": len(items)})
}

// ── 设置 ──────────────────────────────────────────────

type settingsValues struct {
	PublicEnabled   bool   `json:"public_enabled"`
	RegisterPerHour int    `json:"register_per_hour"`
	MaxChannels     int    `json:"max_channels"`
	MaxPerDay       int    `json:"max_per_day"`
	RetentionDays   int    `json:"retention_days"`
	SiteName        string `json:"site_name"`
}

type settingsData struct {
	Values settingsValues `json:"values"`
	// FromConfig 哪些键还是 config.toml 的值、没被后台改过。
	// 界面上那颗小圆点靠它——没有它，用户没法判断「我改了配置文件为什么不生效」。
	FromConfig map[string]bool `json:"from_config"`
}

var settingKeys = []string{
	"public_enabled", "register_per_hour", "max_channels",
	"max_per_day", "retention_days", "site_name",
}

func (s *Server) settingsSnapshot() settingsData {
	st := s.Settings
	d := settingsData{
		Values: settingsValues{
			PublicEnabled:   st.PublicEnabled(),
			RegisterPerHour: st.RegisterPerHour(),
			MaxChannels:     st.MaxChannels(),
			MaxPerDay:       st.MaxPerDay(),
			RetentionDays:   st.RetentionDays(),
			SiteName:        st.SiteName(),
		},
		FromConfig: make(map[string]bool, len(settingKeys)),
	}
	for _, k := range settingKeys {
		d.FromConfig[k] = st.FromConfig(k)
	}
	return d
}

func (s *Server) adminAPISettings(c *gin.Context) { res.Rsucc(c, s.settingsSnapshot()) }

// 供只读接口复用的小件
func (s *Server) messagesJSONWith(rows []service.MessageRow) []messageJSON {
	out := make([]messageJSON, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.messageJSONOf(r))
	}
	return out
}
