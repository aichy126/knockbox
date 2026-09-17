package models

// 表结构体只负责列映射，不负责建表——DDL 由 internal/migrate/sql/*.sql 独占。
// 改字段时两边都要改，以迁移文件为准。

type User struct {
	Id   int64  `xorm:"'id' pk autoincr"`
	Name string `xorm:"'name'"`
	// 已读水位线：id <= 此值的消息视为已读。承载「全部已读」这类批量操作，
	// 避免把几千行 message 的 rev 全部推高。
	ReadCursor int64 `xorm:"'read_cursor'"`
	Status     int   `xorm:"'status'"`
	// 管理界面的登录凭据。纯收消息的用户这两列为空，登录不了。
	Username     string `xorm:"'username'"`
	PasswordHash string `xorm:"'password_hash'"`
	Role         string `xorm:"'role'"`
	LastLoginAt  int64  `xorm:"'last_login_at'"`
	Ctime        int64  `xorm:"'created_at'"`
	Utime        int64  `xorm:"'updated_at'"` // Unlimited 豁免公共配额。管理员身份与「不限额」是两件事，所以单独一列。
	Unlimited    int    `xorm:"'unlimited'"`
}

func (User) TableName() string { return "user" }

// Session 管理界面的登录会话。
// 存库而不是签 JWT：要能「踢掉其它设备」，也要能在改密码后立刻让旧会话失效。
type Session struct {
	Id         string `xorm:"'id' pk"` // 随机 token 的 sha256，明文只在 cookie 里
	UserId     int64  `xorm:"'user_id'"`
	UserAgent  string `xorm:"'user_agent'"`
	IP         string `xorm:"'ip'"`
	ExpiresAt  int64  `xorm:"'expires_at'"`
	Ctime      int64  `xorm:"'created_at'"`
	LastSeenAt int64  `xorm:"'last_seen_at'"`
}

func (Session) TableName() string { return "session" }

type Device struct {
	Id         int64  `xorm:"'id' pk autoincr"`
	UUID       string `xorm:"'uuid'"` // 客户端生成、存 Keychain，app 重装后不变
	UserId     int64  `xorm:"'user_id'"`
	Name       string `xorm:"'name'"`
	Platform   string `xorm:"'platform'"`
	Model      string `xorm:"'model'"`
	OSVersion  string `xorm:"'os_version'"`
	AppVersion string `xorm:"'app_version'"`
	APNsToken  string `xorm:"'apns_token'"`
	// sandbox | production。必须由客户端读 embedded provisioning profile 上报，
	// 不能用 #if DEBUG 判断——TestFlight 是 Release 构建但走 production。
	APNsEnv    string `xorm:"'apns_env'"`
	AuthHash   string `xorm:"'auth_hash'"`
	AuthPrefix string `xorm:"'auth_prefix'"`
	SyncRev    int64  `xorm:"'sync_rev'"`
	Badge      int    `xorm:"'badge'"`
	Status     int    `xorm:"'status'"`
	FailCount  int    `xorm:"'fail_count'"`
	LastPushAt int64  `xorm:"'last_push_at'"`
	LastSeenAt int64  `xorm:"'last_seen_at'"`
	Ctime      int64  `xorm:"'created_at'"`
	Utime      int64  `xorm:"'updated_at'"`
}

func (Device) TableName() string { return "device" }

// Channel 频道。服务端只存它必须知道的部分：
// token（鉴权）、muted/sound/level（组装 APNs payload 要用）。
// 名字、图标、颜色、排序在 Meta 里，服务端只存不读。
type Channel struct {
	Id     string `xorm:"'id' pk"` // app 生成的 ULID，进 payload
	UserId int64  `xorm:"'user_id'"`
	Token  string `xorm:"'token'"` // 与 Id 分开：payload 会留在通知的 userInfo 里
	Muted  int    `xorm:"'muted'"`
	// 定时静音到期时间（unix 秒）；0 = 没设。
	// 和 Muted 分开而不是用 MuteUntil=极大值 表示永久：前者是长期意图、后者会自然过期，
	// 混成一个之后界面上就分不出「我关掉了这个频道」和「我现在忙一小时」。
	MuteUntil int64  `xorm:"'mute_until'"`
	Sound     string `xorm:"'sound'"`
	Level     string `xorm:"'level'"`
	Meta      string `xorm:"'meta'"` // app 写的不透明 JSON
	// 反范式的展示字段，省掉列频道时的 N 次聚合查询。
	MsgCount   int64  `xorm:"'msg_count'"`
	LastMsgId  int64  `xorm:"'last_msg_id'"`
	LastMsgAt  int64  `xorm:"'last_msg_at'"`
	LastUsedAt int64  `xorm:"'last_used_at'"`
	LastUsedIP string `xorm:"'last_used_ip'"`
	Status     int    `xorm:"'status'"`
	Ctime      int64  `xorm:"'created_at'"`
	Utime      int64  `xorm:"'updated_at'"`
}

