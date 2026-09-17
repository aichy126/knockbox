package service

import (
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/uierr"
)

// Admin 管理后台的查询与写操作。
//
// 这一层收的是后台【渲染器无关】的那部分：SQL 与它的结果。服务端直出的 HTML
// 和后台的 JSON 接口读的是同一份数据，两边各写一遍 SQL 的话，一个修复就要打
// 两次补丁，而测试只能钉住其中一次。
//
// 这些查询是统计与钻取，正是 dao.Engine() 注释里列出的裸 SQL 合法用途，
// 所以这里不做仓储抽象，只把语句从 handler 里抬上来。
//
// 结果一律用带类型的结构体接，不用 QueryString()。后者返回的全是字符串，
// 调用方要自己 strconv 一遍，而列名拼错时拿到的是零值而不是错误——
// 「某个数字永远显示 0」这类缺陷不会报错，只能靠人看出来。
type Admin struct{ d *dao.DAO }

func NewAdmin(d *dao.DAO) *Admin { return &Admin{d: d} }

// ── 概览 ──────────────────────────────────────────────

// OverviewStat 概览页的全部数字。
type OverviewStat struct {
	Messages        int64
	MessagesPrev    int64
	Channels        int64
	ChannelsMuted   int64
	DevicesPushable int64
	DevicesSandbox  int64
	PushOK          int64
	PushFailed      int64
	PushRetrying    int64
}

// Rate 推送成功率。没推过时返回 ok=false —— 「成功率 0%」和「没推过」
// 在界面上是两句不同的话，合成一个数字就分不出来了。
func (o OverviewStat) Rate() (float64, bool) {
	t := o.PushOK + o.PushFailed
	if t == 0 {
		return 0, false
	}
	return float64(o.PushOK) * 100 / float64(t), true
}

// Overview 统计 since 之后的窗口。prev 是紧挨着的上一个等长窗口。
//
// uid = 0 表示【全站】，这是管理后台要的那个口径：AdminAuth 的包注释说管理员
// 看得到所有人的消息，而「24 小时消息」「推送成功率」按登录者切分之后，
// 公共实例上管理员自己那个 uid 基本没有流量——于是首页第一个数字恒为 0，
// 而服务器实际推了几万条。推送成功率更是根本不属于某个人：
// 它是这台服务器与 APNs 之间连接的健康度。
func (a *Admin) Overview(uid, since, window int64) (OverviewStat, error) {
	e := a.d.Engine()
	var o OverviewStat
	now := time.Now().Unix()
	// uid=0 时把归属条件整个去掉，而不是拿 0 去比——没有 user_id=0 这个人。
	mine, args0 := "", []any{}
	if uid != 0 {
		mine = " AND user_id=?"
		args0 = []any{uid}
	}
	joined, argsJ := "", []any{}
	if uid != 0 {
		joined = " AND m.user_id=?"
		argsJ = []any{uid}
	}
	arg := func(base []any, rest ...any) []any {
		return append(append([]any{}, base...), rest...)
	}
	for _, q := range []struct {
		dst  *int64
		sql  string
		args []any
	}{
		{&o.Messages, "SELECT COUNT(*) FROM message WHERE created_at>=?" + mine, arg([]any{since}, args0...)},
		{&o.MessagesPrev, "SELECT COUNT(*) FROM message WHERE created_at>=? AND created_at<?" + mine, arg([]any{since - window, since}, args0...)},
		{&o.Channels, "SELECT COUNT(*) FROM channel WHERE 1=1" + mine, arg(nil, args0...)},
		{&o.ChannelsMuted, "SELECT COUNT(*) FROM channel WHERE (muted<>0 OR mute_until>?)" + mine, arg([]any{now}, args0...)},
		{&o.DevicesPushable, "SELECT COUNT(*) FROM device WHERE apns_token<>''" + mine, arg(nil, args0...)},
		{&o.DevicesSandbox, "SELECT COUNT(*) FROM device WHERE apns_env='sandbox'" + mine, arg(nil, args0...)},
		{&o.PushOK, `SELECT COUNT(*) FROM push_log p JOIN message m ON m.id=p.message_id
		             WHERE p.created_at>=? AND p.status=1` + joined, arg([]any{since}, argsJ...)},
		{&o.PushFailed, `SELECT COUNT(*) FROM push_log p JOIN message m ON m.id=p.message_id
		                 WHERE p.created_at>=? AND p.status=3` + joined, arg([]any{since}, argsJ...)},
		{&o.PushRetrying, `SELECT COUNT(*) FROM push_log p JOIN message m ON m.id=p.message_id
		                   WHERE p.created_at>=? AND p.status IN (0,2)` + joined, arg([]any{since}, argsJ...)},
	} {
		if _, err := e.SQL(q.sql, q.args...).Get(q.dst); err != nil {
			return o, err
		}
	}
	return o, nil
}

