package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aichy126/knockbox/internal/models"
)

// ReplySpec 发送方声明「这条消息能回」。
//
// 一个带 type 的对象而不是几个平铺字段：现在只有 choice 和 text，
// 将来要加多选或数值时，加的是一个 type 值，不必再往 SendInput 上挂新字段，
// 也不必让调用方改已经写好的请求。平铺写法（choices=a,b）保留给 curl 当简写，
// 在 HTTP 层归一成这个对象。
type ReplySpec struct {
	Type string `json:"type"`
	// 选项，type=choice 时用。顺序就是界面上的顺序。
	Options []string `json:"options,omitempty"`
	// 回复时限，秒。0 = 不限，这是默认。
	// 只有「不回就会自动发生别的事」的消息才需要它——
	// agent 问一句等回答那种没有默认分支，设了反而会让答案白白作废。
	Timeout int `json:"timeout,omitempty"`
	// 回调地址。必填：没有地址的回复无处可去，不该假装收下。
	Webhook string `json:"webhook"`

	// 下面三个只对 type=number 有意义。
	// 用指针是为了分得出「没写」和「写了 0」—— 0 是完全合法的下限。
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Step *float64 `json:"step,omitempty"`
	// Unit 单位，只用于显示（°C / % / 分钟）。服务端不解释它。
	Unit string `json:"unit,omitempty"`
}

// 选项数量上限。
//
// 回复都在 app 内完成，那里没有硬性的动作数限制，所以这个数卡的是
// 「一屏列表还读得完」，不是任何系统上限。真要问二十个选项的问题，
// 那多半是问法不对。
const (
	replyMaxOptions   = 10
	replyMaxOptionLen = 40
	// 时限上限 30 天。再长就该由发送方自己记着这件事，而不是让一条通知挂在那里。
	replyMaxTimeout = 30 * 24 * 3600
)

// validate 校验并规整。
//
// 报错都写成「哪里不对 + 该怎么改」：这些错误只有发送方看得到，
// 而它多半是一段脚本，人看到的是脚本打出来的这一行。
func (r *ReplySpec) validate() error {
	r.Type = strings.TrimSpace(r.Type)
	if r.Type == "" {
		// 给了选项却没写 type 是最常见的手误，替他补上比报错有用。
		if len(r.Options) > 0 {
			r.Type = models.ReplyChoice
		} else {
			r.Type = models.ReplyText
		}
	}
	switch r.Type {
	case models.ReplyChoice, models.ReplyMulti:
		// 多选和单选的选项规则完全一样，差别只在用户能勾几个。
		if len(r.Options) < 2 {
			return errors.New("reply.options needs at least two entries: a question with one option is not a question")
		}
		if len(r.Options) > replyMaxOptions {
			return fmt.Errorf("reply.options takes at most %d entries", replyMaxOptions)
		}
		seen := make(map[string]bool, len(r.Options))
		for i, o := range r.Options {
			o = strings.TrimSpace(o)
			if o == "" {
				return errors.New("reply.options cannot contain an empty entry")
			}
			if len([]rune(o)) > replyMaxOptionLen {
				return fmt.Errorf("each entry in reply.options is at most %d characters", replyMaxOptionLen)
			}
			// 两个一样的选项在界面上无法区分，而回调只回文本，
			// 发送方也分不出用户点的是哪一个。
			if seen[o] {
				return fmt.Errorf("reply.options contains %q twice", o)
			}
			seen[o] = true
			r.Options[i] = o
		}
	case models.ReplyNumber:
		if len(r.Options) > 0 {
			return errors.New("type=number does not take reply.options; it takes a numeric range")
		}
		if r.Min == nil || r.Max == nil {
			return errors.New("type=number requires both reply.min and reply.max")
		}
		if *r.Min >= *r.Max {
			return fmt.Errorf("reply.min (%g) must be less than reply.max (%g)", *r.Min, *r.Max)
		}
		if r.Step == nil {
			// 不给就按 1：整数是最常见的情形，而 0 或负数会让「落到网格上」这步除以零。
			one := 1.0
			r.Step = &one
		}
		if *r.Step <= 0 {
			return errors.New("reply.step must be greater than 0")
		}
		if *r.Step > *r.Max-*r.Min {
			return fmt.Errorf("reply.step (%g) is wider than the whole range, leaving only one value to pick", *r.Step)
		}
		if len([]rune(r.Unit)) > 8 {
			return errors.New("reply.unit is at most 8 characters; it is the unit shown after the number")
		}
		r.Unit = strings.TrimSpace(r.Unit)
	case models.ReplyText:
		// 文本回复没有选项。带了多半是把 type 写错了，直接说出来。
		if len(r.Options) > 0 {
			return errors.New("type=text does not take reply.options; use type=choice to get buttons")
		}
	default:
		return fmt.Errorf("unsupported reply.type %q: it is one of choice / multi / number / text", r.Type)
	}

	if r.Timeout < 0 {
		return errors.New("reply.timeout cannot be negative")
	}
	if r.Timeout > replyMaxTimeout {
		return fmt.Errorf("reply.timeout is at most %d seconds (30 days)", replyMaxTimeout)
	}

	// 没有回调地址就没有回复的去处。这里必须报错而不是默默收下：
	// 让用户在手机上认真点了一下、而那个回答谁也收不到，是最坏的失败。
	r.Webhook = strings.TrimSpace(r.Webhook)
	if r.Webhook == "" {
		return errors.New("asking for a reply requires reply.webhook: without it the answer has nowhere to go")
	}
	u, err := url.Parse(r.Webhook)
	if err != nil {
		return fmt.Errorf("reply.webhook is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("reply.webhook must be http or https")
	}
	if u.Host == "" {
		return errors.New("reply.webhook has no host")
	}
	return nil
}
