package service

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/aichy126/igo/log"
	"github.com/aichy126/knockbox/internal/dao"
	kapns "github.com/aichy126/knockbox/internal/library/apns"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/sideshow/apns2"
)

// 退避梯度。超过 maxAttempts 次置「已放弃」。
var backoff = []time.Duration{5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute, 30 * time.Minute}

// withJitter 在退避时长上下浮动 20%。
//
// APNs 返 503 或 429 时，同一批请求是一起失败的；固定梯度意味着它们又会在
// 同一刻一起回来，把刚缓过来的服务再压一次。打散之后重试是铺开的。
func withJitter(d time.Duration) time.Duration {
	return d + time.Duration((rand.Float64()*0.4-0.2)*float64(d))
}

// Pusher 消费 push_log 队列。
//
// push_log 本身就是队列：服务重启后扫 status IN (0,2) 即可续推，不丢消息，
// 也不用引入 Redis。这是 SQLite 在单机自建场景的最大红利。
//
// ⚠️ **同一个数据库上只能跑一个实例。** claim 只是把到期的记录取出来，
// 没有加租约，靠「一个进程只有一条 Run 循环」来保证不重复投递。
// 两个进程指向同一个库的话，每条消息会被推两遍。
// 要横向扩就得先给 push_log 加租约列（认领者 + 认领到期时间），现在没有。
type Pusher struct {
	d           *dao.DAO
	cl          *kapns.Client
	concurrency int
	maxAttempts int
	expiration  time.Duration
	timeout     time.Duration
	wake        chan struct{}
	files       *File
}

type PusherConfig struct {
	Concurrency int
	MaxAttempts int
	Expiration  time.Duration
	Timeout     time.Duration
}

func NewPusher(d *dao.DAO, cl *kapns.Client, files *File, cfg PusherConfig) *Pusher {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 8
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 5
	}
	if cfg.Expiration <= 0 {
		cfg.Expiration = 24 * time.Hour
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	return &Pusher{
		d: d, cl: cl, concurrency: cfg.Concurrency, maxAttempts: cfg.MaxAttempts,
		expiration: cfg.Expiration, timeout: cfg.Timeout, wake: make(chan struct{}, 1),
		files: files,
	}
}

// Notify 叫醒一轮。落库之后调它，新消息立刻被取走而不是等下一轮扫描。
func (p *Pusher) Notify() {
	select {
	case p.wake <- struct{}{}:
	default: // 已经有人在叫了，不用排队
	}
}

// Run 常驻循环。30 秒兜底扫一次，处理重试到期和「服务重启前没推完」的。
func (p *Pusher) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		if n, err := p.drain(ctx); err != nil {
			log.Error("推送队列处理失败", log.Any("error", err.Error()))
		} else if n > 0 {
			log.Info("推送完成", log.Any("count", n))
		}
		select {
		case <-ctx.Done():
			return
		case <-p.wake:
		case <-t.C:
		}
	}
}

type job struct {
	logID   int64
	attempt int
	msg     models.Message
	dev     models.Device
	ch      models.Channel
}

// drain 把当前到期的都推完。
func (p *Pusher) drain(ctx context.Context) (int, error) {
	total := 0
	for {
		jobs, err := p.claim(200)
		if err != nil || len(jobs) == 0 {
			return total, err
		}
		// 信号量 + WaitGroup，不用 errgroup：deliver 从不返回错误
		// （每条的失败都记在它自己那行 push_log 上），errgroup 的取消语义
		// 一次都用不上，留着它只会让人以为「有一条失败会中断整批」。
		sem := make(chan struct{}, p.concurrency)
		var wg sync.WaitGroup
		for i := range jobs {
			j := jobs[i]
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				p.deliver(ctx, j)
			}()
		}
		wg.Wait()
		total += len(jobs)
		if ctx.Err() != nil {
			return total, nil
		}
	}
}

func (p *Pusher) claim(limit int) ([]job, error) {
	var rows []claimRow
	if err := p.d.Engine().SQL(claimSQL,
		models.PushPending, models.PushRetry, time.Now().Unix(), limit).Find(&rows); err != nil {
		return nil, err
	}
	out := make([]job, 0, len(rows))
	for _, r := range rows {
		// 关联记录不在了（消息被保留策略清掉、设备被删、频道被删）时，
		// 必须把这条置终态。留着不动的话它会永远停在 pending，
		// 每一轮兜底扫描都把它捞出来再丢掉。
		switch {
		case r.MsgID == 0:
			p.discard(r.LogID, "消息记录已不存在")
		case r.DevID == 0:
			p.discard(r.LogID, "设备记录已不存在")
		case r.ChID == "":
			p.discard(r.LogID, "频道记录已不存在")
		default:
			out = append(out, r.job())
		}
	}
	return out, nil
}