// FailureRow 推送失败的一种。按 reason + http_status 聚合。
type FailureRow struct {
	Reason     string `xorm:"'reason'"`
	HTTPStatus int    `xorm:"'http_status'"`
	Count      int64  `xorm:"'n'"`
}

func (a *Admin) Failures(uid, since int64, limit int) ([]FailureRow, error) {
	mine, args := "", []any{since}
	if uid != 0 {
		mine = " AND m.user_id=?"
		args = append(args, uid)
	}
	args = append(args, limit)
	var out []FailureRow
	err := a.d.Engine().SQL(`
		SELECT COALESCE(NULLIF(p.reason,''),'(无原因)') AS reason, p.http_status, COUNT(*) AS n
		FROM push_log p JOIN message m ON m.id=p.message_id
		WHERE p.created_at>=? AND p.status=3`+mine+`
		GROUP BY reason, p.http_status ORDER BY n DESC LIMIT ?`, args...).Find(&out)
	return out, err
}

// ── 消息 ──────────────────────────────────────────────

// MessageRow 后台看到的一条消息。
//
// Body / Extra 只在需要正文的地方查（消息流、消息详情），列表不带——
// 一页 40 条正文是几百 KB，而列表一个字都不显示它。
type MessageRow struct {
	Id        int64  `xorm:"'id'"`
	UID       string `xorm:"'uid'"`
	UserId    int64  `xorm:"'user_id'"`
	Owner     string `xorm:"'owner'"` // user.name
	ChannelId string `xorm:"'channel_id'"`
	Meta      string `xorm:"'meta'"` // channel.meta，取名字用
	Type      string `xorm:"'type'"`
	Title     string `xorm:"'title'"`
	Summary   string `xorm:"'summary'"`
	Body      string `xorm:"'body'"`
	Extra     string `xorm:"'extra'"`
	Ctime     int64  `xorm:"'created_at'"`
	ReadAt    int64  `xorm:"'read_at'"`

	ReplyWebhook string `xorm:"'reply_webhook'"`
	Reply        string `xorm:"'reply'"`
	RepliedAt    int64  `xorm:"'replied_at'"`
	ReplyUntil   int64  `xorm:"'reply_until'"`

	PushTotal int64 `xorm:"'total'"`
	PushOK    int64 `xorm:"'ok'"`
}

// MessageFilter 消息检索的条件。零值 = 不筛。
type MessageFilter struct {
	Query     string // 匹配 title / summary / body
	UserId    int64
	ChannelId string
	Before    int64 // 游标：只取 id 比它小的
	Limit     int
	WithBody  bool // 要不要带 body / extra
}

const pushCounts = `(SELECT COUNT(*) FROM push_log p WHERE p.message_id=m.id) AS total,
	       (SELECT COUNT(*) FROM push_log p WHERE p.message_id=m.id AND p.status=1) AS ok`

