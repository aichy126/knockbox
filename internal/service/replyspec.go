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
	// Placeholder 输入框里的提示，只对 type=text 有意义。
	Placeholder string `json:"placeholder,omitempty"`
	// Required 能不能空着交。不写 = 必须作答，和这个字段出现之前的行为一致。
	// 用指针分得出「没写」和「写了 false」。
	Required *bool `json:"required,omitempty"`

	// Fields 组合回复：一条消息挂几个控件，一次提交。每个控件就是上面四种之一。
	// 给了它，顶层就只剩 timeout 和 webhook 有意义，type 记成 form。
	Fields []ReplyField `json:"fields,omitempty"`
}

// ReplyField 组合回复里的一个控件。
//
// 字段和单控件写法一一对应，多一个 ID：回调里按它把答案对回去。
type ReplyField struct {
	ID string `json:"id"`
	// Label 控件上方那一行字，告诉人这一项问的是什么。只有一个控件时可以省；
	// 几个控件放在一起时没有它，两个滑块就分不清哪个是哪个。
	Label       string   `json:"label,omitempty"`
	Type        string   `json:"type"`
	Options     []string `json:"options,omitempty"`
	Min         *float64 `json:"min,omitempty"`
	Max         *float64 `json:"max,omitempty"`
	Step        *float64 `json:"step,omitempty"`
	Unit        string   `json:"unit,omitempty"`
	Placeholder string   `json:"placeholder,omitempty"`
	Required    *bool    `json:"required,omitempty"`
}

// optional 这个控件允许空着交。
func (f *ReplyField) optional() bool { return f.Required != nil && !*f.Required }

// 选项数量上限。
//
// 回复都在 app 内完成，那里没有硬性的动作数限制，所以这个数卡的是
// 「一屏列表还读得完」，不是任何系统上限。真要问二十个选项的问题，
// 那多半是问法不对。
const (
	replyMaxOptions   = 10
	replyMaxOptionLen = 40
	// 组合回复最多几个控件。回复在 app 里一整屏完成、能滚动，所以卡的是「一屏表单还填得完」；
	// 与选项上限取同一个数，好记。
	replyMaxFields      = 10
	replyMaxFieldIDLen  = 32
	replyMaxPlaceholder = 60
	replyMaxLabel       = 60
	// 时限上限 30 天。再长就该由发送方自己记着这件事，而不是让一条通知挂在那里。
	replyMaxTimeout = 30 * 24 * 3600
)

