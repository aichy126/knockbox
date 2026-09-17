package api

import (
	"net/http"
	"strings"

	"github.com/aichy126/knockbox/internal/api/web"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/aichy126/knockbox/internal/uierr"
	"github.com/gin-gonic/gin"
)

// reqLang 这次请求该用哪种语言。`?lang=` 优先，其次浏览器的 Accept-Language，默认英文。
//
// 不落 cookie：语言只影响这一次渲染，页面右上角那个开关随时能换。
// 存偏好意味着多一个要解释、要清除的状态，而换回去的代价只是再点一次。
func reqLang(c *gin.Context) web.Lang {
	return web.PickLang(c.Query("lang"), c.GetHeader("Accept-Language"))
}

// sendGuide 一个频道的发送页：说明和示例都在这一页，示例还能真发出去。
func (s *Server) sendGuide(c *gin.Context) {
	lang := reqLang(c)
	t := web.T(lang)
	token := c.Param("token")
	var ch models.Channel
	ok, err := s.DAO.Engine().Where("token = ? AND status = ?", token, models.StatusActive).Get(&ch)
	if err != nil || !ok {
		c.Data(http.StatusNotFound, "text/html; charset=utf-8",
			[]byte(web.NoticePage(lang, t.ErrBadLinkTitle, t.ErrBadLinkDetail)))
		return
	}
	url := s.ExternalURL + "/api/v1/send/" + ch.Token
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(web.GuidePage(web.GuideData{
		SiteName: s.Settings.SiteName(),
		Name:     channelName(ch.Meta, ch.Id),
		Host:     s.ExternalURL,
		Token:    ch.Token,
		// 配额只在公共模式下存在；自建的人看到一节「限制」只会困惑
		Public:        s.Public.Enabled,
		MaxChannels:   s.Public.MaxChannels,
		MaxPerDay:     s.Public.MaxPerDay,
		RetentionDays: s.Public.RetentionDays,
		Samples:       viewSamples(url, true, lang),
		Agent:         agentPrompt(url, lang),
		MCP:           mcpForms(s.ExternalURL, ch.Token, lang),
		Lang:          lang,
	})))
}

// docsPage 同一页的「还没接入」形态：地址是占位符，没有「发这条」。
func (s *Server) docsPage(c *gin.Context) {
	lang := reqLang(c)
	url := s.ExternalURL + "/api/v1/send/" + web.T(lang).GuidePlaceholder
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(web.GuidePage(web.GuideData{
		SiteName:      s.Settings.SiteName(),
		Host:          s.ExternalURL,
		Public:        s.Public.Enabled,
		MaxChannels:   s.Public.MaxChannels,
		MaxPerDay:     s.Public.MaxPerDay,
		RetentionDays: s.Public.RetentionDays,
		Samples:       viewSamples(url, false, lang),
		Agent:         agentPrompt(url, lang),
		MCP:           mcpForms(s.ExternalURL, web.T(lang).GuidePlaceholder, lang),
		Lang:          lang,
	})))
}