// Messages 按条件检索，按 id 倒序。消息搜索与频道消息流共用这一个。
func (a *Admin) Messages(f MessageFilter) ([]MessageRow, error) {
	cols := `m.id, m.uid, m.user_id, m.channel_id, m.type, m.title, m.summary,
	          m.created_at, m.read_at,`
	if f.WithBody {
		cols += ` m.body, m.extra, m.reply_webhook, m.reply, m.replied_at, m.reply_until,`
	}
	sql := `SELECT ` + cols + pushCounts + `,
	       (SELECT meta FROM channel c WHERE c.id=m.channel_id) AS meta,
	       (SELECT name FROM user u WHERE u.id=m.user_id) AS owner
	     FROM message m WHERE m.deleted_at=0`
	var args []any
	if f.UserId != 0 {
		sql += " AND m.user_id=?"
		args = append(args, f.UserId)
	}
	if f.ChannelId != "" {
		sql += " AND m.channel_id=?"
		args = append(args, f.ChannelId)
	}
	if f.Query != "" {
		// LIKE 够用：万级数据量下它比引入 FTS5 的复杂度划算得多。
		// 真到十万级再说——那时是另一个问题，不该现在预支。
		sql += " AND (m.title LIKE ? OR m.summary LIKE ? OR m.body LIKE ?)"
		like := "%" + f.Query + "%"
		args = append(args, like, like, like)
	}
	if f.Before > 0 {
		sql += " AND m.id<?"
		args = append(args, f.Before)
	}
	sql += " ORDER BY m.id DESC LIMIT ?"
	args = append(args, f.Limit)

	var out []MessageRow
	// SQL 与参数分开传。QueryString(...any) 那个变参形态会把语句和用户输入
	// 塞进同一个切片，静态分析分不出哪个是语句，人读起来也一样。
	err := a.d.Engine().SQL(sql, args...).Find(&out)
	return out, err
}

// RecentMessages 概览页的「最近消息」。uid = 0 表示全站，理由同 Overview。
func (a *Admin) RecentMessages(uid int64, limit int) ([]MessageRow, error) {
	mine, args := "", []any{}
	if uid != 0 {
		mine = " AND m.user_id=?"
		args = append(args, uid)
	}
	args = append(args, limit)
	var out []MessageRow
	err := a.d.Engine().SQL(`
		SELECT m.id, m.uid, m.user_id, m.channel_id, m.type, m.title, m.summary, m.created_at, m.read_at,
		       `+pushCounts+`,
		       (SELECT meta FROM channel c WHERE c.id=m.channel_id) AS meta,
		       (SELECT name FROM user u WHERE u.id=m.user_id) AS owner
		FROM message m WHERE m.deleted_at=0`+mine+`
		ORDER BY m.id DESC LIMIT ?`, args...).Find(&out)
	return out, err
}

// Message 一条消息的详情，连同它所属的频道。
//
// 【不按 user_id 过滤】：消息搜索页刻意跨全站（管理界面的定位就是看到这台服务器上
// 的全部消息），详情页若只认自己的，从列表点进别人的消息就是一条死链——列出来了、
// 点不开。权限模型由 middleware.AdminAuth 的包注释定义，它说的就是全权限。
func (a *Admin) Message(uid string) (*models.Message, *models.Channel, error) {
	var m models.Message
	ok, err := a.d.Engine().Where("uid=?", uid).Get(&m)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, uierr.New(uierr.MessageNotFound)
	}
	var ch models.Channel
	if _, err := a.d.Engine().Where("id=?", m.ChannelId).Get(&ch); err != nil {
		return nil, nil, err
	}
	return &m, &ch, nil
}

// PushLogRow 一次投递尝试。
type PushLogRow struct {
	Status     int    `xorm:"'status'"`
	HTTPStatus int    `xorm:"'http_status'"`
	Reason     string `xorm:"'reason'"`
	Attempts   int    `xorm:"'attempts'"`
	APNsID     string `xorm:"'apns_id'"`
	Utime      int64  `xorm:"'updated_at'"`
	Device     string `xorm:"'name'"`
	APNsEnv    string `xorm:"'apns_env'"`
}

