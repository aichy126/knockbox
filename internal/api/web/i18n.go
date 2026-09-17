package web

import (
	"strings"
)

// Lang 对外页面的语言。**只有两种**，而且默认是英文。
//
// 为什么不做成可扩展的多语言框架：这几页的字加起来两百来条，
// 拖一个 go-i18n 进来，换回来的是一套 catalog 文件和一次构建步骤，
// 而真正的成本从来不在框架，在写字本身。第三种语言真的要来的时候再说。
type Lang string

const (
	LangEN Lang = "en"
	LangZH Lang = "zh"
)

// PickLang 决定这次请求用哪种语言。
//
// 顺序是 `?lang=` → Accept-Language → 英文。
// 查询参数放在最前面，是因为页面上那个语言开关就靠它；
// 同时它也让「审核员/用户想看另一种语言」这件事不依赖改浏览器设置。
//
// **默认英文**：这是一个面向全球的开源项目，中文是其中一种，不是基准。
func PickLang(query, acceptLanguage string) Lang {
	switch strings.ToLower(strings.TrimSpace(query)) {
	case "zh", "zh-cn", "zh-hans":
		return LangZH
	case "en", "en-us":
		return LangEN
	}
	// Accept-Language 只看有没有中文：这里不做 q 值排序。
	// 真实的头长这样 `zh-CN,zh;q=0.9,en;q=0.8`，中文用户一定带 zh；
	// 而排在前面的是不是 zh，对「给他看中文还是英文」这个二选一没有影响。
	if strings.Contains(strings.ToLower(acceptLanguage), "zh") {
		return LangZH
	}
	return LangEN
}

// Attr 放进 <html lang="…">。屏幕阅读器和浏览器的翻译提示都读它。
func (l Lang) Attr() string {
	if l == LangZH {
		return "zh-CN"
	}
	return "en"
}

// Other 另一种语言，给页面上那个开关用。
func (l Lang) Other() Lang {
	if l == LangZH {
		return LangEN
	}
	return LangZH
}

// T 取这一语的全部文案。未知语言回落英文，不 panic ——
// 一个拼错的 ?lang= 不该把页面打没。
func T(l Lang) Texts {
	if t, ok := texts[l]; ok {
		return t
	}
	return texts[LangEN]
}

// Texts 对外页面上的每一句话。
//
// **只收公共接入这条路上的字**：接入页、发送说明页、以及它们的错误落地页。
// 管理界面（/login 与 /admin/*）是给服务器运维者看的，不在这里。
type Texts struct {
	// 语言开关上显示的是【另一种语言】的名字，所以这里写的是自身的名字，
	// 由调用方取 Other() 那一份。
	LangName string

	// ── 接入页 ──────────────────────────────────────────────
	JoinTitle      string // <title>
	JoinHeading    string
	JoinLede       string
	JoinValidFor   string // 「%d 分钟内有效 · 只能用一次」
	JoinOpenHere   string
	JoinOpenHint   string
	JoinAfterScan  string
	JoinStep1      string
	JoinStep2      string
	JoinStep3      string
	JoinNoAccount  string
	JoinHowTo      string
	JoinCopy       string
	JoinCopied     string
	JoinPaired     string
	JoinPairedHint string
	JoinYourURL    string
	JoinYourURLSub string
	JoinOpenSend   string
	JoinKeepSecret string
	JoinAddDevice  string

	// ── 发送说明页 ──────────────────────────────────────────
	GuideDocsTitle    string // /docs 形态的大标题，会接上服务器名
	GuideDocsLede     string
	GuideChannelLede  string
	GuideYourURL      string
	GuideYourURLSub   string
	GuideSampleURL    string
	GuideSampleURLSub string
	GuideGoJoin       string
	GuidePlaceholder  string // 发送地址里的 <你的TOKEN>
	GuideHowToJoin    string
	GuideInstallH     string
	GuideInstallP     string
	GuideChannelH     string
	GuideChannelP     string
	GuideHowToSend    string
	GuideAgentH       string
	GuideAgentP       string
	GuidePromptH      string
	GuideMCPH         string
	GuideMCPP         string
	GuideMCPTabAsk    string // 「发给 AI 助手」那一页
	GuideMCPTabJSON   string
	GuideMCPTabCLI    string
	GuideMCPAsk       string // 发给 AI 助手的那段话，地址已经填好
	GuideOtherH       string
	GuideLimitsH      string
	GuideNoLimits     string
	GuideLimitCh      string
	GuideLimitDay     string
	GuideLimitKeep    string
	GuideLimit429     string
	GuideSelfHostH    string
	GuideSelfHostP    string
	GuideTokenWarn    string
	GuideSend         string
	GuideSending      string
	GuideSent         string
	GuideSendFail     string
	GuideCopy         string
	GuideCopied       string

	// ── 错误与提示 ──────────────────────────────────────────
	ErrBadLinkTitle  string
	ErrBadLinkDetail string
	ErrGone          string
	ErrNoSample      string
	ErrJoinTooFast   string
	ErrIssueFailed   string
	ErrQRFailed      string
}

