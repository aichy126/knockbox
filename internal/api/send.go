package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/aichy126/igo/res"
	"github.com/aichy126/knockbox/internal/library/bytesize"
	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
)

// bindSend 按 Content-Type 解析一条消息。
//
// 四种都收，是为了让 curl 怎么写都行——自建工具的门面就是那一行命令：
//
//	curl -d "构建成功" .../api/v1/send/<token>
//	curl ".../api/v1/send/<token>?text=hi&title=CI"
//	curl -F "title=报警" -F "text=磁盘 92%" ...
//
// 标量参数一律也能从 query string 取，覆盖 body 里的同名值。
func bindSend(c *gin.Context, maxBody int64) (service.SendInput, error) {
	var in service.SendInput

	ct := c.ContentType()

	// multipart 不预读 body。
	//
	// 下面那段「先读原始 body」是为了 `curl -d "正文"` 那条招牌命令，但它带着
	// 正文上限；multipart 里可能夹着几 MB 的图片，被这个上限一截，
	// 表单解析就失败，附件【静默消失】而接口照样返回成功——最坏的那种失败。
	// 附件的大小限制该由 storage.max_upload_mb 管，不该由正文上限顺手管。
	var raw []byte
	if !strings.HasPrefix(ct, "multipart/form-data") {
		var err error
		// 多读一个字节用来判断有没有超限。
		// 只 LimitReader 不判断的话，超长的正文会被【静默截断】落库：
		// 调用方收到成功，而存下来的是半截内容——这比直接报错糟得多。
		raw, err = io.ReadAll(io.LimitReader(c.Request.Body, maxBody+1))
		if err != nil {
			return in, err
		}
		if int64(len(raw)) > maxBody {
			return in, fmt.Errorf("body exceeds the limit (%s)", bytesize.Human(maxBody))
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	}
	switch {
	case strings.HasPrefix(ct, "application/json"):
		if err := c.ShouldBindJSON(&in); err != nil {
			return in, err
		}
	case strings.HasPrefix(ct, "multipart/form-data"),
		strings.HasPrefix(ct, "application/x-www-form-urlencoded"):
		in.Title = c.PostForm("title")
		in.Body = firstNonEmpty(c.PostForm("body"), c.PostForm("text"))
		in.Type = c.PostForm("type")
		in.Link = c.PostForm("link")
		in.Copy = c.PostForm("copy")
		in.CollapseID = c.PostForm("collapse_id")
		in.IdemKey = c.PostForm("idem_key")
		// 「整个 body 当正文」这条兜底只对 urlencoded 成立。
		// multipart 的 raw 是空的（上面刻意没读），当成正文会把一整段
		// Content-Disposition 报文写进消息里。
		if in.Title == "" && in.Body == "" && !strings.HasPrefix(ct, "multipart/form-data") {
			in.Body = string(raw)
		}
		if v := c.PostForm("file"); v != "" {
			in.File = v
		}
	default:
		// text/plain、没有 Content-Type、以及任何其它类型：整个 body 就是正文。
		in.Body = string(raw)
	}

	// query 覆盖，让纯 GET 也能发完整的一条
	if v := c.Query("title"); v != "" {
		in.Title = v
	}
	if v := firstNonEmpty(c.Query("body"), c.Query("text")); v != "" {
		in.Body = v
	}
	if v := c.Query("type"); v != "" {
		in.Type = v
	}
	if v := c.Query("link"); v != "" {
		in.Link = v
	}
	if v := c.Query("copy"); v != "" {
		in.Copy = v
	}
	if v := c.Query("idem_key"); v != "" {
		in.IdemKey = v
	}
	if v := c.GetHeader("X-Idempotency-Key"); v != "" && in.IdemKey == "" {
		in.IdemKey = v
	}
	bindReplyShorthand(c, &in)
	return in, nil
}

// bindReplyShorthand 收「可回复」的平铺写法。
//
// 完整形态是一个对象（reply.type / reply.options / …），JSON 调用方直接给它就好。
// 但这台服务器的门面是一行 curl，而 query string 和表单里表达不出嵌套对象，
// 所以另收一组平铺参数，在这里归一成同一个对象 —— 和「整个 body 就是正文」
// 是同一种思路：怎么写都行，服务端负责归一。
//
// 只在调用方【没有】给完整对象时才拼：两种写法同时出现说明调用方在用 JSON，
// 那就以它写的对象为准，别用简写去覆盖它。
func bindReplyShorthand(c *gin.Context, in *service.SendInput) {
	if in.Reply != nil {
		return
	}
	// 表单里可以直接写一段 reply JSON。
	//
	// 这条路不能省：**带附件的消息只能用 multipart 发**，而 multipart 里
	// 表达不出嵌套对象。不接的话「带图 + 要回复」这个组合根本发不出来，
	// 而且不报错 —— 那条消息只是静默变成了单向的，发送方还在等答案。
	//
	// 解析失败要报错而不是退回简写：调用方明确写了一个 reply 对象，
	// 说明它想要回复；把一个写坏的 JSON 当成「没写」是最坏的处理方式。
	if raw := firstNonEmpty(c.PostForm("reply"), c.Query("reply")); raw != "" {
		var spec service.ReplySpec
		if err := json.Unmarshal([]byte(raw), &spec); err != nil {
			in.ReplyParseError = fmt.Errorf("reply is not valid JSON: %w", err)
			return
		}
		in.Reply = &spec
		return
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := c.Query(k); v != "" {
				return v
			}
			if v := c.PostForm(k); v != "" {
				return v
			}
		}
		return ""
	}
	choices := pick("choices", "reply_choices")
	hook := pick("reply_webhook")
	wantText := isTrue(pick("reply_text"))
	if choices == "" && hook == "" && !wantText {
		return
	}
	spec := &service.ReplySpec{Webhook: hook}
	if choices != "" {
		// 逗号分隔。选项里本来就不该有逗号（它是一句给人点的短语，不是数据），
		// 真要带的话就用 JSON 那条路。
		for _, o := range strings.Split(choices, ",") {
			if o = strings.TrimSpace(o); o != "" {
				spec.Options = append(spec.Options, o)
			}
		}
		spec.Type = models.ReplyChoice
	} else if wantText {
		spec.Type = models.ReplyText
	}
	if v := pick("reply_timeout"); v != "" {
		// 解析不了就留 0（不限），不在这里报错：
		// 校验统一在 ReplySpec.validate 里做一次，两处各报一套说法容易分叉。
		n, _ := strconv.Atoi(v)
		spec.Timeout = n
	}
	in.Reply = spec
}