func (a *Admin) PushLog(msgID int64) ([]PushLogRow, error) {
	var out []PushLogRow
	err := a.d.Engine().SQL(`
		SELECT p.status, p.http_status, p.reason, p.attempts, p.apns_id, p.updated_at,
		       d.name, d.apns_env
		FROM push_log p LEFT JOIN device d ON d.id=p.device_id
		WHERE p.message_id=? ORDER BY p.id`, msgID).Find(&out)
	return out, err
}

// ReplyHookRow 一次回调投递。
type ReplyHookRow struct {
	Status     int    `xorm:"'status'"`
	Attempt    int    `xorm:"'attempt'"`
	StatusCode int    `xorm:"'status_code'"`
	Error      string `xorm:"'error'"`
	Utime      int64  `xorm:"'updated_at'"`
}

func (a *Admin) ReplyHooks(msgID int64) ([]ReplyHookRow, error) {
	var out []ReplyHookRow
	err := a.d.Engine().SQL(`
		SELECT status, attempt, status_code, error, updated_at
		FROM reply_hook WHERE message_id=? ORDER BY id`, msgID).Find(&out)
	return out, err
}

// ── 成员 ──────────────────────────────────────────────

// MemberRow 成员列表里的一行。
type MemberRow struct {
	Id          int64  `xorm:"'id'"`
	Name        string `xorm:"'name'"`
	Role        string `xorm:"'role'"`
	Status      int    `xorm:"'status'"`
	LastLoginAt int64  `xorm:"'last_login_at'"`
	Ctime       int64  `xorm:"'created_at'"`
	Unlimited   int    `xorm:"'unlimited'"`
	Devices     int64  `xorm:"'devices'"`
	Channels    int64  `xorm:"'channels'"`
	Messages    int64  `xorm:"'msgs'"`
}

// MemberFilter 成员列表的条件。
//
// Cursor 与 Offset 二选一：JSON 接口走游标，服务端直出的那个分页器还在用
// offset。offset 会静默丢行——翻页时有人注册，第二页就跳过一行，而翻页的人
// 什么都看不出来；所以新的那套不再用它。
type MemberFilter struct {
	Query  string
	Cursor int64 // u.id > Cursor
	Offset int
	Limit  int
}

// Members 成员列表。total 是匹配到的总数，不是全表总数——分页器靠它算页数。
func (a *Admin) Members(f MemberFilter) (rows []MemberRow, total int64, err error) {
	where, args := "", []any{}
	if f.Query != "" {
		where = " WHERE u.name LIKE ?"
		args = append(args, "%"+f.Query+"%")
	}
	if _, err = a.d.Engine().SQL("SELECT COUNT(*) FROM user u"+where, args...).Get(&total); err != nil {
		return nil, 0, err
	}
	if f.Cursor > 0 {
		if where == "" {
			where = " WHERE u.id>?"
		} else {
			where += " AND u.id>?"
		}
		args = append(args, f.Cursor)
	}
	sql := `SELECT u.id, u.name, u.role, u.status, u.last_login_at, u.created_at, u.unlimited,
	          (SELECT COUNT(*) FROM device d WHERE d.user_id=u.id) AS devices,
	          (SELECT COUNT(*) FROM channel c WHERE c.user_id=u.id) AS channels,
	          (SELECT COUNT(*) FROM message m WHERE m.user_id=u.id AND m.deleted_at=0) AS msgs
	        FROM user u` + where + " ORDER BY u.id LIMIT ?"
	args = append(args, f.Limit)
	if f.Cursor == 0 && f.Offset > 0 {
		sql += " OFFSET ?"
		args = append(args, f.Offset)
	}
	err = a.d.Engine().SQL(sql, args...).Find(&rows)
	return rows, total, err
}