func (Channel) TableName() string { return "channel" }

// Silent 这一刻要不要不推。
// 定时静音过期后自动恢复，不需要任何清理任务——判定放在读侧就是为了这个。
func (c *Channel) Silent(now int64) bool {
	return c.Muted != 0 || (c.MuteUntil > 0 && c.MuteUntil > now)
}

type Message struct {
	Id  int64  `xorm:"'id' pk autoincr"`
	UID string `xorm:"'uid'"` // ULID，对外 ID
	// 全局修订号，增量同步的唯一游标。不能用 Id：Id 只对新增单调，
	// 而标记已读 / 删除会改老行，只靠 id > cursor 拉不到这些变更。
	Rev       int64  `xorm:"'rev'"`
	UserId    int64  `xorm:"'user_id'"`
	ChannelId string `xorm:"'channel_id'"`
	Type      string `xorm:"'type'"`
	Title     string `xorm:"'title'"`
	// 推送用的截断摘要，必须非空：NSE 被系统跳过时它就是用户能看到的全部。
	Summary    string `xorm:"'summary'"`
	Body       string `xorm:"'body'"` // 正文全文。不进 payload，上限由 limit.body_max_kb 决定
	Extra      string `xorm:"'extra'"`
	FileId     int64  `xorm:"'file_id'"`
	CollapseId string `xorm:"'collapse_id'"`
	IdemKey    string `xorm:"'idem_key'"`
	FromName   string `xorm:"'from_name'"`
	FromIP     string `xorm:"'from_ip'"`
	ReadAt     int64  `xorm:"'read_at'"`
	// 软删。内容在删除那一刻就被抹掉，这一行只是不含内容的空壳墓碑，
	// 供同步用；GC 在所有活跃设备同步过之后连它一起删。
	DeletedAt int64 `xorm:"'deleted_at'"`
	Ctime     int64 `xorm:"'created_at'"`
	// 用户回了什么。choice 存选中的那一项，text 存原文。
	Reply string `xorm:"'reply'"`
	// 回复的时刻。判断「回没回」看这个，不看 Reply：
	// 文本回复允许是空白（用户真的只发了空格），只看 Reply 会把那种情况误判成没回。
	RepliedAt int64 `xorm:"'replied_at'"`
	// 回复时限，0 = 不限。时限是发送方的可选项。
	ReplyUntil int64 `xorm:"'reply_until'"`
	// 回调地址。有它才说明这条消息可回。
	// 【不下发给客户端】——它是发送方的内部端点，配对过的设备没有理由看到。
	ReplyWebhook string `xorm:"'reply_webhook'"`
}

// Replyable 这条消息是不是可回复的。
// 判据是回调地址：没有地址的回复无处可去，发送时就已经被拒了。
func (m *Message) Replyable() bool { return m.ReplyWebhook != "" }

// Replied 已经回过了。看时刻不看内容，理由见 RepliedAt。
func (m *Message) Replied() bool { return m.RepliedAt != 0 }

// ReplyExpired 这一刻是不是已经过了回复时限。
// 和 Channel.Silent 一样由调用方传 now：判定放在读侧，时限过期不需要任何清理任务。
func (m *Message) ReplyExpired(now int64) bool {
	return m.ReplyUntil > 0 && now > m.ReplyUntil
}

func (Message) TableName() string { return "message" }

