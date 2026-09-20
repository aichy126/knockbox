package api

import (
	"time"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// Server 路由层的依赖。
// PublicConfig 公共实例相关。
type PublicConfig struct {
	Enabled         bool
	RegisterPerHour int
	MaxChannels     int
	MaxPerDay       int
	RetentionDays   int
	SiteName        string
	DocsURL         string
}

type Server struct {
	Name        string
	Version     string
	ExternalURL string
	DAO         *dao.DAO
	// Notify 叫醒推送 worker。落库之后调一次，让新排队的消息立刻被取走，
	// 而不是干等到下一轮扫描。
	Notify func()
	// NotifyHook 叫醒回调 worker，用户回复落库之后调一次。
	// 与 Notify 分开是因为两个队列各有自己的循环：回复不产生推送，
	// 新消息也不产生回调，合用一个唤醒信号只会让两边互相白跑。
	NotifyHook func()
	// Hooks 回调投递。发送时用它先判一次地址（公共实例不许打内网），
	// 让发送方立刻知道地址不行，而不是等用户回复完才在服务端日志里失败。
	Hooks *service.Webhook
	// Files 附件存储。签名 URL 的有效期在它内部（storage.url_ttl）——
	// 通知可能延迟投递，TTL 太短会出现「点开是空白图」。
	Files *service.File
	// Public 公共模式。关闭时首页去登录页，开放注册接口一律 404——
	// 自建的人绝不希望谁摸到地址就能在自己的服务器上开账号。
	Public PublicConfig
	// Quota 公共实例的每用户配额。自建模式下 Limits.Enabled=false，一律放行。
	Quota *service.Quota
	// Settings 运行时可改的设置（后台里能编辑）。config.toml 只是首次启动的默认值。
	Settings *service.Settings

	// PairTTL 配对码有效期（server.pair_ttl）。
	PairTTL time.Duration
	// PairPerMin 每个 IP 每分钟能提交几次配对（limit.pair_per_min）。
	// 配对码只有 40 位，要人手输所以不能更长，靠这条补足。
	PairPerMin int
	// BodyMaxBytes 单条消息正文上限（limit.body_max_kb）。
	// 正文不进 APNs payload，所以这个数只受存储约束。
	BodyMaxBytes int64
	// SendQPS / SendBurst 每个频道 token 的发送速率（limit.send_qps / send_burst）。
	// 这是防滥用，不是业务配额：公共模式的 max_messages_per_day 管的是每天总量，
	// 而它管的是瞬时速率，且【自建模式下同样生效】——
	// 自建模式没有任何配额，一个泄露的 token 不该能无限写库、无限占盘。
	// SendQPS = 0 表示完全不限。
	SendQPS   int
	SendBurst int

	// joinLimit 按 IP 限制签发开放注册码的频率，由 Router 装配。
	joinLimit *middleware.RateLimit
	// sendLimit 按频道 token 限制发送速率，由 Router 装配。
	sendLimit *middleware.TokenBucket
}

// Router 注册全部路由。这是唯一的路由注册处。
//
// /health 由 igo 的 EnableHealthCheck() 提供（带 db 连通性检测），不在这里。
func Router(r *gin.Engine, s *Server) {
	if s.Notify == nil {
		s.Notify = func() {}
	}
	// 配置缺项时的兜底。这几个值同时也是 config.toml.example 里写的默认值。
	if s.PairTTL <= 0 {
		s.PairTTL = 10 * time.Minute
	}
	if s.PairPerMin <= 0 {
		s.PairPerMin = 5
	}
	if s.BodyMaxBytes <= 0 {
		s.BodyMaxBytes = 1 << 20
	}
	// SendQPS 允许显式配成 0（不限），所以这里不兜底，只在没设过时给默认值。
	if s.SendQPS < 0 {
		s.SendQPS = 0
	}
	if s.sendLimit == nil {
		s.sendLimit = middleware.NewTokenBucket(s.SendQPS, s.SendBurst)
	}

	// ── 管理页面：服务端直出 HTML，无前端构建步骤 ──────────────
	verify := func(raw string) (*models.User, error) { return service.NewSession(s.DAO).Verify(raw) }
	r.GET("/", s.home)
	r.GET("/docs", s.docsPage)
	// 频道发送说明页。凭据就是 URL 里那个 token——它与「持有 token」等价，
	// 所以不需要再加一层鉴权，也读不到任何消息。
	r.GET("/s/:token", s.sendGuide)
	r.POST("/s/:token/try", s.sendTry)
	// 开放注册的两条路由常驻，是否放行由【运行时设置】决定。
	// 公共模式能在后台开关，而路由只在启动时注册一次：按启动时的配置决定挂不挂的话，
	// 后台打开公共模式之后，接入页会去轮询一个并不存在的 /join/status。
	// 关闭时 home 跳登录页、joinStatus 回 404，陌生人依旧开不了账号。
	r.GET("/join", s.home)
	r.GET("/join/status", s.joinStatus)
	// 限流挂在【真正签发配对码】那一步，不挂在路由上：接入页刷新一次就算一次的话，
	// 默认每小时 3 次会把正常访客挡在外面。
	if s.joinLimit == nil {
		s.joinLimit = middleware.NewRateLimitFunc(s.Settings.RegisterPerHour, time.Hour)
	}
	// 语言开关。不要求登录：cookie 只影响渲染语言，换一门语言看不到任何多余的东西，
	// 而登录页自己也要能切。
	r.GET("/lang", s.setLang)
	// 图标。/favicon.ico 是浏览器自己会去要的那一条，即便没有哪一页引用它——
	// 不给的话每开一页就多一次 404。
	r.GET("/favicon.ico", s.asset("favicon.png"))
	r.GET("/favicon.png", s.asset("favicon.png"))
	r.GET("/apple-touch-icon.png", s.asset("apple-touch-icon.png"))
	r.GET("/login", s.loginPage)
	r.GET("/logout", s.logout)
	admin := r.Group("", middleware.AdminAuth(verify, uierr.AdminSessionExpired, s.userText))
	// 后台里的配对页（能选发给谁）。/pair 留作旧地址，重定向过去。
	admin.GET("/admin/pair", s.adminPairPage)
	admin.POST("/admin/pair", s.adminPairIssue)
	admin.GET("/pair", func(c *gin.Context) { c.Redirect(302, "/admin/pair") })
	admin.GET("/admin", s.adminDash)
	admin.GET("/admin/messages", s.adminMessages)
	admin.GET("/admin/messages/:uid", s.adminMessageDetail)
	// 频道和设备不再有顶层列表——它们属于成员，从成员详情钻进去
	admin.GET("/admin/channels/:id", s.adminChannel)
	admin.POST("/admin/channels/:id/purge", s.adminChannelPurge)
	admin.GET("/admin/users/:id", s.adminUser)
	admin.POST("/admin/devices/:id/delete", s.adminDeviceDelete)
	admin.GET("/admin/users", s.adminUsers)
	admin.GET("/admin/settings", s.adminSettings)
	admin.POST("/admin/settings", s.adminSettingsSave)
	admin.POST("/admin/account/password", s.adminPasswordSave)
	admin.POST("/admin/users/:id/unlimited", s.adminUserUnlimited)

	// ── 后台自用的 JSON 接口 ──────────────────────────────
	//
	// 【不是公开契约】：/api/v1 才是对外承诺的那一套，这一组只服务本仓库自带的
	// 后台界面，会随版本改动。鉴权与上面那批页面共用同一个 AdminAuth。
	//
	// 路径里第二段 api 是静态串，和 messages / users / channels 那几条不冲突。
	admin.GET("/admin/api/me", s.adminMe)
	admin.GET("/admin/api/overview", s.adminAPIOverview)
	admin.GET("/admin/api/members", s.adminAPIMembers)
	admin.GET("/admin/api/members/:id", s.adminAPIMember)
	admin.GET("/admin/api/channels/:id", s.adminAPIChannel)
	admin.GET("/admin/api/channels/:id/messages", s.adminAPIChannelMessages)
	admin.GET("/admin/api/messages", s.adminAPIMessages)
	admin.GET("/admin/api/messages/:uid", s.adminAPIMessage)
	admin.GET("/admin/api/pair/targets", s.adminAPIPairTargets)
	admin.GET("/admin/api/settings", s.adminAPISettings)
	admin.POST("/admin/api/logout", s.adminAPILogout)
	admin.POST("/admin/api/members/:id/unlimited", s.adminAPIUnlimited)
	admin.POST("/admin/api/devices/:id/revoke", s.adminAPIRevokeDevice)
	admin.POST("/admin/api/channels/:id/purge", s.adminAPIPurge)
	admin.POST("/admin/api/pair", s.adminAPIPairIssue)
	admin.PUT("/admin/api/settings", s.adminAPISettingsSave)
	admin.POST("/admin/api/account/password", s.adminAPIPassword)

	// 配对【之前】就能调：app 填完服务器地址先探一下，地址填错能立刻报错，
	// 而不是卡在「配对失败」让人分不清是地址错了还是配对码错了。
	r.GET("/api/v1/server", s.serverInfo)

	// ── 发送侧：频道 token，只写 ──────────────────────────────────
	send := r.Group("/api/v1", middleware.SendAuth(s.DAO))
	send.POST("/send", s.send)
	send.POST("/send/batch", s.sendBatch)
	sendTok := r.Group("/api/v1")
	sendTok.POST("/send/:token", middleware.SendAuth(s.DAO), s.sendPath)
	sendTok.GET("/send/:token", middleware.SendAuth(s.DAO), s.sendPath)
	send.POST("/upload", s.upload)
	// MCP：同一条发送路径的另一个接入面，凭据与权限都和上面一样。装配见 mcp.go。
	mountMCP(r, s)
	// 附件下载只认签名，不要登录态：通知服务扩展拿着 payload 就得能下图，
	// 不能依赖它在首次解锁前未必读得到的 Keychain。
	r.GET("/api/v1/file/:uid", s.serveFile)

	// ── 配对：不需要鉴权，它就是拿身份的地方 ─────────────────────
	// 配对码只有 40 位（要人手输，不能更长），**必须靠限流补足**：
	// 单次使用 + 有限的有效期 + 这里的每 IP 每分钟若干次，三样缺一不可。
	// 少了限流，攻击者能在有效期窗口里无限次猜。
	r.POST("/api/v1/pair", middleware.NewRateLimit(s.PairPerMin, time.Minute).
		Gin(uierr.PairTooFast, s.userText), s.pair)
	// 登录同理：单账号服务最怕的就是慢速爆破
	r.POST("/login", middleware.NewRateLimit(10, time.Minute).Gin(uierr.LoginTooFast, s.userText), s.doLogin)

	// ── 客户端侧：设备 token ─────────────────────────────────────
	app := r.Group("/api/v1", middleware.DeviceAuth(s.DAO))
	app.POST("/device/push-token", s.pushToken)
	app.POST("/pair/issue", s.issuePair)
	app.GET("/devices", s.listDevices)
	app.DELETE("/devices/:uuid", s.deleteDevice)
	app.GET("/channels", s.listChannels)
	app.POST("/channels", s.createChannel)
	app.PATCH("/channels/:id", s.updateChannel)
	app.POST("/channels/:id/rotate", s.rotateChannelToken)
	app.DELETE("/channels/:id", s.deleteChannel)
	app.POST("/channels/:id/purge", s.purgeChannel)
	app.GET("/sync", s.sync)
	app.GET("/messages", s.messages)
	app.POST("/messages/read", s.markRead)
	app.POST("/messages/delete", s.deleteMessages)
	app.POST("/messages/:uid/reply", s.reply)
}

func (s *Server) serverInfo(c *gin.Context) {
	res.Rsucc(c, gin.H{
		"name":         s.Name,
		"version":      s.Version,
		"require_pair": true,
		// 客户端据此判断这台服务器支不支持某个能力。
		// 老服务端不会出现 reply，客户端于是不画回复界面 —— 而不是画了却调不通。
		"features": []string{"batch", "mcp", "reply"},
	})
}