// claimRow 一批投递任务所需的全部字段，一次查询取回。
//
// 原先是先 JOIN 出几个 id、再逐条 Get 出 message / device / channel：
// 一批 200 条要跑 601 次查询。
//
// 三张表都用 LEFT JOIN 而不是 INNER：关联记录缺失时这一行仍要返回，
// 上面才有机会把它置终态。用 INNER 的话它压根不出现在结果里，
// 于是永远留在 pending —— 那正是这段逻辑要消除的状态。
type claimRow struct {
	LogID    int64 `xorm:"'log_id'"`
	Attempts int   `xorm:"'attempts'"`

	MsgID       int64  `xorm:"'msg_id'"`
	MsgUID      string `xorm:"'msg_uid'"`
	MsgType     string `xorm:"'msg_type'"`
	MsgTitle    string `xorm:"'msg_title'"`
	MsgSummary  string `xorm:"'msg_summary'"`
	MsgBody     string `xorm:"'msg_body'"`
	MsgExtra    string `xorm:"'msg_extra'"`
	MsgRev      int64  `xorm:"'msg_rev'"`
	MsgCollapse string `xorm:"'msg_collapse'"`
	MsgCtime    int64  `xorm:"'msg_ctime'"`
	MsgChannel  string `xorm:"'msg_channel'"`

	DevID    int64  `xorm:"'dev_id'"`
	DevUUID  string `xorm:"'dev_uuid'"`
	DevToken string `xorm:"'dev_token'"`
	DevEnv   string `xorm:"'dev_env'"`
	DevUtime int64  `xorm:"'dev_utime'"`

	ChID    string `xorm:"'ch_id'"`
	ChLevel string `xorm:"'ch_level'"`
	ChSound string `xorm:"'ch_sound'"`
}

func (r claimRow) job() job {
	return job{
		logID:   r.LogID,
		attempt: r.Attempts,
		msg: models.Message{
			Id: r.MsgID, UID: r.MsgUID, Type: r.MsgType, Title: r.MsgTitle,
			Summary: r.MsgSummary, Body: r.MsgBody, Extra: r.MsgExtra, Rev: r.MsgRev,
			CollapseId: r.MsgCollapse, Ctime: r.MsgCtime, ChannelId: r.MsgChannel,
		},
		dev: models.Device{
			Id: r.DevID, UUID: r.DevUUID, APNsToken: r.DevToken,
			APNsEnv: r.DevEnv, Utime: r.DevUtime,
		},
		ch: models.Channel{Id: r.ChID, Level: r.ChLevel, Sound: r.ChSound},
	}
}

// 设备存在但没有 APNs token 的记录不取：用户还没授权通知，
// 之后重新上报 token 时这批会自然被捞起来。设备【不存在】的要取，好置终态。
const claimSQL = `
	SELECT pl.id AS log_id, pl.attempts AS attempts,
	       m.id AS msg_id, m.uid AS msg_uid, m.type AS msg_type, m.title AS msg_title,
	       m.summary AS msg_summary, m.body AS msg_body, m.extra AS msg_extra,
	       m.rev AS msg_rev, m.collapse_id AS msg_collapse, m.created_at AS msg_ctime,
	       m.channel_id AS msg_channel,
	       d.id AS dev_id, d.uuid AS dev_uuid, d.apns_token AS dev_token,
	       d.apns_env AS dev_env, d.updated_at AS dev_utime,
	       c.id AS ch_id, c.level AS ch_level, c.sound AS ch_sound
	FROM push_log pl
	LEFT JOIN message m ON m.id = pl.message_id
	LEFT JOIN device  d ON d.id = pl.device_id
	LEFT JOIN channel c ON c.id = m.channel_id
	WHERE pl.status IN (?, ?) AND pl.next_retry_at <= ?
	  AND (d.id IS NULL OR d.apns_token <> '')
	ORDER BY pl.id LIMIT ?`

// discard 把一条无法再投递的记录置终态。
func (p *Pusher) discard(logID int64, reason string) {
	_, _ = p.d.Engine().Exec(
		"UPDATE push_log SET status = ?, reason = ?, updated_at = ? WHERE id = ?",
		models.PushFailed, reason, time.Now().Unix(), logID)
}