// ReplyHook 一条待投递的回调。
//
// url / payload / secret 都是回复那一刻的快照，投递时不回查 message：
// 保留策略随时可能把那条消息物理删掉，而回调该不该送出去在用户按下按钮时就定了。
type ReplyHook struct {
	Id        int64  `xorm:"'id' pk autoincr"`
	MessageId int64  `xorm:"'message_id'"`
	URL       string `xorm:"'url'"`
	Payload   string `xorm:"'payload'"`
	// 签名密钥的快照。用频道 token 当密钥，而 token 可以被轮换——
	// 轮换之后再投递，接收方拿新 token 验不过在旧 token 下签出来的名。
	Secret     string `xorm:"'secret'"`
	Status     int    `xorm:"'status'"`
	Attempt    int    `xorm:"'attempt'"`
	NextAt     int64  `xorm:"'next_at'"`
	StatusCode int    `xorm:"'status_code'"`
	Error      string `xorm:"'error'"`
	Ctime      int64  `xorm:"'created_at'"`
	Utime      int64  `xorm:"'updated_at'"`
}

func (ReplyHook) TableName() string { return "reply_hook" }

type File struct {
	Id        int64  `xorm:"'id' pk autoincr"`
	UID       string `xorm:"'uid'"`
	UserId    int64  `xorm:"'user_id'"`
	SHA256    string `xorm:"'sha256'"` // 内容寻址，天然去重
	Size      int64  `xorm:"'size'"`
	Mime      string `xorm:"'mime'"`
	Name      string `xorm:"'name'"`
	Width     int    `xorm:"'width'"`
	Height    int    `xorm:"'height'"`
	ThumbPath string `xorm:"'thumb_path'"`
	Storage   string `xorm:"'storage'"`
	Path      string `xorm:"'path'"`
	// 同一张图可能被多个频道的消息引用，清空频道时必须靠它判断能不能删 blob。
	RefCount int64 `xorm:"'ref_count'"`
	// 是否被任何消息引用过。用于区分「引用降回 0」（立即可回收）和
	// 「上传后还没发出去」（要等宽限期），两者的 ref_count 都是 0。
	EverReferenced int   `xorm:"'ever_referenced'"`
	Ctime          int64 `xorm:"'created_at'"`
}

func (File) TableName() string { return "file" }

type PairCode struct {
	Id        int64  `xorm:"'id' pk autoincr"`
	Code      string `xorm:"'code'"`
	UserId    int64  `xorm:"'user_id'"`
	IssuedBy  string `xorm:"'issued_by'"`
	ExpiresAt int64  `xorm:"'expires_at'"`
	UsedAt    int64  `xorm:"'used_at'"`
	UsedBy    string `xorm:"'used_by'"`
	Ctime     int64  `xorm:"'created_at'"`
}

func (PairCode) TableName() string { return "pair_code" }

// PushLog 兼做重试队列：重启后扫 status IN (0,2) 即可续推，不需要 Redis。
type PushLog struct {
	Id          int64  `xorm:"'id' pk autoincr"`
	MessageId   int64  `xorm:"'message_id'"`
	DeviceId    int64  `xorm:"'device_id'"`
	APNsId      string `xorm:"'apns_id'"`
	Status      int    `xorm:"'status'"`
	HTTPStatus  int    `xorm:"'http_status'"`
	Reason      string `xorm:"'reason'"` // APNs reason 原样保留，排障全靠它
	Attempts    int    `xorm:"'attempts'"`
	NextRetryAt int64  `xorm:"'next_retry_at'"`
	Ctime       int64  `xorm:"'created_at'"`
	Utime       int64  `xorm:"'updated_at'"`
}

func (PushLog) TableName() string { return "push_log" }

// PurgeLog 频道清空事件。硬删之后那些行不存在了，别的设备学不到「这些没了」，
// 所以把删除事件上升为频道级一条记录，一条顶掉 N 个墓碑。
type PurgeLog struct {
	Id        int64  `xorm:"'id' pk autoincr"`
	UserId    int64  `xorm:"'user_id'"`
	ChannelId string `xorm:"'channel_id'"`
	// 删「id <= 此值」而不是「全部」：清空过程中到达的新消息不会被静默吃掉。
	BeforeMsgId  int64 `xorm:"'before_msg_id'"`
	DeletedCount int64 `xorm:"'deleted_count'"`
	Rev          int64 `xorm:"'rev'"`
	Ctime        int64 `xorm:"'created_at'"`
}

func (PurgeLog) TableName() string { return "purge_log" }

type KV struct {
	K     string `xorm:"'k' pk"`
	V     string `xorm:"'v'"`
	Utime int64  `xorm:"'updated_at'"`
}

func (KV) TableName() string { return "kv" }