// isTrue 收 reply_text=1 / true / yes 这几种写法。
// 只要出现了这个参数就当成想要文本回复 —— 写 reply_text= 空值的人是想开它，不是想关它。
func isTrue(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "no", "off":
		return false
	case "":
		return false
	}
	return true
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

// attachInline 处理 `curl -F "image=@chart.png"` —— 一条命令直接带图，
// 不用先 /upload 再引用。批量投递才需要先传后发（N 个频道共用一个 uid）。
func (s *Server) attachInline(c *gin.Context, in *service.SendInput) error {
	if in.File != "" || s.Files == nil {
		return nil
	}
	fh, err := c.FormFile("image")
	if err != nil {
		if fh, err = c.FormFile("file"); err != nil {
			return nil // 没带附件是正常路径
		}
	}
	max := s.Files.MaxBytes()
	if fh.Size > max {
		return fmt.Errorf("attachment exceeds the size limit (%s)", bytesize.Human(max))
	}
	src, err := fh.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	// 即便声明的 Size 达标也照样封顶：Size 来自请求方，不可信。
	data, err := io.ReadAll(io.LimitReader(src, max+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > max {
		return fmt.Errorf("attachment exceeds the size limit (%s)", bytesize.Human(max))
	}
	f, err := s.Files.Store(middleware.UserID(c), fh.Filename, data)
	if err != nil {
		return err
	}
	in.File = f.UID
	if in.Type == "" {
		in.Type = models.TypeImage
	}
	return nil
}

// errSendTooFast 超过发送速率时给调用方的说法。
// 说清是「频率」而不是「额度」：前者等一下就好，后者要改用法，两者的下一步不同。
const errSendTooFast = "发送过于频繁，请降低频率后重试"

// multipartSlack multipart 的分隔符、表单字段和头部占的额外字节。
// 请求体上限 = 附件上限 + 它，免得刚好卡在上限的附件被整体拒掉。
const multipartSlack = 1 << 20

// checkReplyAddr 发送时先判一次回调地址。
//
// 安全边界不在这里（真正的判定在 Webhook 拨号那一刻，见 service/webhook.go），
// 这里判是为了让发送方【立刻】知道地址不行。少了这一步，一条带内网回调地址的消息
// 照样发出去、用户在手机上认真回了一次，而回调在服务端静默失败 ——
// 发送方一直等一个永远不来的答案，两边都不会自己发现。
func (s *Server) checkReplyAddr(in *service.SendInput) error {
	if in.Reply == nil || s.Hooks == nil {
		return nil
	}
	return s.Hooks.CheckAddr(in.Reply.Webhook)
}

func (s *Server) send(c *gin.Context) {
	in, err := bindSend(c, s.BodyMaxBytes)
	if err != nil {
		res.Rfail(c, "cannot parse the request: "+err.Error())
		return
	}
	if err := s.attachInline(c, &in); err != nil {
		res.Rfail(c, "cannot process the attachment: "+err.Error())
		return
	}
	ch := middleware.Channel(c)
	// 速率限制排在配额之前：它更便宜，而且挡的是「这个 token 正在被刷」，
	// 越早挡住越好，不该先去查一遍数据库。
	if !s.sendLimit.Allow(ch.Token) {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 1, "msg": errSendTooFast})
		return
	}
	// 配额超了要【明确报错】，不能静默丢——一个通知系统最不能干的事，
	// 就是让上游以为发出去了而其实没有。
	if err := s.quota().CheckSend(ch.UserId); err != nil {
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"code": 1, "msg": err.Error()})
		return
	}
	// 地址检查排在配额之后：它要做一次 DNS 解析，比前两道都贵。
	if err := s.checkReplyAddr(&in); err != nil {
		res.Rfail(c, err.Error())
		return
	}
	out, err := service.NewSend(s.DAO).Deliver(ch, in, c.ClientIP())
	if err != nil {
		res.Rfail(c, err.Error())
		return
	}
	s.Notify()
	res.Rsucc(c, out)
}

