package web

// AdminTexts 管理界面（/login 与 /admin/*）的每一句话。
//
// 按页分组而不是摊平成两百个字段：找一句话的时候，「它在哪一页」
// 永远比「它叫什么名字」好回忆。加一门语言仍然只是往 locales/ 放一个
// JSON 文件——没翻的字段回落成英文，半翻完的语料照样能合。
type AdminTexts struct {
	Nav      AdminNav      `json:"nav"`      // 侧栏导航与顶栏。
	Common   AdminCommon   `json:"common"`   // 各页反复出现的字：表头、按钮、状态徽章。
	Time     AdminTime     `json:"time"`     // 相对时间。这几句会出现在每一张表里。
	Login    AdminLogin    `json:"login"`    // /login。
	Dash     AdminDash     `json:"dash"`     // /admin 概览页。
	Member   AdminMember   `json:"member"`   // /admin/users/:id 成员详情。
	Channel  AdminChannel  `json:"channel"`  // /admin/channels/:id 频道详情。
	Mute     AdminMute     `json:"mute"`     // 静音状态。频道表和频道页都用。
	Msgs     AdminMessages `json:"msgs"`     // /admin/messages 搜索与消息详情。
	Users    AdminUsers    `json:"users"`    // /admin/users 成员列表。
	Settings AdminSettings `json:"settings"` // /admin/settings。
	Pair     AdminPair     `json:"pair"`     // /admin/pair 配对设备。
}

// AdminNav 侧栏导航与顶栏。
type AdminNav struct {
	Dash     string `json:"dash"`
	Users    string `json:"users"`
	Messages string `json:"messages"`
	Pair     string `json:"pair"`
	Settings string `json:"settings"`
	Logout   string `json:"logout"`
	Online   string `json:"online"` // 侧栏底部的运行状态
	Offline  string `json:"offline"`
}

// AdminCommon 各页反复出现的字：表头、按钮、状态徽章。
type AdminCommon struct {
	Time         string `json:"time"` // 表头
	Member       string `json:"member"`
	Channel      string `json:"channel"`
	Type         string `json:"type"`
	Title        string `json:"title"`
	Push         string `json:"push"`
	Name         string `json:"name"`
	Role         string `json:"role"`
	Device       string `json:"device"`
	Messages     string `json:"messages"`
	Status       string `json:"status"`
	Attempts     string `json:"attempts"`
	Reason       string `json:"reason"`
	Search       string `json:"search"` // 按钮
	Clear        string `json:"clear"`
	Save         string `json:"save"`
	NextPage     string `json:"next_page"`
	Older        string `json:"older"`
	Unread       string `json:"unread"` // 徽章
	RoleAdmin    string `json:"role_admin"`
	RoleMember   string `json:"role_member"`
	NotFound     string `json:"not_found"`  // 面包屑最后一节
	PeopleOne    string `json:"people_one"` // 英语的 1 和其余不一样，中文两条写成一样的
	PeopleCount  string `json:"people_count"`
	Delivered    string `json:"delivered"` // 投递状态
	Retrying     string `json:"retrying"`
	Queued       string `json:"queued"`
	GivenUp      string `json:"given_up"`
	Failed       string `json:"failed"`
	NotPushed    string `json:"not_pushed"`
	Dash         string `json:"dash"` // 没有值时的占位
	PwChangeFail string `json:"pw_change_fail"`
	NoReason     string `json:"no_reason"` // APNs 没给失败原因时
	ActionFail   string `json:"action_fail"`
}

// AdminTime 相对时间。这几句会出现在每一张表里。
type AdminTime struct {
	JustNow    string `json:"just_now"`
	MinutesAgo string `json:"minutes_ago"`
	HoursAgo   string `json:"hours_ago"`
	DaysAgo    string `json:"days_ago"`
	Today      string `json:"today"` // 消息流里的日期分隔
	Yesterday  string `json:"yesterday"`
	DateFormat string `json:"date_format"` // Go 的时间版式，按本语言的习惯写
}

// AdminLogin /login。
type AdminLogin struct {
	Title    string `json:"title"` // <title> 与按钮
	Username string `json:"username"`
	Password string `json:"password"`
	Bad      string `json:"bad"`
	Changed  string `json:"changed"`
	NoSignup string `json:"no_signup"`
}