// sendTry 「发这条」。
//
// 走的是和正式发送**完全一样**的 Deliver 路径——测试用另一条代码路径的话，
// 它就证明不了正式路径是通的。内容也取自展示那条示例的同一份定义。
//
// 返回 JSON 而不是重定向：页面用 fetch 调它，不刷新也不跳锚点。
// 重定向会让浏览器重新载入并滚到那一节，视觉上就是「按一下页面自己动了」。
func (s *Server) sendTry(c *gin.Context) {
	fail := func(msg string) {
		c.JSON(http.StatusOK, gin.H{"ok": false, "msg": msg})
	}
	t := web.T(reqLang(c))
	var ch models.Channel
	ok, _ := s.DAO.Engine().Where("token = ? AND status = ?", c.Param("token"), models.StatusActive).Get(&ch)
	if !ok {
		fail(t.ErrGone)
		return
	}
	// 只认预置的几条，不接受页面传来的任意内容——这个接口没有鉴权
	// （凭据就在 URL 里），别让它变成一个能发任意消息的代理。
	sp := sampleByID(c.PostForm("sample"))
	if sp == nil {
		fail(t.ErrNoSample)
		return
	}
	// 这一页没有鉴权（凭据就在 URL 里），和正式发送走同一条速率限制。
	if !s.sendLimit.Allow(ch.Token) {
		fail(s.userText(c, uierr.SendTooFast))
		return
	}
	if err := s.quota().CheckSend(ch.UserId); err != nil {
		fail(err.Error())
		return
	}
	// 发的内容跟着页面语言走：英文页面上按「Send this one」，
	// 手机上收到的就该是英文那条，而不是页面写英文、通知是中文。
	in := sp.text(reqLang(c)).Input
	if sp.NeedsImage && s.Files != nil {
		if f, err := s.Files.Store(ch.UserId, "sample-chart.png", sampleChart()); err == nil {
			in.File = f.UID
		}
	}
	if _, err := service.NewSend(s.DAO).Deliver(&ch, in, c.ClientIP()); err != nil {
		fail(err.Error())
		return
	}
	s.Notify()
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// mcpForms 接 MCP 的三种写法，**地址都已经填好**。
//
// 客户端分三类，各要一种形态：能自己读说明去配的 AI 助手要一段话，要人手改配置文件的
// 要一段 JSON，有命令行的要一行命令。只给其中一种，另外两类的人得自己照着改一遍，而照着改是会错的。
func mcpForms(host, token string, lang web.Lang) []web.MCPForm {
	t := web.T(lang)
	url := host + "/mcp/" + token
	return []web.MCPForm{
		{Tab: t.GuideMCPTabAsk, Lang: "prompt",
			Code: strings.NewReplacer("{{URL}}", url).Replace(t.GuideMCPAsk)},
		{Tab: t.GuideMCPTabJSON, Lang: "json", Code: `{
  "mcpServers": {
    "knockbox": {
      "type": "http",
      "url": "` + url + `"
    }
  }
}`},
		{Tab: t.GuideMCPTabCLI, Lang: "bash",
			Code: "claude mcp add --transport http knockbox " + url},
	}
}

// agentPrompt 交给 AI agent 的整段提示词，**地址已经填好**。
//
// 用户的真实动作是选中、复制、贴进自己的 agent；
// 中间任何一步需要他自己替换，都是一次可能出错的摩擦。
func agentPrompt(url string, lang web.Lang) string {
	tpl := agentPromptEN
	if lang == web.LangZH {
		tpl = agentPromptZH
	}
	return strings.NewReplacer("{{URL}}", url).Replace(tpl)
}

const agentPromptZH = `我有一个 Knockbox 推送服务，可以把消息发到我的 iPhone 上。

发送地址（这就是我的频道，直接用，不用替换任何东西）：
  {{URL}}

四种写法都支持：
  curl -d "消息正文" {{URL}}
  curl "{{URL}}?title=标题&text=正文"
  curl -F "title=标题" -F "text=正文" -F "image=@图.png" {{URL}}
  curl -H 'Content-Type: application/json' -d '{"type":"markdown","title":"标题","body":"正文"}' {{URL}}

字段：
  type        text | markdown | image | link | card，缺省 text
  title       标题，通知第一行
  body        正文，可以写得很长；markdown 支持代码块、表格、列表、引用
  link        附一个链接，通知上可以直接打开
  items       type=card 时的键值对数组 [{"k":"分支","v":"main","style":"ok"}]，顺序稳定
  idem_key    幂等键，重试带同一个值不会重复推送
  collapse_id 折叠键，同一个值的新通知会顶掉旧的，适合进度刷新
  reply       要一个答复（见下），答复不从这条连接回来，而是 POST 到你给的地址

请在需要通知我的时候用它发消息：事情做完了、出了问题、需要我拿个主意。
标题一句话说清是什么事，正文放细节；几项并列的结果用 markdown 表格或 card。

要一个答复：
  curl "{{URL}}?title=窗帘 30 秒后自动打开&choices=打开,不要打开&reply_timeout=30&reply_webhook=https://你的地址/hook"

  或者写成对象：
  curl -H 'Content-Type: application/json' -d '{
    "title": "要部署到生产吗？",
    "reply": {"type":"choice","options":["部署","先别"],"webhook":"https://你的地址/hook"}
  }' {{URL}}

  type      choice 出按钮点一个，multi 勾几个再提交，number 拖滑块选一个数，text 出输入框
  options   choice / multi 的选项，2 到 10 项，每项不超过 40 字
  min max   type=number 必填的范围；step 不填按 1，unit 是显示在数字后面的单位（°C / 分钟）
  timeout   多少秒内有效，不填就一直可以回。只有「不回就会自动发生别的事」才需要设
  webhook   必填。我点了之后，服务端把答复 POST 到这个地址

  收到的内容：{"uid":"…","channel":"…","title":"…","reply":"不要打开","replied_at":1726…}
  reply 的类型随形态变：choice / text 是字符串，multi 是字符串数组，number 是数字。
  请求头带 X-Knockbox-Signature: sha256=<HMAC-SHA256(请求体, 频道 token)>，可以用它验来源。

  没有能收 POST 的地址就别用 reply：在消息里说清你在等，然后停下来等我。`

const agentPromptEN = `I have a Knockbox push service that delivers messages to my iPhone.

The send address (this is my channel — use it directly, nothing to replace):
  {{URL}}

Four forms all work:
  curl -d "the message body" {{URL}}
  curl "{{URL}}?title=Title&text=Body"
  curl -F "title=Title" -F "text=Body" -F "image=@chart.png" {{URL}}
  curl -H 'Content-Type: application/json' -d '{"type":"markdown","title":"Title","body":"Body"}' {{URL}}

Fields:
  type        text | markdown | image | link | card, defaults to text
  title       the first line of the notification
  body        the message; it can be long. markdown supports code blocks, tables, lists and quotes
  link        attach a link the notification can open directly
  items       key-value array for type=card, [{"k":"Branch","v":"main","style":"ok"}], order is kept
  idem_key    idempotency key; a retry carrying the same value does not push twice
  collapse_id collapse key; a new notification with the same value replaces the old one, good for progress
  reply       ask for an answer (below). The answer does not come back on this connection —
              it is POSTed to a URL you provide

Use it whenever you need to tell me something: a task finished, something broke, a decision is needed.
Put what happened in the title and the detail in the body; for several parallel results use a markdown table or a card.

Asking for an answer:
  curl "{{URL}}?title=Deploy to production?&choices=Deploy,Hold&reply_webhook=https://your-host/hook"

  Or as an object:
  curl -H 'Content-Type: application/json' -d '{
    "title": "Deploy 1.1.0 to production?",
    "reply": {"type":"choice","options":["Deploy","Hold"],"webhook":"https://your-host/hook"}
  }' {{URL}}

  type      choice: buttons, one tap. multi: checkboxes then submit. number: a slider. text: a field
  options   the labels for choice and multi, 2 to 10, up to 40 characters each
  min max   required for type=number; step defaults to 1, unit is shown after the number (°C / minutes)
  timeout   seconds until the answer is no longer accepted. Omit for no limit — only set it when
            something happens by itself once the time is up
  webhook   required. Once I answer, the server POSTs it to this URL

  What you receive: {"uid":"…","channel":"…","title":"…","reply":"Hold","replied_at":1726…}
  reply is a string for choice and text, an array of strings for multi, a number for number.
  Signed with X-Knockbox-Signature: sha256=<HMAC-SHA256(body, channel token)> so you can verify it.

  If you have no endpoint that can receive a POST, leave reply out: say in the message that you
  are waiting, then stop and wait.`