func (p *Pusher) deliver(ctx context.Context, j job) {
	n := &apns2.Notification{
		DeviceToken: j.dev.APNsToken,
		ApnsID:      "",
		// ⚠️ 一定要设过期时间。不设（0）表示「投递一次就丢」，
		// 手机关机或飞行模式时这条消息就永久没了。
		Expiration: time.Unix(j.msg.Ctime, 0).Add(p.expiration),
		Priority:   apns2.PriorityHigh,
		PushType:   apns2.PushTypeAlert,
		CollapseID: j.msg.CollapseId,
	}
	if j.ch.Level == models.LevelPassive {
		n.Priority = apns2.PriorityLow
	}

	// 这个环境根本没配密钥：是配置问题，不是瞬时故障。
	// 当成可重试会白试五轮、污染队列，还会把真正的原因埋在一堆退避里。
	if !p.cl.Has(j.dev.APNsEnv) {
		log.Error("设备所在的 APNs 环境没有配置密钥，该设备收不到推送",
			log.Any("device", j.dev.UUID), log.Any("env", j.dev.APNsEnv),
			log.Any("fix", "在 config.toml 的 [apns."+j.dev.APNsEnv+"] 里配上密钥；只用 App Store 版 app 的话不需要 sandbox"))
		p.finish(j, models.PushFailed, 0, "未配置 "+j.dev.APNsEnv+" 环境的 APNs 密钥", "")
		return
	}

	payload, err := kapns.Build(p.buildPayload(j))
	if err != nil {
		p.finish(j, models.PushFailed, 0, "payload 组装失败: "+err.Error(), "")
		return
	}
	n.Payload = payload

	cctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	res, err := p.cl.Push(cctx, j.dev.APNsEnv, n)
	if err != nil {
		p.retry(j, 0, "传输失败: "+err.Error())
		return
	}
	p.record(j, res)
}

func (p *Pusher) buildPayload(j job) kapns.Payload {
	var pl kapns.Payload
	pl.APS.Alert.Title = j.msg.Title
	if pl.APS.Alert.Title == "" {
		pl.APS.Alert.Title = j.msg.Summary
		pl.APS.Alert.Body = ""
	} else {
		pl.APS.Alert.Body = j.msg.Summary
	}
	pl.APS.Sound = j.ch.Sound
	// 同频道的通知在通知中心自动堆叠。
	pl.APS.ThreadID = j.ch.Id
	// 让通知扩展介入：拉全文、下附件、改写标题。
	pl.APS.MutableContent = 1
	pl.APS.InterruptionLevel = j.ch.Level
	// critical 需要 Apple 单独审批的 entitlement。允许配，但没有时静默降级，
	// 不能让整条推送因此失败。
	if pl.APS.InterruptionLevel == models.LevelCritical {
		pl.APS.InterruptionLevel = models.LevelTimeSensitive
	}
	pl.MsgUID, pl.ChannelID, pl.Rev, pl.Type = j.msg.UID, j.ch.Id, j.msg.Rev, j.msg.Type
	// 带图就把【缩略图】的签名链接放进 payload：通知扩展据此下图做成附件，
	// 通知上那块 app 图标就换成图片本身。给原图会撑爆扩展那 24MB 预算。
	if p.files != nil && j.msg.Extra != "" {
		if e, err := decodeExtra(j.msg.Extra); err == nil && e.File != "" {
			pl.Image = p.files.URL(e.File, true)
		}
	}
	// 预算够就内联正文与附加信息：短消息和小卡片因此完全不用联网。
	//
	// 【不按类型卡，按预算】原先只给 text / markdown 内联，卡片一律不带——
	// 可预算这件事 Build 已经管了（放不下就丢 b 与 e 并标 tr=1），
	// 在这里再按类型卡一道，只会让一张几百字节的小卡片白白多一趟网络。
	//
	// 【卡片的内容在 extra 里，body 是空的】所以两样都要带，
	// 而 extraForPush 会把 reply 剥掉——回复的任何东西都不进 payload。
	pl.Body = j.msg.Body
	pl.Extra = extraForPush(j.msg.Extra)
	// 回复的东西【一样都不带】：选项、时限、形态都不进 payload。
	// 通知只负责把人叫进来，回复在 app 内完成。客户端从 /sync 拿这些。
	return pl
}

