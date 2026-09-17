package service

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/aichy126/knockbox/internal/dao"
	"github.com/aichy126/knockbox/internal/library/idgen"
	"github.com/aichy126/knockbox/internal/models"
	"xorm.io/xorm"
)

// SendInput 业务端发来的一条消息。
//
// 没有 sound / level / priority：通知怎么响是【频道】的属性，在 app 里设置。
// 发送方只管消息内容——推送服务不懂业务语义，本来就无从判断什么算急。
// 要区分紧急度就建两个频道，往不同 token 发。
type SendInput struct {
	Type       string     `json:"type"`
	Title      string     `json:"title"`
	Body       string     `json:"body"`
	Summary    string     `json:"summary"`
	Link       string     `json:"link"`
	Copy       string     `json:"copy"`
	Items      []CardItem `json:"items"`
	Actions    []Action   `json:"actions"`
	File       string     `json:"file"`
	CollapseID string     `json:"collapse_id"`
	IdemKey    string     `json:"idem_key"`
	// Reply 声明这条消息可以回，并给出回复的形态和回调地址。
	// nil = 单向消息，这是绝大多数。
	Reply *ReplySpec `json:"reply"`
	// ReplyParseError HTTP 层解析 reply 时出的错。
	//
	// 不在那一层直接返回，是为了让「怎么报这个错」只有一处 ——
	// Deliver 是所有发送路径的必经之地，而 HTTP 层有四种 Content-Type 分支。
	// 不进 JSON：它是层间传递的东西，不是调用方能设的字段。
	ReplyParseError error `json:"-"`
}

// CardItem 卡片里的一行。
//
// 用有序数组而不是 map，因为 map 的遍历顺序随机，卡片的行序会每次都变；
// 值用字符串而不是数字，因为「分支 = main」这类内容本来就不是数值。
type CardItem struct {
	K     string `json:"k"`
	V     string `json:"v"`
	Style string `json:"style"` // ok|warn|error|muted
}

type Action struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

type SendResult struct {
	UID     string `json:"uid"`
	Rev     int64  `json:"rev"`
	Devices int    `json:"devices"`
	Queued  bool   `json:"queued"`
	Muted   bool   `json:"muted,omitempty"`
	Dedup   bool   `json:"dedup,omitempty"` // 命中幂等键，复用了已有的那条
	// ReplyUntil 这条消息的回复时限（unix 秒）。发送方据此知道等到什么时候，
	// 不必自己拿本地时钟去加 —— 两边的钟未必一致，而判定在服务端。
	ReplyUntil int64 `json:"reply_until,omitempty"`
}

type Send struct {
	d   *dao.DAO
	dev *Device
}

func NewSend(d *dao.DAO) *Send { return &Send{d: d, dev: NewDevice(d)} }

// apnsSummaryRunes 摘要长度上限。
// 它会进 APNs 的 alert.body，而 NSE 被系统跳过时（内存压力、低电量时真的会发生）
// 用户能看到的就只有它，所以必须是一句能独立读懂的话。
const apnsSummaryRunes = 300

// Summarize 从正文生成推送摘要。
//
// 这段文字会进 APNs 的 alert.body，而通知扩展被系统跳过时（内存压力、低电量时真的会发生）
// 用户在锁屏上看到的就只有它。所以它必须是一句【人能读懂的话】，
// 不能出现 ``` 、|---|---| 这类源码记号。
//
// 不做完整的 markdown 解析——这是通知条不是渲染器，只处理最常见的几种块。
func Summarize(body string, limit int) string {
	var b strings.Builder
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)

		// 代码块整块跳过：一屏通知放不下代码，放进去只会把真正的结论挤掉
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || t == "" {
			continue
		}
		// 表格：分隔行直接丢，数据行把竖线换成间隔号
		if isTableSeparator(t) {
			continue
		}
		if strings.HasPrefix(t, "|") {
			cells := []string{}
			for _, c := range strings.Split(strings.Trim(t, "|"), "|") {
				if c = strings.TrimSpace(c); c != "" {
					cells = append(cells, c)
				}
			}
			t = strings.Join(cells, " ")
		}
		// 水平分割线
		if t == "---" || t == "***" || t == "___" {
			continue
		}
		// 标题 / 列表 / 引用的前缀记号
		t = strings.TrimLeft(t, "#>-*+ \t")
		// 行内记号
		t = strings.ReplaceAll(t, "`", "")
		t = strings.ReplaceAll(t, "**", "")
		t = strings.ReplaceAll(t, "__", "")
		// 链接只留文字：[看日志](https://…) → 看日志
		t = linkText.ReplaceAllString(t, "$1")
		if t = strings.TrimSpace(t); t == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString(" · ")
		}
		b.WriteString(t)
		if len([]rune(b.String())) >= limit {
			break
		}
	}
	return truncateRunes(strings.TrimSpace(b.String()), limit)
}

