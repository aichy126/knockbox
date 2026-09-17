package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/aichy126/knockbox/internal/uierr"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// Reply 处理用户对一条消息的回复。
type Reply struct{ d *dao.DAO }

func NewReply(d *dao.DAO) *Reply { return &Reply{d: d} }

// 回复文本的长度上限。
// 它要进回调的 JSON、也要在界面上显示，不是正文，不需要很长。
const replyMaxRunes = 1000

// 这些说法会原样显示给用户，所以写成「发生了什么 + 还能做什么」。
// 分类依据是「用户的下一步不同」：过期了只能作罢，别人回过了只需知道结果，
// 选项不对是客户端的 bug（用户自己无能为力，但得知道不是白点了）。
const (
	errReplyNotFound  = "这条消息不在了"
	errReplyNotOpen   = "这条消息不能回复"
	errReplyExpired   = "回复时限已过，这次回复没有送出"
	errReplyDone      = "已经回复过了"
	errReplyBadChoice = "这个选项不在可选范围里，请更新到最新版本再试"
	errReplyBadNumber = "这不是一个有效的数值"
	errReplyEmpty     = "回复内容不能为空"
)

// ReplyResult 回复成功后返回给客户端的。
type ReplyResult struct {
	UID       string `json:"uid"`
	Rev       int64  `json:"rev"`
	Reply     string `json:"reply"`
	RepliedAt int64  `json:"replied_at"`
}

