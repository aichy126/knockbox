package api

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/aichy126/knockbox/internal/middleware"
	"github.com/aichy126/knockbox/internal/models"
	"github.com/aichy126/knockbox/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCP 接入：让 agent 把这台服务器当成一个工具，而不是一条要拼的 curl 命令。
//
// 它不是一条新的能力，是同一条发送路径的另一个接入面——权限、速率限制、配额
// 和 /api/v1/send 完全一样，凭据也是同一个频道 token。两件事它比 curl 强：
// 带换行和引号的 markdown 正文不用过 shell 转义；工具常驻在 agent 的工具表里，
// 不需要每个项目粘一遍提示词。shell 用得顺手的人继续用 curl 就好。
//
// 形态：Streamable HTTP，挂在主服务的 /mcp 与 /mcp/<token>，与 Web 同端口。
// 无状态（WithStateLess）：进程重启是常态，有状态 session 只会添乱。

// mcpCtxKey 从 HTTP 层传给工具处理函数的那几样东西。
//
// 工具处理函数拿不到 *gin.Context：它由 MCP 库在解析完 JSON-RPC 之后调用，
// 中间隔着协议层。能穿过去的只有 request 的 context，所以频道和来源 IP
// 在 gin 这一侧就塞进去。
type mcpCtxKey struct{ name string }

var (
	mcpChannelKey = mcpCtxKey{"channel"}
	mcpIPKey      = mcpCtxKey{"ip"}
)

// mcpInstructions 这套工具的说明，客户端连上时就读到。
//
// 写给 agent 看，所以重点不是字段表（那在工具的 schema 里），而是**什么时候该敲**：
// 不写这一段，agent 要么不用它，要么每跑完一步都敲一下。
const mcpInstructions = `Knockbox delivers a push notification to its owner's iPhone.
The channel is fixed by the credential in this endpoint — there is nothing to address or choose.

Knock when the person needs to know something and is not watching the screen: a long task
finished, something broke, a decision is needed before you can continue. Do not knock for
the routine progress of a task they are watching, and do not knock twice for one event —
use collapse_id for a status that keeps updating, idem_key when retrying a failed call.

Put what happened in title; it is the first line of the notification. Put the detail in body,
which can be long: type=markdown renders code blocks, tables, lists and quotes. Several
parallel results (branch, duration, result) read better as type=card with items.

You can ask for an answer, but it does not come back through this tool. Set reply with a
webhook you control: the person picks an option (or types a line) on their phone, and the
server POSTs their answer to that URL. Use type=choice with 2-4 short options for a decision,
type=text when the answer is open-ended. Set reply.timeout only when something happens by
itself if they do not answer in time — an open question with no default should wait
indefinitely rather than expire. If you have no endpoint that can receive an HTTP POST,
leave reply out: say in the message that you are waiting, then stop and wait.

When you asked for a reply, read reply_until in the result: it is when the answer stops being
accepted, or absent if there is no limit. Then stop and wait for your webhook to be called;
nothing arrives through this tool.

Read the result before reporting success: muted=true means it was stored and synced but the
channel is silent right now, so no notification appeared. devices=0 means no phone is paired
with this server at all. dedup=true means idem_key matched an earlier message and nothing new
was sent.`

// newMCPServer 装一个只有一件工具的 MCP 服务。
//
// 刻意只有一件。这台服务器只做一件事，而工具越多，agent 选错的概率越大——
// 「查配额」「列频道」这类工具省不了它任何一步，却每次都要被读一遍。
func newMCPServer(s *Server) *server.MCPServer {
	srv := server.NewMCPServer("knockbox", s.Version,
		server.WithInstructions(mcpInstructions),
		server.WithRecovery(), // 一次 panic 不该拖垮整个端点
	)
	srv.AddTool(
		mcp.NewTool("knock",
			mcp.WithDescription("Send a push notification to the owner's iPhone. "+
				"Use it to tell them something they would otherwise have to watch the screen for: "+
				"a task finished, something broke, a decision is needed."),
			mcp.WithString("title", mcp.Description(
				"One line saying what happened. It is the first line of the notification.")),
			mcp.WithString("body", mcp.Description(
				"The detail. Can be long — it is not limited by the notification size.")),
			mcp.WithString("type", mcp.Enum(models.TypeText, models.TypeMarkdown,
				models.TypeLink, models.TypeCard), mcp.Description(
				"Defaults to text. markdown renders code blocks, tables and lists; "+
					"card renders items as labelled rows; link makes the notification open link.")),
			mcp.WithString("link", mcp.Description(
				"A URL the notification can open directly, e.g. the failed build.")),
			mcp.WithArray("items", mcp.Description(
				"Rows for type=card, in the order given: k is the label, v the value, "+
					"style one of ok / warn / error / muted."),
				mcp.Items(map[string]any{
					"type": "object",
					"properties": map[string]any{
						"k":     map[string]any{"type": "string"},
						"v":     map[string]any{"type": "string"},
						"style": map[string]any{"type": "string", "enum": []string{"ok", "warn", "error", "muted"}},
					},
					"required": []string{"k", "v"},
				})),
			mcp.WithString("collapse_id", mcp.Description(
				"Notifications sharing this value replace each other on the phone. "+
					"Use one value for a status that keeps updating, so it stays one notification.")),
			mcp.WithString("idem_key", mcp.Description(
				"Retrying with the same value reuses the earlier message instead of pushing again.")),
			mcp.WithObject("reply", mcp.Description(
				"Ask for an answer. The answer is POSTed as JSON to reply.webhook — it does not "+
					"come back through this tool, so only use this when you control an HTTP endpoint."),
				mcp.Properties(map[string]any{
					"type": map[string]any{
						"type": "string",
						"enum": []string{models.ReplyChoice, models.ReplyMulti,
							models.ReplyNumber, models.ReplyText},
						"description": "choice: buttons, one tap. multi: checkboxes, several picked " +
							"then submitted. number: a slider between min and max. text: a field they type in.",
					},
					"options": map[string]any{
						"type": "array", "items": map[string]any{"type": "string"},
						"description": "For choice and multi: 2-10 labels, up to 40 characters each, " +
							"no duplicates. Not accepted for number or text.",
					},
					"min": map[string]any{
						"type":        "number",
						"description": "For type=number: the low end of the range. Required with max.",
					},
					"max": map[string]any{
						"type":        "number",
						"description": "For type=number: the high end of the range. Required with min.",
					},
					"step": map[string]any{
						"type": "number",
						"description": "For type=number: the grid the answer lands on. Defaults to 1. " +
							"Ask for 0.5 and you get 24.0 or 24.5, never 24.317.",
					},
					"unit": map[string]any{
						"type": "string",
						"description": "For type=number: up to 8 characters shown after the number " +
							"(°C, %, minutes). Display only.",
					},
					"timeout": map[string]any{
						"type": "integer",
						"description": "Seconds until the answer is no longer accepted. Omit for no limit. " +
							"Only set it when something happens by itself once the time is up.",
					},
					"webhook": map[string]any{
						"type": "string",
						"description": "Required. The URL the answer is POSTed to, as " +
							"{uid, channel, title, reply, replied_at}, signed with " +
							"X-Knockbox-Signature: sha256=HMAC-SHA256(body, channel token). " +
							"reply is a string, an array of strings for multi, a number for number.",
					},
				})),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.mcpKnock(ctx, req)
		},
	)
	return srv
}