var texts = map[Lang]Texts{
	LangEN: {
		LangName: "English",

		JoinTitle:   "Join %s",
		JoinHeading: "Join %s",
		JoinLede: "Scan the code below with Knockbox and you have your own inbox. " +
			"No sign-up, no password. After that, anything can send a notification to your phone — " +
			"the laundry has finished, a parcel is downstairs, the thing you were watching dropped in price.",
		JoinValidFor:  "Valid for %d minutes · single use",
		JoinOpenHere:  "Open on this iPhone",
		JoinOpenHint:  "If Knockbox is already installed, this connects it directly",
		JoinAfterScan: "After you scan",
		JoinStep1: "A channel appears in the app, and this page hands you " +
			"<b>a send page of your own</b>. Bookmark it.",
		JoinStep2: "That page has a worked example of every format, each with a " +
			"<b>Send this one</b> button — press it and your phone really does buzz.",
		JoinStep3: "Give the address to anything that can make one HTTP request: " +
			"home automation, a price watcher, a script you wrote, or your own coding agent.",
		JoinNoAccount: "<b>There is no account or password.</b> Your identity is the credential on the device. " +
			"<b>Lose every device and it is gone</b> — to add a second device, generate a code " +
			"from the app that is already connected.",
		JoinHowTo:      "How it works",
		JoinCopy:       "Copy",
		JoinCopied:     "Copied",
		JoinPaired:     "Connected",
		JoinPairedHint: "This device is yours now.",
		JoinYourURL:    "Your send page",
		JoinYourURLSub: "Bookmark the address below. The examples, the test button and the prompt " +
			"for an AI agent are all on that page.",
		JoinOpenSend:   "Open the send page",
		JoinKeepSecret: "<b>Do not post it anywhere public</b> — anyone holding it can send to your channel.",
		JoinAddDevice:  "To add another device, generate a code in the app.",

		GuideDocsTitle: "Using %s",
		GuideDocsLede: "Anything that can make one HTTP request can push a message to your iPhone — " +
			"home automation, a price watcher, a script you wrote, or your own coding agent.",
		GuideChannelLede: "This page belongs to one channel, and the address already carries its token. " +
			"Bookmark it — how to send, what it looks like on the phone, and what to hand an AI agent are all here. " +
			"Every example has a “Send this one” button, so you can see it work before copying anything.",
		GuideYourURL:      "Your send address",
		GuideYourURLSub:   "Every example below uses this address. Copy and it works.",
		GuideSampleURL:    "A send address looks like this",
		GuideSampleURLSub: "Once you join, <b>you get this same page with the token filled in</b>, and every example gains a “Send this one” button.",
		GuideGoJoin:       "Go to the join page",
		GuidePlaceholder:  "<YOUR_TOKEN>",
		GuideHowToJoin:    "Joining",
		GuideInstallH:     "Install the app, scan the code",
		GuideInstallP: `Download Knockbox and scan the QR code on the <a href="/">join page</a>. ` +
			"No sign-up, no password — once you scan you have your own inbox and a first channel. " +
			"<b>No account also means losing every device loses the identity</b>; " +
			"to add a device, generate a code in the app that is already connected.",
		GuideChannelH: "One channel per thing",
		GuideChannelP: "Each channel has its own send address, sound, interruption level and mute switch. " +
			"Give “home” and “deliveries” one each, and only the first will wake you at night. " +
			"In the app, open a channel → “…” at the top right → Channel settings to get back to its send page.",
		GuideHowToSend: "Sending",
		GuideAgentH:    "Hand it to an AI agent",
		GuidePromptH:   "Paste one block into it",
		GuideAgentP: "Copy the whole block into Claude Code or any other agent that can run commands. " +
			"The address is already in it; nothing needs changing.",
		GuideMCPH: "Or connect it as an MCP server",
		GuideMCPP: "The same address also speaks MCP over streamable HTTP, with a single tool that sends one " +
			"notification — for a client that cannot run commands, and so the block above does not have to be " +
			"pasted into every project. Add it in whichever form your client takes: hand the first block to an " +
			"assistant and it configures itself, paste the JSON into a config file, or run the command in " +
			"Claude Code. <b>The endpoint carries exactly the permission of the send address</b>: " +
			"post to this channel, read nothing.",
		GuideMCPTabAsk:  "Ask an assistant",
		GuideMCPTabJSON: "JSON config",
		GuideMCPTabCLI:  "Claude Code",
		GuideMCPAsk: `Please add an MCP server for me:
- Name: knockbox
- Transport: Streamable HTTP (remote http)
- URL: {{URL}}
It has one tool, knock, which sends a notification to my phone.
Once it is added, call it to send me a test notification.`,
		GuideOtherH:    "Other",
		GuideLimitsH:   "Limits",
		GuideNoLimits:  "This server has no quotas.",
		GuideLimitCh:   "At most <b>%d</b> channels",
		GuideLimitDay:  "At most <b>%d</b> messages per 24 hours (a rolling window, not a daily reset)",
		GuideLimitKeep: "Messages are kept for <b>%d</b> days, then cleared automatically",
		GuideLimit429:  `Going over returns a clear <code class="code">429</code> saying when it recovers — <b>messages are never dropped silently</b>`,
		GuideSelfHostH: "Run your own",
		GuideSelfHostP: "The server is open source — one binary and one SQLite file, with the messages entirely on your own machine. " +
			"Joining works exactly as it does here, only the address is yours.",
		GuideTokenWarn: "A send address is the permission to post to that channel, so <b>do not publish it</b>. " +
			"If it leaks, rotate it once in the channel settings; the channel and its history are untouched.",
		GuideSend:     "Send this one",
		GuideSending:  "Sending…",
		GuideSent:     "Sent",
		GuideSendFail: "Could not send",
		GuideCopy:     "Copy",
		GuideCopied:   "Copied",

		ErrBadLinkTitle:  "This address is not valid",
		ErrBadLinkDetail: "The channel may have been deleted, or its token was rotated. Copy the address again from the app.",
		ErrGone:          "This address is no longer valid",
		ErrNoSample:      "No such example",
		ErrJoinTooFast:   "Too many join requests. Try again in a moment.",
		ErrIssueFailed:   "Could not issue a pairing code: ",
		ErrQRFailed:      "Could not generate the QR code: ",
	},

	LangZH: {
		LangName: "中文",

		JoinTitle:   "接入 %s",
		JoinHeading: "接入 %s",
		JoinLede: "用 Knockbox 扫下面的二维码，就有了自己的收件身份。不需要注册，不需要密码。" +
			"之后任何东西都能给你的手机发通知——洗衣机洗完了、快递到楼下了、盯的东西降价了。",
		JoinValidFor:   "%d 分钟内有效 · 只能用一次",
		JoinOpenHere:   "就在这台 iPhone 上打开",
		JoinOpenHint:   "已经装了 Knockbox 的话，点它直接接入",
		JoinAfterScan:  "扫完之后",
		JoinStep1:      "app 里会出现一个频道，同时这一页会给出<b>只属于你的发送页</b>。收藏它。",
		JoinStep2:      "那一页上每种写法都有现成的例子，旁边还有一颗「发这条」——<b>按一下手机就真的响</b>，不用先照着抄。",
		JoinStep3:      "把地址交给任何能发一条 HTTP 请求的东西：家里的自动化、网上的价格监控、你写的小脚本，或者你自己的 AI agent。",
		JoinNoAccount:  "<b>没有账号密码。</b>你的身份就是设备上那把凭据。<b>所有设备都丢掉的话身份就找不回了</b>——想多一台设备，在已经接入的 app 里出码给新设备扫。",
		JoinHowTo:      "怎么用",
		JoinCopy:       "复制",
		JoinCopied:     "已复制",
		JoinPaired:     "已接入",
		JoinPairedHint: "这台设备已经是你的了。",
		JoinYourURL:    "你的专属发送页",
		JoinYourURLSub: "收藏下面这条地址。发送示例、测试按钮、给 AI agent 的提示词都在那一页。",
		JoinOpenSend:   "打开发送页",
		JoinKeepSecret: "<b>别把它贴到公开的地方</b>——拿到它就能往你的频道发消息。",
		JoinAddDevice:  "要再加一台设备，去 app 里出码。",

		GuideDocsTitle: "怎么用 %s",
		GuideDocsLede: "任何能发一条 HTTP 请求的东西，一行命令就能把消息推到你的 iPhone 上 ——" +
			"家里的自动化、网上的价格监控、你写的小脚本，或者你自己的 AI agent。",
		GuideChannelLede: "这一页是这个频道专用的，地址里已经带好了你的 token。收藏它 —— 怎么发、发出来长什么样、怎么交代给 AI agent，都在这一页。" +
			"每条示例旁边都有「发这条」，按一下手机就会真的响，不用先照着抄。",
		GuideYourURL:      "你的发送地址",
		GuideYourURLSub:   "下面每条示例里的地址都是它，复制就能用。",
		GuideSampleURL:    "发送地址长这样",
		GuideSampleURLSub: "扫码接入后，<b>你会拿到一份填好 TOKEN 的同一页</b>，每条示例旁边还多一颗「发这条」，按下去手机就响。",
		GuideGoJoin:       "去接入页扫码",
		GuidePlaceholder:  "<你的TOKEN>",
		GuideHowToJoin:    "怎么接",
		GuideInstallH:     "装 app，扫码",
		GuideInstallP: `下载 Knockbox，打开<a href="/">接入页</a>扫二维码。不需要注册，不需要密码 ——` +
			"扫完你就有了自己的收件身份和第一个频道。<b>没有账号密码意味着设备全丢就找不回</b>；" +
			"想多一台设备，在已接入的 app 里出码给新设备扫。",
		GuideChannelH: "一个频道一个用途",
		GuideChannelP: "每个频道有自己独立的发送地址，也有自己的铃声、打扰级别和静音开关。" +
			"给「家里」和「快递」各建一个，半夜就只有前者会把你吵醒。" +
			"在 app 里点进频道 → 右上角「…」→ 频道设置，能随时回到它的发送页。",
		GuideHowToSend: "怎么发",
		GuideAgentH:    "交给 AI agent",
		GuidePromptH:   "整段提示词粘给它",
		GuideAgentP:    "整段复制给 Claude Code 或任何能执行命令的 agent。地址已经在里面了，不用改任何东西。",
		GuideMCPH:      "或者接成 MCP",
		GuideMCPP: "同一个地址也说 MCP（streamable HTTP），只有一件工具：发一条通知——" +
			"给执行不了命令的客户端用，也省掉每个项目粘一遍提示词。三种加法按客户端选：" +
			"第一段整段发给 AI 助手，它自己会配；JSON 填进客户端的配置文件；命令在 Claude Code 里跑。" +
			"<b>这个端点的权限和发送地址完全相同</b>：只能往这个频道发，读不到任何东西。",
		GuideMCPTabAsk:  "发给 AI 助手",
		GuideMCPTabJSON: "JSON 配置",
		GuideMCPTabCLI:  "Claude Code",
		GuideMCPAsk: `请帮我添加一个 MCP 服务器：
- 名称：knockbox
- 传输方式：Streamable HTTP（远程 http）
- 地址：{{URL}}
它只有一件工具 knock，用来给我的手机发通知。
添加完成后调用它给我发一条测试通知。`,
		GuideOtherH:    "其它",
		GuideLimitsH:   "限制",
		GuideNoLimits:  "这台服务器没有配额限制。",
		GuideLimitCh:   "最多 <b>%d</b> 个频道",
		GuideLimitDay:  "每 24 小时最多 <b>%d</b> 条（滚动窗口，不是按自然日清零）",
		GuideLimitKeep: "消息保留 <b>%d</b> 天，之后自动清理",
		GuideLimit429:  `超了会明确返回 <code class="code">429</code> 并说明多久恢复，<b>不会静默丢消息</b>`,
		GuideSelfHostH: "自己搭一个",
		GuideSelfHostP: "服务端是开源的，一个二进制加一个 SQLite 文件，消息完全存在你自己的机器上。" +
			"接入方式和这里一模一样，只是地址换成你自己的。",
		GuideTokenWarn: "发送地址等同于往这个频道发消息的权限，<b>别公开贴出来</b>。" +
			"泄露了就在 app 的频道设置里轮换一次，频道和历史都不受影响。",
		GuideSend:     "发这条",
		GuideSending:  "发送中…",
		GuideSent:     "已发送",
		GuideSendFail: "发送失败",
		GuideCopy:     "复制",
		GuideCopied:   "已复制",

		ErrBadLinkTitle:  "这个地址无效",
		ErrBadLinkDetail: "频道可能已经被删除，或者 token 被轮换过。到 app 里重新复制一次。",
		ErrGone:          "这个地址已经失效了",
		ErrNoSample:      "没有这条示例",
		ErrJoinTooFast:   "接入请求过于频繁，请稍后再试。",
		ErrIssueFailed:   "签发配对码失败: ",
		ErrQRFailed:      "生成二维码失败: ",
	},
}

// LangSwitch 页面右上角那个开关。显示的是【另一种语言】的名字，
// 点它就换过去——不写「切换语言」，因为一个中文用户看不懂英文的「Language」，
// 反过来也一样，而语言自己的名字两边都认得。
func LangSwitch(cur Lang) string {
	o := cur.Other()
	return `<a class="langsw" href="?lang=` + string(o) + `" hreflang="` + o.Attr() + `">` +
		E(T(o).LangName) + `</a>`
}

// LangCSS 开关的样式。固定在右上角，不参与各页自己的栅格。
const LangCSS = `
.langsw{position:fixed;top:14px;right:16px;z-index:9;font-size:12.5px;padding:5px 10px;
  border:1px solid var(--line);border-radius:999px;background:var(--card);
  color:var(--muted-fg);text-decoration:none}
.langsw:hover{color:var(--fg);border-color:var(--fg)}
`