// validate 校验并规整。
//
// 报错都写成「哪里不对 + 该怎么改」：这些错误只有发送方看得到，
// 而它多半是一段脚本，人看到的是脚本打出来的这一行。
func (r *ReplySpec) validate() error {
	if len(r.Fields) > 0 {
		if err := r.validateFields(); err != nil {
			return err
		}
	} else {
		// 单控件写法：借 ReplyField 的校验，校完把规整过的值写回顶层。
		f := ReplyField{Type: r.Type, Options: r.Options, Min: r.Min, Max: r.Max, Step: r.Step,
			Unit: r.Unit, Placeholder: r.Placeholder, Required: r.Required}
		if err := f.validate("reply"); err != nil {
			return err
		}
		r.Type, r.Options, r.Step, r.Unit, r.Placeholder = f.Type, f.Options, f.Step, f.Unit, f.Placeholder
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

func (r *ReplyField) validate(at string) error {
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
			return errors.New(at + ".options needs at least two entries: a question with one option is not a question")
		}
		if len(r.Options) > replyMaxOptions {
			return fmt.Errorf("%s.options takes at most %d entries", at, replyMaxOptions)
		}
		seen := make(map[string]bool, len(r.Options))
		for i, o := range r.Options {
			o = strings.TrimSpace(o)
			if o == "" {
				return errors.New(at + ".options cannot contain an empty entry")
			}
			if len([]rune(o)) > replyMaxOptionLen {
				return fmt.Errorf("each entry in %s.options is at most %d characters", at, replyMaxOptionLen)
			}
			// 两个一样的选项在界面上无法区分，而回调只回文本，
			// 发送方也分不出用户点的是哪一个。
			if seen[o] {
				return fmt.Errorf("%s.options contains %q twice", at, o)
			}
			seen[o] = true
			r.Options[i] = o
		}
	case models.ReplyNumber:
		if len(r.Options) > 0 {
			return errors.New("type=number does not take " + at + ".options; it takes a numeric range")
		}
		if r.Min == nil || r.Max == nil {
			return fmt.Errorf("type=number requires both %s.min and %s.max", at, at)
		}
		if *r.Min >= *r.Max {
			return fmt.Errorf("%s.min (%g) must be less than %s.max (%g)", at, *r.Min, at, *r.Max)
		}
		if r.Step == nil {
			// 不给就按 1：整数是最常见的情形，而 0 或负数会让「落到网格上」这步除以零。
			one := 1.0
			r.Step = &one
		}
		if *r.Step <= 0 {
			return errors.New(at + ".step must be greater than 0")
		}
		if *r.Step > *r.Max-*r.Min {
			return fmt.Errorf("%s.step (%g) is wider than the whole range, leaving only one value to pick", at, *r.Step)
		}
		if len([]rune(r.Unit)) > 8 {
			return errors.New(at + ".unit is at most 8 characters; it is the unit shown after the number")
		}
		r.Unit = strings.TrimSpace(r.Unit)
	case models.ReplyText:
		// 文本回复没有选项。带了多半是把 type 写错了，直接说出来。
		if len(r.Options) > 0 {
			return errors.New("type=text does not take " + at + ".options; use type=choice to get buttons")
		}
	default:
		return fmt.Errorf("unsupported %s.type %q: it is one of choice / multi / number / text", at, r.Type)
	}
	r.Placeholder = strings.TrimSpace(r.Placeholder)
	if r.Placeholder != "" && r.Type != models.ReplyText {
		return errors.New(at + ".placeholder is only for type=text")
	}
	if len([]rune(r.Placeholder)) > replyMaxPlaceholder {
		return fmt.Errorf("%s.placeholder is at most %d characters", at, replyMaxPlaceholder)
	}
	return nil
}

// validateFields 组合回复的整体校验：数量、id，再逐个校验控件。
func (r *ReplySpec) validateFields() error {
	if r.Type != "" && r.Type != models.ReplyForm {
		return errors.New("reply.type and reply.fields cannot be used together: put the type on each field")
	}
	if len(r.Options) > 0 || r.Min != nil || r.Max != nil || r.Step != nil || r.Unit != "" ||
		r.Placeholder != "" || r.Required != nil {
		return errors.New("with reply.fields, options / min / max / step / unit / placeholder / required go on each field, not on reply")
	}
	if len(r.Fields) > replyMaxFields {
		return fmt.Errorf("reply.fields takes at most %d entries; three or fewer still reads like a quick reply", replyMaxFields)
	}
	seen := make(map[string]bool, len(r.Fields))
	for i := range r.Fields {
		f := &r.Fields[i]
		at := fmt.Sprintf("reply.fields[%d]", i)
		f.ID = strings.TrimSpace(f.ID)
		if !validFieldID(f.ID) {
			return fmt.Errorf("%s.id must be 1-%d characters of a-z, 0-9 and _; the callback keys each answer by it", at, replyMaxFieldIDLen)
		}
		f.Label = strings.TrimSpace(f.Label)
		if len([]rune(f.Label)) > replyMaxLabel {
			return fmt.Errorf("%s.label is at most %d characters", at, replyMaxLabel)
		}
		if seen[f.ID] {
			return fmt.Errorf("reply.fields uses the id %q twice", f.ID)
		}
		seen[f.ID] = true
		// 控件里不能省 type：单控件写法里「给了选项就当 choice」的猜测，放在一组控件里只会更难查。
		if strings.TrimSpace(f.Type) == "" {
			return fmt.Errorf("%s.type is required", at)
		}
		if err := f.validate(at); err != nil {
			return err
		}
	}
	r.Type = models.ReplyForm
	return nil
}

// validFieldID id 只许小写字母、数字、下划线：它要直接当回调 JSON 的键，也常被拿去当脚本里的变量名。
func validFieldID(id string) bool {
	if id == "" || len(id) > replyMaxFieldIDLen {
		return false
	}
	for _, c := range id {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '_' {
			return false
		}
	}
	return true
}