// mcpKnock 发一条。
//
// 顺序与 /api/v1/send 里那段**必须一致**：速率限制（便宜，先挡住正在被刷的 token）
// → 配额（要明确报错，一个通知系统最不能干的是让上游以为发出去了）→ 落库投递。
// 失败一律用 NewToolResultError 回给 agent：那是它读得懂、能据此改下一步的形态，
// 而返回 error 在客户端那边显示成一次协议故障。
func (s *Server) mcpKnock(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ch, _ := ctx.Value(mcpChannelKey).(*models.Channel)
	if ch == nil {
		return nil, errors.New("no channel resolved")
	}
	in, err := mcpSendInput(req)
	if err != nil {
		return mcp.NewToolResultError("cannot parse the arguments: " + err.Error()), nil
	}
	if in.Title == "" && in.Body == "" && len(in.Items) == 0 {
		return mcp.NewToolResultError("title and body cannot both be empty"), nil
	}
	if !s.sendLimit.Allow(ch.Token) {
		return mcp.NewToolResultError(errSendTooFast), nil
	}
	if err := s.quota().CheckSend(ch.UserId); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if err := s.checkReplyAddr(&in); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	ip, _ := ctx.Value(mcpIPKey).(string)
	out, err := service.NewSend(s.DAO).Deliver(ch, in, ip)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	s.Notify()
	b, err := json.Marshal(out)
	if err != nil {
		return nil, err
	}
	return mcp.NewToolResultText(string(b)), nil
}

// mcpSendInput 把工具参数转成发送入参。
//
// 走一次 JSON 而不是逐个字段取：工具的参数名与 HTTP API 的 JSON 字段名就是同一份
// 定义（service.SendInput 的 tag），逐个手抄等于多一份会悄悄对不上的副本。
//
// file 是唯一被刻意丢掉的字段：MCP 这条路上传不了附件，而 uid 是别处传上来的引用，
// 留着它只是给一个猜 uid 的入口，换不来任何能力。
func mcpSendInput(req mcp.CallToolRequest) (service.SendInput, error) {
	var in service.SendInput
	raw, err := json.Marshal(req.GetArguments())
	if err != nil {
		return in, err
	}
	if err := json.Unmarshal(raw, &in); err != nil {
		return in, err
	}
	in.File = ""
	return in, nil
}

// mountMCP 把 MCP 挂到主服务。
//
// 两个挂载点是为了两种客户端写法：/mcp/<token> 让只能配一个 URL 的客户端也能接，
// /mcp 收 Authorization: Bearer。鉴权都走 SendAuth，和 curl 发送同一套凭据口径。
//
// GET 与 DELETE 也注册：它们是协议里的监听流与会话终止，不注册的话客户端在连接阶段
// 收到的是 404，而不是一个它认得的协议响应。
//
// 关掉监听流（GET 直接 405，协议允许）是因为这台服务器没有任何服务端→客户端的通知：
// 留着它只会挂一条永远没有内容的长连接，而反向代理会在读超时把它掐掉，
// 客户端于是反复重连，看起来像服务不稳定。
func mountMCP(r gin.IRouter, s *Server) {
	httpSrv := server.NewStreamableHTTPServer(newMCPServer(s),
		server.WithStateLess(true),
		server.WithDisableStreaming(true),
	)
	handler := func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), mcpChannelKey, middleware.Channel(c))
		ctx = context.WithValue(ctx, mcpIPKey, c.ClientIP())
		httpSrv.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
	}
	for _, path := range []string{"/mcp", "/mcp/:token"} {
		g := r.Group(path, middleware.SendAuth(s.DAO))
		g.POST("", handler)
		g.GET("", handler)
		g.DELETE("", handler)
	}
}