// Submit 记下一条回复，并把回调排进投递队列。
//
// 三条不变量，破坏之后都不报错、只是悄悄不对：
//
//  1. **一条消息只回一次，先到先得。** 判定和写入必须在同一个事务里。
//     只在事务外先 SELECT 再 UPDATE 的话，两边都会看到「还没回」，于是都写进去、
//     排出两条回调，发送方收到两个互相矛盾的答案。
//  2. **回复要 bump rev。** 这是改老行，不 bump 的话别的设备永远同步不到
//     「已回复」这个状态，界面上按钮一直亮着。
//  3. **回调入队与回复写入同一个事务。** 分开的话，进程在两步之间挂掉，
//     用户看到自己回复成功、而发送方永远等不到 —— 这正是这个功能唯一不能出的错。
func (r *Reply) Submit(userID int64, uid, text string) (*ReplyResult, error) {
	text = strings.TrimSpace(text)

	var out ReplyResult
	err := r.d.Tx(func(sess *xorm.Session) error {
		var msg models.Message
		has, err := sess.Where("uid = ? AND user_id = ?", uid, userID).Get(&msg)
		if err != nil {
			return err
		}
		// 归属没查到和消息不存在共用一句：对这个设备来说下一步都一样，
		// 而分开报等于把「这个 uid 存在」告诉一个无权知道的人。
		if !has || msg.DeletedAt != 0 {
			return errors.New(errReplyNotFound)
		}
		if !msg.Replyable() {
			return errors.New(errReplyNotOpen)
		}
		if msg.Replied() {
			// 已经回过时把原来的答案一起带回去：另一台设备回的，
			// 这台的用户需要知道结果，而不只是被拒绝。
			return &ErrAlreadyReplied{Reply: msg.Reply, At: msg.RepliedAt}
		}

		now := time.Now().Unix()
		// 过期判定在服务端，且用的是服务端的钟。
		// 客户端自己判一次是为了不让用户白点（按钮该灰掉），但那只是显示；
		// 真正的判定必须在这里，否则改一下手机时间就能绕过去。
		if msg.ReplyExpired(now) {
			return errors.New(errReplyExpired)
		}

		spec, err := replySpecOf(msg.Extra)
		if err != nil {
			return err
		}
		if err := validateReplyText(spec, &text); err != nil {
			return err
		}

		rev, err := dao.NextRev(sess)
		if err != nil {
			return err
		}
		// 条件里带 replied_at = 0 是【纵深防御】，不是当前生效的那道锁。
		//
		// 真正让两个并发请求串行的是 DSN 里的 _txlock=immediate：写事务一开始就拿写锁，
		// 第二个请求要等第一个提交完才开始，于是它在上面那次 SELECT 就已经看到「已回复」。
		// 实测把这个条件去掉，并发测试照样过 —— 所以它现在是冗余的。
		//
		// 留着是因为它的成本是零，而它挡的是一类换了地基就会立刻出现的错误：
		// 换掉 DSN 的 _txlock、换成别的数据库、或者把这段挪出事务，
		// 都会让上面那次 SELECT 不再可靠，而这一行仍然成立。
		res, err := sess.Exec(
			"UPDATE message SET reply = ?, replied_at = ?, rev = ? WHERE id = ? AND replied_at = 0",
			text, now, rev, msg.Id)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			// 我们读到的是「没回」，写的时候却已经有人回了。重查一次拿那个答案。
			var cur models.Message
			if _, err := sess.ID(msg.Id).Get(&cur); err == nil && cur.Replied() {
				return &ErrAlreadyReplied{Reply: cur.Reply, At: cur.RepliedAt}
			}
			return errors.New(errReplyDone)
		}

		if err := enqueueHook(sess, &msg, text, now); err != nil {
			return err
		}
		out = ReplyResult{UID: msg.UID, Rev: rev, Reply: text, RepliedAt: now}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &out, nil
}

// ErrAlreadyReplied 这条已经被（可能是另一台设备）回过了。
//
// 单独一个类型而不是一句话：客户端要拿到那个答案把界面切成「已回复 · X」，
// 光给一句「已经回复过了」的话，用户只知道自己没点成，不知道结果是什么。
type ErrAlreadyReplied struct {
	Reply string
	At    int64
}

func (e *ErrAlreadyReplied) Error() string { return errReplyDone }

// replySpecOf 从 extra 里取回复规格。
func replySpecOf(raw string) (*replyView, error) {
	e, err := decodeExtra(raw)
	if err != nil {
		return nil, fmt.Errorf("读取消息附加信息失败: %w", err)
	}
	if e.Reply == nil {
		// 有回调地址却没有规格，说明这行数据被写坏了。
		// 当成「不能回」而不是放行：不知道该收什么形态的答案时，收下比拒绝更糟。
		return nil, errors.New(errReplyNotOpen)
	}
	return e.Reply, nil
}

// validateReplyText 按回复形态校验用户交回来的内容，并归一成入库的形式。
//
// 入库的形式按类型不同：
//
//	choice  选中那一项的原文
//	text    用户打的那行字
//	multi   选中项的 JSON 数组，例如 ["api","worker"]
//	number  规范化到 step 网格之后的数字，例如 "24.5"
//
// 校验不能省。任何持有设备 token 的人都能往这个接口送任意字符串，
// 而回调那一端多半拿它直接做分支判断。
func validateReplyText(spec *replyView, text *string) error {
	switch spec.Type {
	case models.ReplyChoice:
		for _, o := range spec.Options {
			if o == *text {
				return nil
			}
		}
		return errors.New(errReplyBadChoice)

	case models.ReplyText:
		if *text == "" {
			return errors.New(errReplyEmpty)
		}
		*text = truncateRunes(*text, replyMaxRunes)
		return nil

	case models.ReplyMulti:
		return validateMulti(spec, text)

	case models.ReplyNumber:
		return validateNumber(spec, text)

	default:
		return errors.New(errReplyNotOpen)
	}
}

// validateMulti 多选。客户端送来一个 JSON 数组。
//
// 允许一个都不选：「这几个都不要」是一个有意义的答案，和「还没回」不同 ——
// 后者看 replied_at，两者分得开。
func validateMulti(spec *replyView, text *string) error {
	var picked []string
	if err := json.Unmarshal([]byte(*text), &picked); err != nil {
		return errors.New(errReplyBadChoice)
	}
	allowed := make(map[string]bool, len(spec.Options))
	for _, o := range spec.Options {
		allowed[o] = true
	}
	seen := make(map[string]bool, len(picked))
	// 按【发送方给的顺序】重排，不按用户点的顺序。
	// 回调那一端拿到的顺序因此是稳定的，它不必自己再排一次。
	out := make([]string, 0, len(picked))
	for _, p := range picked {
		if !allowed[p] {
			return errors.New(errReplyBadChoice)
		}
		seen[p] = true
	}
	for _, o := range spec.Options {
		if seen[o] {
			out = append(out, o)
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	*text = string(b)
	return nil
}

// validateNumber 数值。客户端送来一个数字的字符串形式。
//
// 服务端把它【规范化到 step 网格上】，而不只是校验。
// 发送方声明 step=0.5 时，它期待拿到的是 24.0 或 24.5，不是 24.3178——
// 只校验范围的话，任何精度的值都能送进它的回调。
func validateNumber(spec *replyView, text *string) error {
	v, err := strconv.ParseFloat(strings.TrimSpace(*text), 64)
	if err != nil {
		return errors.New(errReplyBadNumber)
	}
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return errors.New(errReplyBadNumber)
	}
	if spec.Min == nil || spec.Max == nil || spec.Step == nil || *spec.Step <= 0 {
		// 规格不全，说明这行数据写坏了。当成不能回，而不是放行一个没约束的数。
		return errors.New(errReplyNotOpen)
	}
	if v < *spec.Min || v > *spec.Max {
		return uierr.New(uierr.ReplyOutOfRange, *spec.Min, *spec.Max)
	}
	// 落到网格上，再夹回范围内：靠近上界时四舍五入可能越界一格。
	steps := math.Round((v - *spec.Min) / *spec.Step)
	v = math.Min(*spec.Max, *spec.Min+steps**spec.Step)
	// 用 -1 精度：它给出「能精确还原这个 float64 的最短表示」，
	// 所以 24 就是 "24"、24.5 就是 "24.5"，不会出现 24.500000 或 0.30000000000000004。
	*text = strconv.FormatFloat(v, 'f', -1, 64)
	return nil
}

// hookPayload 发给回调地址的内容。
//
// 带上 title 而不只是 uid：接收方多半是一段自动化脚本，它不一定还记得这条消息是什么，
// 也不该为了认出自己的问题再回头查一次。
type hookPayload struct {
	UID     string `json:"uid"`
	Channel string `json:"channel"`
	Title   string `json:"title"`
	// Reply 的 JSON 类型随回复形态变：
	//
	//	choice / text  字符串    "不要打开"
	//	multi          字符串数组 ["api","worker"]
	//	number         数字      24.5
	//
	// 让它随类型变而不是一律给字符串：接收方多半直接拿它做判断，
	// 数值回成 "24.5" 的话每一端都要自己再 parse 一次，而那正是出错的地方。
	// choice 和 text 仍是字符串，所以已经接上的那些不受影响。
	Reply     any   `json:"reply"`
	RepliedAt int64 `json:"replied_at"`
}

// hookReplyValue 把入库的回复还原成回调里该有的 JSON 类型。
//
// 解析失败时退回原始字符串：回调发出去比发不出去重要得多，
// 而接收方至少还能看到用户回了什么。
func hookReplyValue(kind, raw string) any {
	switch kind {
	case models.ReplyMulti:
		var picked []string
		if err := json.Unmarshal([]byte(raw), &picked); err != nil {
			return raw
		}
		return picked
	case models.ReplyNumber:
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return raw
		}
		return v
	default:
		return raw
	}
}

// enqueueHook 把回调排进队列。必须与回复写入同一事务，理由见 Submit 的注释 3。
func enqueueHook(sess *xorm.Session, msg *models.Message, text string, now int64) error {
	kind := ""
	if e, err := decodeExtra(msg.Extra); err == nil && e.Reply != nil {
		kind = e.Reply.Type
	}
	body, err := json.Marshal(hookPayload{
		UID: msg.UID, Channel: msg.ChannelId, Title: msg.Title,
		Reply: hookReplyValue(kind, text), RepliedAt: now,
	})
	if err != nil {
		return err
	}
	// 签名密钥用频道 token：双方本来就都持有它，不必再发一把新的。
	// 这里取一次存进队列行，因为 token 可以被轮换，而重试可能发生在轮换之后。
	var ch models.Channel
	if _, err := sess.Where("id = ?", msg.ChannelId).Get(&ch); err != nil {
		return err
	}
	_, err = sess.Insert(&models.ReplyHook{
		MessageId: msg.Id, URL: msg.ReplyWebhook, Payload: string(body),
		Secret: ch.Token, Status: models.HookPending, NextAt: now,
		Ctime: now, Utime: now,
	})
	return err
}