// AdminDash /admin 概览页。
type AdminDash struct {
	Title        string `json:"title"`
	Last24h      string `json:"last24h"` // 接在服务器名后面
	Msgs24h      string `json:"msgs24h"`
	PrevDay      string `json:"prev_day"`
	PushRate     string `json:"push_rate"`
	FailRetry    string `json:"fail_retry"`
	Reachable    string `json:"reachable"`
	SandboxCount string `json:"sandbox_count"`
	Channels     string `json:"channels"`
	MutedCount   string `json:"muted_count"`
	Recent       string `json:"recent"`
	ViewAll      string `json:"view_all"`
	Failures24h  string `json:"failures24h"`
	NoFailures   string `json:"no_failures"`
	Empty        string `json:"empty"`
	UnregNote    string `json:"unreg_note"`
}

// AdminMember /admin/users/:id 成员详情。
type AdminMember struct {
	CreatedOn    string `json:"created_on"`
	StatChannels string `json:"stat_channels"` // 用量方块
	StatNow      string `json:"stat_now"`
	StatMsgs24h  string `json:"stat_msgs24h"`
	StatRolling  string `json:"stat_rolling"`
	StatTotal    string `json:"stat_total"`
	StatKept     string `json:"stat_kept"`
	StatFiles    string `json:"stat_files"`
	StatDerived  string `json:"stat_derived"`
	CardChannels string `json:"card_channels"` // 卡片标题
	CardDevices  string `json:"card_devices"`
	PairNew      string `json:"pair_new"`
	NoChannels   string `json:"no_channels"`
	NoDevices    string `json:"no_devices"`
	ColLast      string `json:"col_last"` // 频道表
	ColSound     string `json:"col_sound"`
	ColLevel     string `json:"col_level"`
	ColOS        string `json:"col_os"` // 设备表
	ColApp       string `json:"col_app"`
	ColSynced    string `json:"col_synced"`
	ColSeen      string `json:"col_seen"`
	StateOK      string `json:"state_ok"`
	NoPush       string `json:"no_push"`
	Revoke       string `json:"revoke"`
	RevokeAsk    string `json:"revoke_ask"` // confirm() 弹窗
}

// AdminChannel /admin/channels/:id 频道详情。
type AdminChannel struct {
	BelongsTo    string `json:"belongs_to"`
	SendGuide    string `json:"send_guide"`
	Props        string `json:"props"`
	Purge        string `json:"purge"`
	PurgeAsk     string `json:"purge_ask"` // confirm() 弹窗
	PropId       string `json:"prop_id"`
	PropToken    string `json:"prop_token"`
	PropUsed     string `json:"prop_used"`
	Empty        string `json:"empty"`
	OlderSearch  string `json:"older_search"`
	DeliveryLog  string `json:"delivery_log"`
	UnknownReply string `json:"unknown_reply"`
	WaitingText  string `json:"waiting_text"` // 占位
	Replied      string `json:"replied"`
	Expired      string `json:"expired"`
	WaitingTill  string `json:"waiting_till"`
	WaitingOpen  string `json:"waiting_open"`
	MatchOne     string `json:"match_one"` // 搜索命中数，英语的 1 和其余不一样
	MatchCount   string `json:"match_count"`
}

// AdminMute 静音状态。频道表和频道页都用。
type AdminMute struct {
	Always string `json:"always"`
	Until  string `json:"until"`
}

// AdminMessages /admin/messages 搜索与消息详情。
type AdminMessages struct {
	Sub           string `json:"sub"`
	SearchPh      string `json:"search_ph"` // 输入框占位
	AllMembers    string `json:"all_members"`
	AllChannels   string `json:"all_channels"`
	EmptyQuery    string `json:"empty_query"`
	Crumb         string `json:"crumb"` // 面包屑
	CardBody      string `json:"card_body"`
	CardExtra     string `json:"card_extra"`
	CardSummary   string `json:"card_summary"`
	CardReply     string `json:"card_reply"`
	ReplyExpired  string `json:"reply_expired"` // 徽章
	ReplyExpSub   string `json:"reply_exp_sub"`
	ReplyWaiting  string `json:"reply_waiting"`
	ReplyUntil    string `json:"reply_until"`
	ReplyNoLimit  string `json:"reply_no_limit"`
	ReplyWebhook  string `json:"reply_webhook"`
	ReplyNoHooks  string `json:"reply_no_hooks"`
	ColHook       string `json:"col_hook"`
	ColEnv        string `json:"col_env"`
	NoPushLog     string `json:"no_push_log"`
	DeliveredCode string `json:"delivered_code"`
}