// Member 一个成员。
func (a *Admin) Member(id int64) (*models.User, error) {
	var u models.User
	ok, err := a.d.Engine().ID(id).Get(&u)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, uierr.New(uierr.MemberNotFound)
	}
	return &u, nil
}

// NameRow id ↔ 名字。成员下拉与筛选器用。
type NameRow struct {
	Id   int64  `xorm:"'id'"`
	Name string `xorm:"'name'"`
}

func (a *Admin) MemberNames() ([]NameRow, error) {
	var out []NameRow
	err := a.d.Engine().SQL("SELECT id, name FROM user ORDER BY id").Find(&out)
	return out, err
}

// MemberIdByName 按名字反查。空名字或查不到都返回 0 = 不筛选。
//
// 查不到当作没筛选而不是返回空列表：名字打错时给一个空列表，
// 用户会以为「这个人没有消息」。
func (a *Admin) MemberIdByName(name string) int64 {
	if name == "" {
		return 0
	}
	var id int64
	_, _ = a.d.Engine().SQL("SELECT id FROM user WHERE name=?", name).Get(&id)
	return id
}

// MemberName 一个成员的名字。查不到返回空串——它只用在展示上。
func (a *Admin) MemberName(id int64) string {
	var name string
	_, _ = a.d.Engine().SQL("SELECT name FROM user WHERE id=?", id).Get(&name)
	return name
}

// CreateMember 建一个纯收件人：没有用户名密码，登录不了后台。
func (a *Admin) CreateMember(name string) (*models.User, error) {
	now := time.Now().Unix()
	u := &models.User{Name: name, Role: models.RoleMember,
		Status: models.StatusActive, Ctime: now, Utime: now}
	if _, err := a.d.Engine().Insert(u); err != nil {
		return nil, err
	}
	return u, nil
}

// SetUnlimited 配额豁免。
//
// 影响 0 行【不是成功】：成员可能已经被删了，或者这是一个开着的旧标签页。
// 原来这里是 `_, _ = Exec(...)` 然后无条件重定向，于是 DB 出错、id 不存在、
// 真的改成功了，三种结果在界面上长得一模一样。
//
// 两种失败分开：DB 真错了是内部错误（调用方记日志、当 500），
// 影响 0 行是一个用户能理解的失败，给他一个 code。
func (a *Admin) SetUnlimited(id int64, on bool) error {
	v := 0
	if on {
		v = 1
	}
	res, err := a.d.Engine().Exec("UPDATE user SET unlimited=?, updated_at=? WHERE id=?",
		v, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return uierr.New(uierr.MemberNotFound)
	}
	return nil
}

// ── 频道 ──────────────────────────────────────────────

// ChannelRow 成员详情里的频道一行。
type ChannelRow struct {
	Id        string `xorm:"'id'"`
	UserId    int64  `xorm:"'user_id'"`
	Meta      string `xorm:"'meta'"`
	Muted     int    `xorm:"'muted'"`
	MuteUntil int64  `xorm:"'mute_until'"`
	Sound     string `xorm:"'sound'"`
	Level     string `xorm:"'level'"`
	Messages  int64  `xorm:"'n'"`
	LastMsgAt int64  `xorm:"'last'"`
}

func (a *Admin) MemberChannels(uid int64) ([]ChannelRow, error) {
	var out []ChannelRow
	err := a.d.Engine().SQL(`
		SELECT c.id, c.meta, c.muted, c.mute_until, c.sound, c.level,
		       (SELECT COUNT(*) FROM message m WHERE m.channel_id=c.id AND m.deleted_at=0) AS n,
		       (SELECT COALESCE(MAX(created_at),0) FROM message m WHERE m.channel_id=c.id) AS last
		FROM channel c WHERE c.user_id=? ORDER BY c.created_at`, uid).Find(&out)
	return out, err
}

