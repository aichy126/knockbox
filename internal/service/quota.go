package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
)

// Quota 公共实例的每用户配额。
//
// **自建模式下完全不生效**（Limits.Enabled = false）：自己的服务器自己的数据，
// 不该被限。只有对外提供服务的那个实例才需要它——那里的存储和带宽是运营者掏钱，
// 而且一定会有人拿它刷。
//
// 设计上刻意选了「**按天滚动 + 拒绝而不是丢弃**」：
//   - 滚动窗口而不是自然日重置：否则每天零点会有一波集中补发
//   - 超额时【明确返回错误】，不静默丢消息。一个通知系统最不能干的事
//     就是让上游以为发出去了而其实没有
type Quota struct {
	d      *dao.DAO
	limits Limits
}

type Limits struct {
	Enabled     bool
	MaxChannels int
	MaxPerDay   int
	MaxFileMB   int
	// RetentionDays 超过这个天数的消息会被 GC 清掉；0 = 永久保留（自建的默认值）
	RetentionDays int
}

// QuotaError 配额类错误单独成型：调用方要能区分「你发错了」和「你发太多了」，
// 前者该改代码，后者只要等。
type QuotaError struct {
	Kind  string
	Limit int
	Reset time.Duration
}

func (e *QuotaError) Error() string {
	switch e.Kind {
	case "channels":
		return fmt.Sprintf("频道数已达上限（%d 个）", e.Limit)
	case "daily":
		// 窗口是滚动的 24 小时，不是自然日，所以不能说「今天」；
		// 恢复时刻由窗口内最早那条消息决定，算不出来时就不承诺时间。
		if e.Reset > 0 {
			return fmt.Sprintf("最近 24 小时的消息数已达上限（%d 条），%s 后恢复",
				e.Limit, e.Reset.Round(time.Minute))
		}
		return fmt.Sprintf("最近 24 小时的消息数已达上限（%d 条）", e.Limit)
	}
	return "已达配额上限"
}

var ErrQuota = errors.New("quota")

func NewQuota(d *dao.DAO, l Limits) *Quota { return &Quota{d: d, limits: l} }

// CheckSend 发一条之前问一次。
func (q *Quota) CheckSend(userID int64) error {
	if !q.limits.Enabled || q.limits.MaxPerDay <= 0 {
		return nil
	}
	exempt, err := q.exempt(userID)
	if err != nil || exempt {
		return nil
	}
	now := time.Now()
	since := now.Add(-24 * time.Hour).Unix()
	var n int64
	if _, err := q.d.Engine().SQL(
		"SELECT COUNT(*) FROM message WHERE user_id=? AND created_at>=?", userID, since).Get(&n); err != nil {
		return nil
	}
	if n < int64(q.limits.MaxPerDay) {
		return nil
	}
	// 滚动窗口的恢复时刻 = 窗口内最早那条消息滑出窗口的时刻。
	var oldest int64
	_, _ = q.d.Engine().SQL(
		"SELECT COALESCE(MIN(created_at), 0) FROM message WHERE user_id=? AND created_at>=?",
		userID, since).Get(&oldest)
	var reset time.Duration
	if oldest > 0 {
		reset = time.Until(time.Unix(oldest, 0).Add(24 * time.Hour))
	}
	return &QuotaError{Kind: "daily", Limit: q.limits.MaxPerDay, Reset: reset}
}

// CheckChannel 建频道之前问一次。
func (q *Quota) CheckChannel(userID int64) error {
	if !q.limits.Enabled || q.limits.MaxChannels <= 0 {
		return nil
	}
	exempt, err := q.exempt(userID)
	if err != nil || exempt {
		return nil
	}
	var n int64
	if _, err := q.d.Engine().SQL(
		"SELECT COUNT(*) FROM channel WHERE user_id=?", userID).Get(&n); err != nil {
		return nil
	}
	if n >= int64(q.limits.MaxChannels) {
		return &QuotaError{Kind: "channels", Limit: q.limits.MaxChannels}
	}
	return nil
}

// exempt 这个用户是不是被豁免了。
//
// 错误原样返回，由调用方决定怎么办。全局的取向是【出错一律放行】：
// 配额是运营手段，不该因为它自己查不出来就把消息挡住。
// 两个 Check* 都照这个方向处理，不要一个放一个收。
func (q *Quota) exempt(userID int64) (bool, error) {
	var n int64
	if _, err := q.d.Engine().SQL(
		"SELECT unlimited FROM user WHERE id=?", userID).Get(&n); err != nil {
		return false, err
	}
	return n != 0, nil
}

// Usage 给「用量」页面和 /api/v1/server 用。
type Usage struct {
	Channels     int   `json:"channels"`
	ChannelLimit int   `json:"channel_limit"`
	Today        int   `json:"today"`
	DailyLimit   int   `json:"daily_limit"`
	Messages     int64 `json:"messages"`
	FileBytes    int64 `json:"file_bytes"`
	RetainDays   int   `json:"retention_days"`
	Unlimited    bool  `json:"unlimited"`
}

func (q *Quota) Usage(userID int64) Usage {
	u := Usage{
		ChannelLimit: q.limits.MaxChannels, DailyLimit: q.limits.MaxPerDay,
		RetainDays: q.limits.RetentionDays,
	}
	u.Unlimited, _ = q.exempt(userID)
	if !q.limits.Enabled || u.Unlimited {
		u.ChannelLimit, u.DailyLimit, u.RetainDays = 0, 0, 0 // 0 = 不限
	}
	e := q.d.Engine()
	var n int64
	_, _ = e.SQL("SELECT COUNT(*) FROM channel WHERE user_id=?", userID).Get(&n)
	u.Channels = int(n)
	_, _ = e.SQL("SELECT COUNT(*) FROM message WHERE user_id=? AND created_at>=?",
		userID, time.Now().Add(-24*time.Hour).Unix()).Get(&n)
	u.Today = int(n)
	_, _ = e.SQL("SELECT COUNT(*) FROM message WHERE user_id=? AND deleted_at=0", userID).Get(&n)
	u.Messages = n
	_, _ = e.SQL("SELECT COALESCE(SUM(size),0) FROM file WHERE user_id=?", userID).Get(&n)
	u.FileBytes = n
	return u
}
