package models

// DBName 是 config.toml 里 [sqlite.knockbox] 的配置名。
const DBName = "knockbox"

// 消息类型。
const (
	TypeText     = "text"
	TypeMarkdown = "markdown"
	TypeImage    = "image"
	TypeFile     = "file"
	TypeLink     = "link"
	TypeCard     = "card"
)

// 打扰级别，对应 APNs 的 interruption-level。
// 频道属性，不是单条消息的属性——推送服务不懂业务语义，无从判断什么算急。
// 要区分紧急度就建两个频道，往不同 token 发。
const (
	LevelPassive       = "passive"
	LevelActive        = "active"
	LevelTimeSensitive = "time-sensitive"
	// critical 需要向 Apple 单独申请 entitlement 且通过率不高。
	// API 允许传，但 entitlement 不具备时静默降级为 time-sensitive，不让整条推送失败。
	LevelCritical = "critical"
)

// push_log.status
const (
	PushPending = 0 // 待推
	PushOK      = 1
	PushRetry   = 2 // 可重试失败
	PushFailed  = 3 // 终态失败
	PushAbandon = 4 // 超过 max_attempts，放弃
)

// 回复类型（message.extra 里 reply.type）。
//
// 四种都只在 app 内回——通知上不回复，所以这里【没有】「哪些能上通知」的分界。
// 客户端不认识的形态只提示升级，不假装能回。
const (
	ReplyChoice = "choice"
	ReplyText   = "text"
	ReplyMulti  = "multi"
	ReplyNumber = "number"
)

// reply_hook.status。与 push_log.status 同一套语义，值也刻意对齐，
// 免得看两张表的人要在脑子里换算。
const (
	HookPending = 0 // 待投
	HookOK      = 1
	HookRetry   = 2 // 可重试失败
	HookAbandon = 3 // 超过上限，放弃
)

// device.status
const (
	DeviceLive         = 1
	DeviceLoggedOut    = 2
	DeviceUnregistered = 3 // APNs 返回 410
)

// device.apns_env
const (
	EnvSandbox    = "sandbox"
	EnvProduction = "production"
)

// user.role
const (
	RoleAdmin  = "admin"  // 能进管理界面，看所有人的东西
	RoleMember = "member" // 只是一个收消息的身份，登录不了
)

// 通用状态
const (
	StatusActive   = 1
	StatusDisabled = 2
)

// kv 表的键
const (
	// 附件签名 URL 的 HMAC 密钥，首次启动时随机生成。
	KVFileSignKey = "file_sign_key"
	// GC 硬删时记录的最高 rev。客户端的 since 比它小就必须全量重载（sync 响应里的 reset）。
	// 没有这个值，一台离线很久的设备会永久缺一块数据且毫无察觉。
	KVGCWatermark = "gc_watermark"
)