// record 按 APNs 的回应分类处理。
func (p *Pusher) record(j job, res kapns.Result) {
	if res.OK() {
		if res.SwitchedEnv != "" {
			// 环境纠偏成功：把设备的环境改过来，下次就直接对了。
			log.Warn("设备 APNs 环境不符，已自动纠正",
				log.Any("device", j.dev.UUID), log.Any("to", res.SwitchedEnv))
			_, _ = p.d.Engine().Exec("UPDATE device SET apns_env = ?, updated_at = ? WHERE id = ?",
				res.SwitchedEnv, time.Now().Unix(), j.dev.Id)
		}
		p.finish(j, models.PushOK, res.StatusCode, "", res.APNsID)
		_, _ = p.d.Engine().Exec("UPDATE device SET last_push_at = ?, fail_count = 0 WHERE id = ?",
			time.Now().Unix(), j.dev.Id)
		return
	}

	switch res.Reason {
	case apns2.ReasonUnregistered:
		// ⚠️ 410 的时间戳竞态：Apple 给的 Timestamp 是它认为 token 失效的时刻。
		// 只有当它【晚于】设备最后一次更新时才能注销，否则「重装 app 拿到新 token」
		// 之后延迟到达的旧 410 会把刚配对好的设备误杀。
		if res.Timestamp > 0 && res.Timestamp <= j.dev.Utime {
			log.Warn("忽略过期的 410：设备在 Apple 记录的失效时刻之后重新注册过",
				log.Any("device", j.dev.UUID),
				log.Any("apns_ts", res.Timestamp), log.Any("device_updated", j.dev.Utime))
			p.finish(j, models.PushFailed, res.StatusCode, "Unregistered（已过期，忽略）", res.APNsID)
			return
		}
		_, _ = p.d.Engine().Exec(
			"UPDATE device SET apns_token = '', status = ?, updated_at = ? WHERE id = ?",
			models.DeviceUnregistered, time.Now().Unix(), j.dev.Id)
		p.finish(j, models.PushFailed, res.StatusCode, res.Reason, res.APNsID)

	case apns2.ReasonBadDeviceToken, apns2.ReasonDeviceTokenNotForTopic,
		apns2.ReasonPayloadTooLarge, apns2.ReasonTopicDisallowed:
		// 终态：重试也不会变好。DeviceTokenNotForTopic / PayloadTooLarge 是
		// 服务端自己配错或算错，要能在状态页上看见。
		if res.Reason == apns2.ReasonDeviceTokenNotForTopic || res.Reason == apns2.ReasonPayloadTooLarge {
			log.Error("APNs 配置或组装有问题", log.Any("reason", res.Reason))
		}
		p.finish(j, models.PushFailed, res.StatusCode, res.Reason, res.APNsID)

	case apns2.ReasonExpiredProviderToken, apns2.ReasonInvalidProviderToken,
		apns2.ReasonMissingProviderToken, apns2.ReasonTooManyProviderTokenUpdates:
		// 这是【服务端的密钥问题，不是设备问题】——绝对不能因此删设备。
		log.Error("APNs 鉴权失败，请检查密钥配置", log.Any("reason", res.Reason))
		p.retry(j, res.StatusCode, res.Reason)

	default:
		// 429 / 5xx 以及其它：可重试。
		p.retry(j, res.StatusCode, res.Reason)
	}
}

func (p *Pusher) retry(j job, status int, reason string) {
	attempt := j.attempt + 1
	if attempt >= p.maxAttempts {
		p.finish(j, models.PushAbandon, status, reason+"（已达最大重试次数）", "")
		return
	}
	d := withJitter(backoff[min(attempt-1, len(backoff)-1)])
	now := time.Now()
	_, _ = p.d.Engine().Exec(`UPDATE push_log SET status = ?, attempts = ?, http_status = ?,
		reason = ?, next_retry_at = ?, updated_at = ? WHERE id = ?`,
		models.PushRetry, attempt, status, reason, now.Add(d).Unix(), now.Unix(), j.logID)
}

func (p *Pusher) finish(j job, status, httpStatus int, reason, apnsID string) {
	now := time.Now().Unix()
	_, _ = p.d.Engine().Exec(`UPDATE push_log SET status = ?, attempts = ?, http_status = ?,
		reason = ?, apns_id = ?, updated_at = ? WHERE id = ?`,
		status, j.attempt+1, httpStatus, reason, apnsID, now, j.logID)
	if status != models.PushOK {
		_, _ = p.d.Engine().Exec("UPDATE device SET fail_count = fail_count + 1 WHERE id = ?", j.dev.Id)
	}
}
