package service

import (
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

type Sync struct{ d *dao.DAO }

func NewSync(d *dao.DAO) *Sync { return &Sync{d: d} }

// MessageView 同步流里的一条。
// 删除过的只剩 UID / Rev / Deleted —— 内容在删除那一刻就被抹掉了。
type MessageView struct {
	UID       string `json:"uid"`
	Rev       int64  `json:"rev"`
	ID        int64  `json:"id,omitempty"`
	ChannelID string `json:"channel,omitempty"`
	Type      string `json:"type,omitempty"`
	Title     string `json:"title,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Body      string `json:"body,omitempty"`
	Extra     string `json:"extra,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	Read      bool   `json:"read,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
	// 回复相关。Replyable 单独给一个布尔而不是让客户端去看 extra.reply 有没有：
	// 「能不能回」的判据在服务端是 reply_webhook 非空，而那一列【不下发】，
	// 客户端自己推不出来。
	Replyable bool `json:"replyable,omitempty"`
	// 回复时限（unix 秒），0 / 缺省 = 不限。客户端拿它算倒计时和「已过时限」。
	ReplyUntil int64 `json:"reply_until,omitempty"`
	// 已回复的答案与时刻。回复会 bump rev，所以别的设备靠同步就能拿到这两个值。
	Reply     string `json:"reply,omitempty"`
	RepliedAt int64  `json:"replied_at,omitempty"`
}

// PurgeView 频道清空事件。
// 硬删之后那些行不存在了，别的设备学不到「这些没了」，
// 所以把删除事件从「每条一个墓碑」上升为频道级一条记录——一条顶掉 N 个墓碑。
type PurgeView struct {
	ChannelID   string `json:"channel"`
	BeforeMsgID int64  `json:"before_msg_id"`
	Rev         int64  `json:"rev"`
}

type SyncResult struct {
	Rev        int64         `json:"rev"`
	Reset      bool          `json:"reset"`
	HasMore    bool          `json:"has_more"`
	Badge      int           `json:"badge"`
	ReadCursor int64         `json:"read_cursor"`
	Channels   []channelMeta `json:"channels"`
	Messages   []MessageView `json:"messages"`
	Purges     []PurgeView   `json:"purges"`
}

// channelMeta 同步流里的频道。形状要与 /api/v1/channels 的 channelView 对得上——
// 两个接口喂的是客户端同一个模型，少一个字段就是解析失败。token 也在这里给：
// 同步和列表是同一把 DeviceAuth，不给反而逼客户端为了拿 token 再跑一趟。
type channelMeta struct {
	ID        string `json:"id"`
	Token     string `json:"token"`
	Muted     bool   `json:"muted"`
	MuteUntil int64  `json:"mute_until"`
	Sound     string `json:"sound"`
	Level     string `json:"level"`
	Meta      string `json:"meta"`
}

// Since 增量同步。
//
// 游标是 rev 而不是 id：id 只对新增单调，而标记已读 / 删除会改老行，
// 只靠 id > cursor 拉不到那些变更。
func (s *Sync) Since(userID, since int64, limit int) (*SyncResult, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	out := &SyncResult{Messages: []MessageView{}, Purges: []PurgeView{}, Channels: []channelMeta{}}

	cur, err := s.d.CurrentRev()
	if err != nil {
		return nil, err
	}
	out.Rev = cur

	// GC 硬删过的那段 rev 已经不存在了。客户端的 since 比水位线还旧，
	// 说明它缺的那块永远补不回来，必须全量重载。
	// 没有这个标志，一台离线很久的设备会永久缺一块数据且毫无察觉。
	wm, has, err := s.d.KVGet(models.KVGCWatermark)
	if err != nil {
		return nil, err
	}
	if has {
		var mark int64
		_, _ = fmtSscan(wm, &mark)
		if since > 0 && since < mark {
			out.Reset = true
			since = 0
		}
	}

	var u models.User
	if _, err := s.d.Engine().ID(userID).Get(&u); err != nil {
		return nil, err
	}
	out.ReadCursor = u.ReadCursor

	var msgs []models.Message
	if err := s.d.Engine().Where("user_id = ? AND rev > ?", userID, since).
		OrderBy("rev").Limit(limit).Find(&msgs); err != nil {
		return nil, err
	}
	for _, m := range msgs {
		v := MessageView{UID: m.UID, Rev: m.Rev}
		if m.DeletedAt != 0 {
			v.Deleted = true
		} else {
			v.ID, v.ChannelID, v.Type = m.Id, m.ChannelId, m.Type
			v.Title, v.Summary, v.Body, v.Extra = m.Title, m.Summary, m.Body, m.Extra
			v.CreatedAt = m.Ctime
			v.Read = m.ReadAt != 0 || m.Id <= u.ReadCursor
			v.Replyable, v.ReplyUntil = m.Replyable(), m.ReplyUntil
			v.Reply, v.RepliedAt = m.Reply, m.RepliedAt
		}
		out.Messages = append(out.Messages, v)
	}
	out.HasMore = len(msgs) == limit
	if out.HasMore {
		out.Rev = msgs[len(msgs)-1].Rev
	}

	var purges []models.PurgeLog
	if err := s.d.Engine().Where("user_id = ? AND rev > ? AND rev <= ?", userID, since, out.Rev).
		OrderBy("rev").Find(&purges); err != nil {
		return nil, err
	}
	for _, p := range purges {
		out.Purges = append(out.Purges, PurgeView{ChannelID: p.ChannelId, BeforeMsgID: p.BeforeMsgId, Rev: p.Rev})
	}

	chs, err := NewChannel(s.d).List(userID)
	if err != nil {
		return nil, err
	}
	for _, c := range chs {
		out.Channels = append(out.Channels, channelMeta{
			ID: c.Id, Token: c.Token, Muted: c.Muted != 0, MuteUntil: c.MuteUntil, Sound: c.Sound,
			Level: c.Level, Meta: c.Meta})
	}

	out.Badge, err = s.Badge(userID)
	return out, err
}

// Badge 未读数。静音的频道不计入。
func (s *Sync) Badge(userID int64) (int, error) {
	rows, err := s.d.Engine().QueryString(`
		SELECT COUNT(*) AS n FROM message m
		JOIN channel c ON c.id = m.channel_id
		WHERE m.user_id = ? AND m.deleted_at = 0 AND m.read_at = 0
		  AND m.id > (SELECT read_cursor FROM user WHERE id = ?)
		  AND c.muted = 0`, userID, userID)
	if err != nil || len(rows) == 0 {
		return 0, err
	}
	var n int
	for _, v := range rows[0] {
		_, _ = fmtSscanInt(v, &n)
	}
	return n, nil
}

// MarkRead 标记已读。
//
// upTo 走 user.read_cursor 水位线：这让「全部已读」是一次 UPDATE，
// 而不是把几千行 message 的 rev 全部推高、把同步流量炸掉。
// 越过水位线单独读某条才写 message.read_at。
func (s *Sync) MarkRead(userID int64, upToID int64, uids []string) error {
	return s.d.Tx(func(sess *xorm.Session) error {
		now := time.Now().Unix()
		if upToID > 0 {
			_, err := sess.Exec(
				"UPDATE user SET read_cursor = MAX(read_cursor, ?), updated_at = ? WHERE id = ?",
				upToID, now, userID)
			return err
		}
		for _, uid := range uids {
			rev, err := dao.NextRev(sess)
			if err != nil {
				return err
			}
			if _, err := sess.Exec(
				"UPDATE message SET read_at = ?, rev = ? WHERE uid = ? AND user_id = ? AND read_at = 0",
				now, rev, uid, userID); err != nil {
				return err
			}
		}
		return nil
	})
}

// Delete 删除单条。
//
// 内容【当场物理抹掉】，只留一行不含任何内容的空壳墓碑供同步；
// 后台 GC 在所有活跃设备都同步过这个 rev 之后再把空壳也删掉。
// 这样同步是对的，而内容从按下删除那一刻起就已经不在库里了。
func (s *Sync) Delete(userID int64, uids []string) (int, error) {
	n := 0
	err := s.d.Tx(func(sess *xorm.Session) error {
		now := time.Now().Unix()
		for _, uid := range uids {
			var m models.Message
			has, err := sess.Where("uid = ? AND user_id = ? AND deleted_at = 0", uid, userID).Get(&m)
			if err != nil {
				return err
			}
			if !has {
				continue
			}
			rev, err := dao.NextRev(sess)
			if err != nil {
				return err
			}
			if _, err := sess.Exec(`UPDATE message SET title = '', summary = '', body = '', extra = '',
				deleted_at = ?, rev = ? WHERE id = ?`, now, rev, m.Id); err != nil {
				return err
			}
			if err := releaseFile(sess, m.FileId); err != nil {
				return err
			}
			if _, err := sess.Exec(
				"UPDATE channel SET msg_count = MAX(msg_count - 1, 0), updated_at = ? WHERE id = ?",
				now, m.ChannelId); err != nil {
				return err
			}
			n++
		}
		return nil
	})
	return n, err
}

// releaseFile 附件引用减一。归零才由 GC 删 blob——
// sha256 内容寻址意味着同一张图可能被别的频道引用，不能看到就删。
func releaseFile(sess *xorm.Session, fileID int64) error {
	if fileID == 0 {
		return nil
	}
	_, err := sess.Exec("UPDATE file SET ref_count = MAX(ref_count - 1, 0) WHERE id = ?", fileID)
	return err
}