type batchBody struct {
	Tokens []string `json:"tokens"`
	service.SendInput
}

// sendBatch 一次投到 N 个频道。调用方本来就持有这 N 个 token，
// 因此逐个独立校验即可，不需要引入权限更大的广播凭据。
func (s *Server) sendBatch(c *gin.Context) {
	var body batchBody
	if err := c.ShouldBindJSON(&body); err != nil {
		res.Rfail(c, "cannot parse the request: "+err.Error())
		return
	}
	if len(body.Tokens) == 0 {
		res.Rfail(c, "tokens cannot be empty")
		return
	}
	if len(body.Tokens) > 200 {
		res.Rfail(c, "at most 200 channels per call")
		return
	}
	// 逐个 token 判速率。被限的那几个各自报错，不连累同一批里其余的频道——
	// 和「一个坏 token 不该让整批失败」是同一条原则。
	out := service.NewSend(s.DAO).Batch(body.Tokens, body.SendInput, c.ClientIP(),
		func(tok string) error {
			if s.sendLimit.Allow(tok) {
				return nil
			}
			return errors.New(errSendTooFast)
		})
	s.Notify()

	// 部分失败不改 HTTP 状态：调用方要逐条看 results，
	// 一个坏 token 不该让另外 N-1 个成功的投递也被当成失败。
	ok := 0
	for _, r := range out {
		if r.OK {
			ok++
		}
	}
	res.Rsucc(c, gin.H{"results": out, "ok": ok, "failed": len(out) - ok})
}

func (s *Server) sendPath(c *gin.Context) {
	if middleware.SendToken(c) == "" {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 1, "msg": "missing channel token"})
		return
	}
	s.send(c)
}