var linkText = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)

// isTableSeparator 认 |---|:--:|---| 这类分隔行。
func isTableSeparator(t string) bool {
	if !strings.HasPrefix(t, "|") {
		return false
	}
	for _, r := range t {
		if r != '|' && r != '-' && r != ':' && r != ' ' {
			return false
		}
	}
	return strings.Contains(t, "-")
}

func truncateRunes(s string, limit int) string {
	r := []rune(s)
	if len(r) <= limit {
		return s
	}
	return strings.TrimRightFunc(string(r[:limit-1]), unicode.IsSpace) + "…"
}

func validType(t string) bool {
	switch t {
	case models.TypeText, models.TypeMarkdown, models.TypeImage,
		models.TypeFile, models.TypeLink, models.TypeCard:
		return true
	}
	return false
}

// Deliver 落库一条消息并排上推送队列。
//
// 顺序是刻意的：先落库、再排队。反过来的话，推送发出去了而消息没存下，
// app 点开通知就什么都拉不到。APNs 本身是 best-effort，Apple 明确不保证送达，
// 所以「消息一定在库里 + 客户端进前台必同步」才是唯一的可靠性保证。
func (s *Send) Deliver(ch *models.Channel, in SendInput, fromIP string) (*SendResult, error) {
	if in.Type == "" {
		in.Type = models.TypeText
	}
	if !validType(in.Type) {
		return nil, fmt.Errorf("unsupported message type %q", in.Type)
	}
	if strings.TrimSpace(in.Title) == "" && strings.TrimSpace(in.Body) == "" && len(in.Items) == 0 {
		return nil, errors.New("title and body cannot both be empty")
	}

	summary := in.Summary
	if summary == "" {
		summary = Summarize(in.Body, apnsSummaryRunes)
	}
	if summary == "" {
		summary = truncateRunes(in.Title, apnsSummaryRunes)
	}
	// 调用方明确写了 reply 却写坏了，必须报错。
	// 当成「没写」放行的话，那条消息静默变成单向的，而发送方还在等答案。
	if in.ReplyParseError != nil {
		return nil, in.ReplyParseError
	}
	if in.Reply != nil {
		if err := in.Reply.validate(); err != nil {
			return nil, err
		}
	}
	extra, err := encodeExtra(in)
	if err != nil {
		return nil, err
	}

	targets, err := s.dev.PushTargets(ch.UserId)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	msg := &models.Message{
		UID: idgen.ULID(), UserId: ch.UserId, ChannelId: ch.Id, Type: in.Type,
		Title: truncateRunes(in.Title, 200), Summary: summary, Body: in.Body, Extra: extra,
		CollapseId: truncateRunes(in.CollapseID, 64), IdemKey: in.IdemKey,
		FromName: ch.Id, FromIP: fromIP, Ctime: now,
	}
	if in.Reply != nil {
		msg.ReplyWebhook = in.Reply.Webhook
		if in.Reply.Timeout > 0 {
			// 时限从【落库】起算，不从推送送达起算。APNs 是 best-effort，
			// 送达时刻没有上界，用它起算的话「30 秒内有效」可以在手机上变成任意长。
			// 发送方设时限是因为它那边有个自动动作要等，那个等待从它调接口那刻就开始了。
			msg.ReplyUntil = now + int64(in.Reply.Timeout)
		}
	}

	// message.file_id 必须在这里填上。
	//
	// 删除、清空、保留策略三条路都靠它找到该给哪个附件减引用
	// （sync.releaseFile 与 purge 都是 `WHERE file_id ...`）。
	// 这一列若不写，三条路会全部静默空转：引用减不下去、blob 不会被回收，
	// 而「记录已删除」和「文件仍在磁盘上」从外部看不出区别。
	if in.File != "" {
		fileID, err := s.resolveFile(in.File, ch.UserId)
		if err != nil {
			return nil, err
		}
		msg.FileId = fileID
	}

	var dedup bool
	err = s.d.Tx(func(sess *xorm.Session) error {
		// 幂等：上游重试不该造出第二条消息，更不该推第二遍。
		if in.IdemKey != "" {
			var old models.Message
			has, err := sess.Where("channel_id = ? AND idem_key = ?", ch.Id, in.IdemKey).Get(&old)
			if err != nil {
				return err
			}
			if has {
				*msg, dedup = old, true
				return nil
			}
		}
		rev, err := dao.NextRev(sess)
		if err != nil {
			return err
		}
		msg.Rev = rev
		if _, err := sess.Insert(msg); err != nil {
			return err
		}
		// 引用计数必须在确定插入之后加，并且与插入处于同一事务。
		// 放在事务外面的话，幂等命中那条路不会新建消息，引用却已经加上，
		// 该附件从此再也减不回零。
		if msg.FileId != 0 {
			if _, err := sess.Exec(
				"UPDATE file SET ref_count = ref_count + 1, ever_referenced = 1 WHERE id = ?",
				msg.FileId); err != nil {
				return err
			}
		}
		if _, err := sess.Exec(`UPDATE channel SET msg_count = msg_count + 1, last_msg_id = ?,
			last_msg_at = ?, last_used_at = ?, last_used_ip = ?, updated_at = ? WHERE id = ?`,
			msg.Id, now, now, fromIP, now, ch.Id); err != nil {
			return err
		}
		// 静音的频道照常落库、照常进历史，只是不排推送。
		// 这必须在服务端做：通知扩展拦不住一条通知的展示。
		if ch.Silent(now) {
			return nil
		}
		for _, dev := range targets {
			if _, err := sess.Insert(&models.PushLog{
				MessageId: msg.Id, DeviceId: dev.Id,
				Status: models.PushPending, Ctime: now, Utime: now,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	res := &SendResult{UID: msg.UID, Rev: msg.Rev, Dedup: dedup, ReplyUntil: msg.ReplyUntil}
	if ch.Silent(now) {
		res.Muted = true
		return res, nil
	}
	res.Devices, res.Queued = len(targets), !dedup && len(targets) > 0
	return res, nil
}

// errFileNotUsable 引用的附件取不到时给调用方的说法。
//
// 「这个 uid 不存在」和「它是别人的」共用同一句：对持有 token 的人来说，
// 两种情况的下一步都是重新上传；分开报等于把「这个 uid 存在」告诉一个
// 本来无权知道的人。
const errFileNotUsable = "the attachment is gone or expired; upload it again before sending"

// resolveFile 把附件 uid 换成 file 表的行 id，并确认它属于本频道所属的账户。
//
// 归属这一句不能省。频道 token 只能写自己的频道，但 file 表是全站共用的，
// 只按 uid 查的话，拿到别人的 uid 就能把别人的附件挂到自己的消息上，
// 在多人共用的实例上这是跨账户的。uid 是 ULID、实际猜不到，
// 所以这里是纵深防御——但边界应该由这一句判断来划，而不是由「猜不到」来划。
//
// 取不到就报错，不能像先前那样跳过去照发：静默丢附件的话调用方收到成功，
// 而消息里那张图没了，从外面看不出区别。
func (s *Send) resolveFile(uid string, userID int64) (int64, error) {
	var f models.File
	has, err := s.d.Engine().Where("uid = ?", uid).Get(&f)
	if err != nil {
		return 0, fmt.Errorf("cannot look up the attachment: %w", err)
	}
	if !has || f.UserId != userID {
		return 0, errors.New(errFileNotUsable)
	}
	return f.Id, nil
}

// BatchResult 批量投递里单个频道的结果。
type BatchResult struct {
	Token   string `json:"token"`
	OK      bool   `json:"ok"`
	UID     string `json:"uid,omitempty"`
	Rev     int64  `json:"rev,omitempty"`
	Devices int    `json:"devices,omitempty"`
	Muted   bool   `json:"muted,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Batch 一次把同一条消息投到 N 个频道。
//
// 每个 token 各自独立校验：不引入「广播 token」那种一把钥匙能写所有频道的越权面，
// 而上游本来就持有这 N 个 token。部分失败逐条报告，不能让一个坏 token 把整批打回。
//
// allow 由 HTTP 层传入，用来逐个 token 做速率限制（限流器是进程级的状态，
// 不属于这一层）。传 nil 表示不限。
func (s *Send) Batch(tokens []string, in SendInput, fromIP string, allow func(string) error) []BatchResult {
	out := make([]BatchResult, 0, len(tokens))
	for _, tok := range tokens {
		r := BatchResult{Token: tok}
		var ch models.Channel
		has, err := s.d.Engine().Where("token = ?", tok).Get(&ch)
		switch {
		case err != nil:
			r.Error = "cannot look up the channel: " + err.Error()
		case !has || ch.Status != models.StatusActive:
			r.Error = "invalid channel token"
		default:
			// 速率判定放在 token 校验【之后】：无效 token 不该消耗额度，
			// 否则拿一堆乱码 token 就能把别人的桶刷空。
			if allow != nil {
				if err := allow(tok); err != nil {
					r.Error = err.Error()
					out = append(out, r)
					continue
				}
			}
			res, err := s.Deliver(&ch, in, fromIP)
			if err != nil {
				r.Error = err.Error()
			} else {
				r.OK, r.UID, r.Rev, r.Devices, r.Muted = true, res.UID, res.Rev, res.Devices, res.Muted
			}
		}
		out = append(out, r)
	}
	return out
}