// Channel 频道及其属主。
func (a *Admin) Channel(id string) (*models.Channel, *models.User, error) {
	var ch models.Channel
	ok, err := a.d.Engine().Where("id=?", id).Get(&ch)
	if err != nil {
		return nil, nil, err
	}
	if !ok {
		return nil, nil, uierr.New(uierr.ChannelNotFound)
	}
	var owner models.User
	if _, err := a.d.Engine().ID(ch.UserId).Get(&owner); err != nil {
		return nil, nil, err
	}
	return &ch, &owner, nil
}

// ChannelMaxMsgId 清空频道时的快照上界。
func (a *Admin) ChannelMaxMsgId(id string) (int64, error) {
	var maxID int64
	_, err := a.d.Engine().SQL("SELECT COALESCE(MAX(id),0) FROM message WHERE channel_id=?", id).Get(&maxID)
	return maxID, err
}

// AllChannels 搜索页的频道下拉：一次全带出来，在前端按成员过滤。
func (a *Admin) AllChannels() ([]ChannelRow, error) {
	var out []ChannelRow
	err := a.d.Engine().SQL(
		"SELECT id, user_id, meta FROM channel ORDER BY user_id, created_at").Find(&out)
	return out, err
}

// ── 设备 ──────────────────────────────────────────────

// DeviceRow 成员详情里的设备一行。APNs token 本身不出这一层。
type DeviceRow struct {
	Id         int64  `xorm:"'id'"`
	Name       string `xorm:"'name'"`
	Platform   string `xorm:"'platform'"`
	Model      string `xorm:"'model'"`
	OSVersion  string `xorm:"'os_version'"`
	AppVersion string `xorm:"'app_version'"`
	APNsEnv    string `xorm:"'apns_env'"`
	APNsToken  string `xorm:"'apns_token'"`
	LastSeenAt int64  `xorm:"'last_seen_at'"`
	SyncRev    int64  `xorm:"'sync_rev'"`
	Status     int    `xorm:"'status'"`
}

// CanPush 有 token 才收得到推送。
func (d DeviceRow) CanPush() bool { return d.APNsToken != "" }

func (a *Admin) MemberDevices(uid int64) ([]DeviceRow, error) {
	var out []DeviceRow
	err := a.d.Engine().SQL(`
		SELECT id, name, platform, model, os_version, app_version, apns_env,
		       apns_token, last_seen_at, sync_rev, status
		FROM device WHERE user_id=? ORDER BY created_at DESC`, uid).Find(&out)
	return out, err
}

// RevokeDevice 后台注销一台设备，返回它属于谁（注销完要回那个成员的详情页）。
//
// 走的是和 app 侧 DELETE /api/v1/devices/:uuid 同一条路——Device.Logout，
// 软登出而不是删行。硬删会带来两件看不见的坏事：
//
//   - push_log.device_id 悬空，pushLogTable 的 LEFT JOIN device 从此显示空名字，
//     那台手机的历史投递记录在界面上就消失了；
//   - middleware.DeviceAuth 专门为 DeviceLoggedOut 留的那句「这台设备被登出了，
//     请重新配对」失效，被删掉的设备再来只拿到泛泛的 invalid device token。
//
// 同一个动作在两条路径上有两种物理效果，是这个仓库反复防的那类不一致。
//
// 【不按当前管理员过滤】：后台是全权限的，成员详情页上那颗「注销」按钮
// 点的就是别人的设备，带上 user_id=<登录者> 的话它永远静默无效。
func (a *Admin) RevokeDevice(deviceID int64) (owner int64, err error) {
	var dev models.Device
	ok, err := a.d.Engine().ID(deviceID).Get(&dev)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, uierr.New(uierr.DeviceNotFound)
	}
	if err := NewDevice(a.d).Logout(dev.UserId, dev.UUID); err != nil {
		return dev.UserId, uierr.Wrap(err, uierr.DeviceNotFound)
	}
	return dev.UserId, nil
}