// AdminUsers /admin/users 成员列表。
type AdminUsers struct {
	Sub         string `json:"sub"`
	SearchPh    string `json:"search_ph"`
	Clear       string `json:"clear"`
	EmptyQuery  string `json:"empty_query"`
	EmptyAll    string `json:"empty_all"`
	ColLogin    string `json:"col_login"`
	ColQuota    string `json:"col_quota"`
	Note        string `json:"note"`
	Unlimited   string `json:"unlimited"`
	UnlimitedOn string `json:"unlimited_on"`
}

// AdminSettings /admin/settings。
type AdminSettings struct {
	Sub           string `json:"sub"`
	TabPublic     string `json:"tab_public"`
	TabServer     string `json:"tab_server"`
	Saved         string `json:"saved"`
	DotTitle      string `json:"dot_title"` // 小圆点的 title
	PublicMode    string `json:"public_mode"`
	PublicHint    string `json:"public_hint"`
	RegPerHour    string `json:"reg_per_hour"`
	RegHint       string `json:"reg_hint"`
	SiteName      string `json:"site_name"`
	SiteHint      string `json:"site_hint"`
	CardQuota     string `json:"card_quota"`
	MaxChannels   string `json:"max_channels"`
	MaxChHint     string `json:"max_ch_hint"`
	MaxPerDay     string `json:"max_per_day"`
	MaxDayHint    string `json:"max_day_hint"`
	Retention     string `json:"retention"`
	RetHint       string `json:"ret_hint"`
	QuotaNote     string `json:"quota_note"`
	SaveHint      string `json:"save_hint"` // %s 是那个小圆点
	ModeSelf      string `json:"mode_self"`
	ModePublic    string `json:"mode_public"`
	InfoName      string `json:"info_name"`
	InfoURL       string `json:"info_url"`
	InfoVersion   string `json:"info_version"`
	InfoMode      string `json:"info_mode"`
	CardServer    string `json:"card_server"`
	CardUsage     string `json:"card_usage"`
	UsageChannels string `json:"usage_channels"`
	UsageToday    string `json:"usage_today"`
	UsageTotal    string `json:"usage_total"`
	UsageFiles    string `json:"usage_files"`
	PwCard        string `json:"pw_card"`
	PwCurrent     string `json:"pw_current"`
	PwNew         string `json:"pw_new"`
	PwConfirm     string `json:"pw_confirm"`
	PwNote        string `json:"pw_note"`
	PwSubmit      string `json:"pw_submit"`
	CardCLI       string `json:"card_cli"`
	CLIIntro      string `json:"cli_intro"`
	CLIAdd        string `json:"cli_add"`
	CLIPasswd     string `json:"cli_passwd"`
	CLIList       string `json:"cli_list"`
	CLIDisable    string `json:"cli_disable"`
	SaveFailed    string `json:"save_failed"`
}

// AdminPair /admin/pair 配对设备。
type AdminPair struct {
	Sub        string `json:"sub"`
	CreateFail string `json:"create_fail"`
	IssueFail  string `json:"issue_fail"`
	NamePh     string `json:"name_ph"` // 输入框占位
	SendTo     string `json:"send_to"`
	Issue      string `json:"issue"`
	Hint       string `json:"hint"`
	CardWho    string `json:"card_who"`
	CardScan   string `json:"card_scan"`
	QRFail     string `json:"qr_fail"`
	Step1      string `json:"step1"`
	Step2      string `json:"step2"` // 和 app 里那个按钮的名字一致
	Step3      string `json:"step3"`
	Note       string `json:"note"`
}
