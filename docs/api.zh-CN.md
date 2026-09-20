# 发送与集成

[English](api.md) · **简体中文**

脚本、CI、agent 想给手机发一条通知，需要知道的全部在这里。
怎么把服务端跑起来见 [deploy.zh-CN.md](deploy.zh-CN.md)。

- [开始之前](#开始之前)
- [发送](#发送)
- [消息字段](#消息字段)
- [附件](#附件)
- [要一个答复](#要一个答复)
- [一次发给多个频道](#一次发给多个频道)
- [接给 AI agent](#接给-ai-agent)
- [限流与配额](#限流与配额)
- [命令行](#命令行)
- [什么是公开契约](#什么是公开契约)
- [接口清单](#接口清单)

## 开始之前

**频道**相当于群机器人的收件线。每个频道有自己的**发送地址**，而这个地址就是凭据——
拿到它的人能往这个频道发消息，也只能往这个频道发。频道在 iOS 客户端里建，
每个频道的页面上有一条能直接粘的 `curl`。

```
https://push.example.com/api/v1/send/<频道 token>
```

`push.example.com` 就是你给 `server.external_url` 配的地址。下面所有例子都用它，
换成你自己的。

频道的**铃声、打扰级别、静音存在服务端**，不在消息里。发送方是一段脚本，
它无从知道什么对你算急。要区分紧急度就建两个频道，往不同地址发。

## 发送

收四种 Content-Type，标量参数也一律能从 query string 取，所以 `curl` 怎么写都行。

**整个 body 就是正文**，什么都不用配，收到的是一条纯文本通知：

```bash
curl -d "磁盘 92%" https://push.example.com/api/v1/send/<token>
```

**纯 GET**，给不方便发 POST 的场景：

```bash
curl "https://push.example.com/api/v1/send/<token>?title=CI&text=构建失败"
```

**multipart**，顺带发一张图：

```bash
curl -F "title=磁盘告警" -F "text=/data 92%" -F "image=@chart.png" \
  https://push.example.com/api/v1/send/<token>
```

**JSON**，完整形态：

```bash
curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "Build #1832 failed",
  "body": "**main** · 42s\n\n```go\nfunc Execute() {}\n```",
  "idem_key": "drone-build-1832"
}' https://push.example.com/api/v1/send/<token>
```

发送成功返回：

```json
{"code":0,"msg":"success","data":{"uid":"01J…","rev":184,"devices":2,"queued":true}}
```

`devices` 是这条消息给你几台设备排了推送，`queued` 是它有没有进推送队列。
静音的频道照样把消息存下来，并回一个 `"muted":true`——静音是不响，不是不收。

**正文不进 payload。** payload 只带摘要和消息 id，正文留在库里由 app 去取。
所以正文可以远超一条通知装得下的长度（上限由 `limit.body_max_kb` 决定，默认 1 MB），
而且推送永远发得出去——payload 装不下时会逐级降级，直到塞得进去。

### 读响应

**判断成败看 `code`，不要看 HTTP 状态码。** 所有响应是同一个包装，而一个「听懂了但拒绝」
的请求——附件超限、`reply` 写错——仍然是 HTTP 200，只是 `code` 非零、`msg` 里写着原因。

| | HTTP | 响应体 |
|---|---|---|
| 发出去了 | 200 | `{"code":0,"msg":"success","data":{…}}` |
| 被拒绝 | 200 | `{"code":1,"msg":"<哪里不对>","data":null}` |
| token 不对或没带 | 401 | `{"code":1,"msg":"invalid channel token"}` |
| 撞到限流 | 429 | `{"code":1,"msg":"…","data":{"error_code":"…"}}` |

所以在脚本里 `curl -f` 是不够的：

```bash
curl -sS -d "磁盘 92%" https://push.example.com/api/v1/send/<token> \
  | grep -q '"code":0' || echo "发送失败" >&2
```

token 不存在和已停用给的是同一句话，这是刻意的——不让调用方用错误信息探测哪些 token 存在。

### token 放在哪

路径是最好读的一种写法，此外也收 `Authorization: Bearer <token>`、`token` 请求头、
`?token=`、以及表单里的 `token` 字段，调用方哪种顺手用哪种。
`POST /api/v1/send`（路径里不带 token）就是为这几种准备的。

## 消息字段

除了「总得有点内容」——标题、正文、附件至少有一样——其余字段都可选。

| 字段 | 类型 | 作用 |
|---|---|---|
| `title` | string | 通知标题。 |
| `body` | string | 正文。`text` 是它的别名。 |
| `type` | string | app 按哪种方式渲染正文：`text`（默认）、`markdown`、`image`、`file`、`link`、`card`。 |
| `summary` | string | 覆盖自动生成的推送摘要。不填就从正文生成。 |
| `link` | string | 消息要打开的地址。 |
| `copy` | string | 给人复制的一段内容——验证码、地址、一条命令。 |
| `file` | string | 先前上传得到的 `uid`，见[附件](#附件)。 |
| `items` | array | 卡片的行：`[{"k":"branch","v":"main","style":"ok"}]`。`style` 取 `ok`、`warn`、`error`、`muted`，按给的顺序显示在正文下方。 |
| `collapse_id` | string | 同一个 id 的消息在锁屏上互相替换，而不是堆叠。 |
| `idem_key` | string | 同一个键发第二次，返回第一条并带 `"dedup":true`，不会多出一条。重试循环因此是安全的。 |
| `reply` | object | 要一个答复，见[要一个答复](#要一个答复)。 |

**没有 `sound` / `level` / `priority` 字段**，这是刻意的，原因见[开始之前](#开始之前)。

推送摘要从正文生成，而且是**按「给人读」生成的**：代码块整块跳过，表格的竖线换成间隔号，
分割线丢掉。落在锁屏上的是一句话，不是一段源码——这很重要，因为客户端的通知扩展
**可能被系统跳过**，跳过时用户能看到的就只有它。

## 附件

上面那条 multipart 是一步到位的写法。同一张图要发给多个频道时，先传一次：

```bash
curl -F "file=@chart.png" -H "Authorization: Bearer <token>" \
  https://push.example.com/api/v1/upload
# {"code":0,"msg":"success","data":{"uid":"…","name":"chart.png","mime":"image/png",
#   "size":48213,"width":1600,"height":900,"url":"https://…","thumb":"https://…"}}
curl -H 'Content-Type: application/json' \
  -d '{"type":"image","title":"磁盘告警","file":"<uid>"}' \
  https://push.example.com/api/v1/send/<token>
```

一步到位那种写法里，文件字段叫 `image` 或 `file` 都行；`/api/v1/upload` 上是 `file`。

附件按内容寻址：同样的字节传两次只存一份，最后一条引用它的消息没了才删 blob。

上限来自 `[storage]`：`max_upload_mb`（默认 20）、`image_max_px`（1600）、
`thumb_max_px`（600）。图片会被重新编码到这个范围内——通知扩展只有 24MB 内存预算，
一张原图放不进去。

下载地址带签名，`storage.url_ttl`（默认 7 天）后失效，且不认登录态：
通知扩展要在手机首次解锁前就把图拉下来，那时它未必读得到 Keychain。

## 要一个答复

消息可以要一个答复：点一个选项、勾几个再提交、拖滑块选一个数，或者打一行字。
人点开通知，在 app 里回一下，服务端把答复 POST 到你给的地址。

```bash
curl -H 'Content-Type: application/json' -d '{
  "title": "窗帘 30 秒后自动打开",
  "reply": {
    "type": "choice",
    "options": ["打开", "不要打开"],
    "timeout": 30,
    "webhook": "https://home.example.com/api/webhook/curtain"
  }
}' https://push.example.com/api/v1/send/<token>
```

| `reply` 字段 | 用于 | 含义 |
|---|---|---|
| `type` | 全部 | `choice`、`multi`、`number`、`text`。不填时：给了 `options` 就是 `choice`，否则是 `text`。 |
| `options` | `choice` `multi` | 最多 10 个，每个最长 40 字。按给的顺序显示。 |
| `webhook` | 全部 | **必填。** 答复往哪里 POST。 |
| `timeout` | 全部 | 多少秒后不能再回。0 或不填表示一直可以回，最长 30 天。 |
| `min` `max` `step` | `number` | 滑块的范围，`min` 可以是 0。 |
| `unit` | `number` | 只用于显示（`°C`、`%`、`分钟`），服务端不解释它。 |

`timeout` 不填就一直可以回，只有「不回就会自动发生别的事」才需要设它。
agent 问一句等回答那种没有默认分支，设了反而会让答案白白作废。

**必须给 `webhook`**——没有地址的答复无处可去，与其让人白点一下，不如发送时就报错。

人回完之后，你的地址收到：

```json
{"uid":"01J…","channel":"…","title":"窗帘 30 秒后自动打开","reply":"不要打开","replied_at":1726000000}
```

`reply` 的类型随形态变：`multi` 是字符串数组，`number` 是数字，其余是字符串。

一条消息只回一次，先到先得；多台设备之间靠同步看到同一个答案。
答复**不会**从发送接口回来——那条连接在消息存下的那一刻就结束了，而人可能几分钟后才看手机。

### 校验回调

每个回调都带 `X-Knockbox-Signature: sha256=<HMAC-SHA256(请求体, 频道 token)>`。
动手之前先验它，而且要用常数时间比较：

```python
import hmac, hashlib

def verify(raw_body: bytes, header: str, token: str) -> bool:
    expected = "sha256=" + hmac.new(token.encode(), raw_body, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, header or "")
```

签的是**原始字节**，要在任何 JSON 解析之前取——重新序列化就变了。

投递最多重试 `reply.webhook_max_attempts` 次（默认 3），退避 5s / 30s / 2m，
所以你的地址要容忍同一个答复到达两次。`uid` 在重试之间不变，适合当去重键。

### 平铺写法

`curl` 一行也能问：

```bash
curl "https://push.example.com/api/v1/send/<token>?title=窗帘&choices=打开,不要打开&reply_timeout=30&reply_webhook=https://…"
```

### 回调打到内网地址

公共实例默认不允许把回调发到内网地址（这是 SSRF 面）；自建实例默认允许，
因为服务器就在你自己的网里，要够得着的正是那些地址。开关是 `reply.public_allow_private`。

## 一次发给多个频道

```bash
curl -H 'Content-Type: application/json' -d '{
  "tokens": ["<token-a>", "<token-b>"],
  "title": "部署完成"
}' https://push.example.com/api/v1/send/batch
```

**逐条**报告结果，一个坏 token 不会把整批打回——响应里有 `results`、`ok`、`failed`。
上游一次调用就能打到很多人，而且不需要一把「能写所有频道」的广播凭据。

单次最多 200 个频道。

## 接给 AI agent

同一个发送凭据也可以当 [MCP](https://modelcontextprotocol.io) 端点用（streamable HTTP），
只暴露一件工具：发一条通知。

```bash
claude mcp add --transport http knockbox https://push.example.com/mcp/<token>
```

用配置文件的客户端填这一段：

```json
{
  "mcpServers": {
    "knockbox": {
      "type": "http",
      "url": "https://push.example.com/mcp/<token>"
    }
  }
}
```

自己配不了的助手，可以直接把「加一个 MCP 服务器，Streamable HTTP，地址是……」
这句话连地址发给它。这三种写法在 `/docs` 和频道的发送页上都有，地址已经填好，复制即可。

它和 `/api/v1/send` 是同一条路，权限、速率限制、配额完全一样，能执行命令的 agent
改用它并不多拿到什么。它换来的是三件事：执行不了命令的客户端也能接；带换行和引号的正文
不必过 shell 转义；工具常驻在 agent 的工具表里，不用先粘一段提示词告诉它有这么个地址。

## 限流与配额

这是两件事，而且同时生效。

**限流**是防滥用，**在哪种模式下都生效**，自建也不例外：每个频道 token 每秒
`limit.send_qps` 条、可攒 `limit.send_burst` 条（默认 20 和 60）。超了返回 `429`。
泄露的 token 刷不爆数据库。

**配额**只有公共实例才有：`public.max_channels` 是每个成员的频道数上限，
`public.max_messages_per_day` 是滚动 24 小时内的条数。自建实例没有任何配额。

`limit.body_max_kb`（默认 1 MB）超了是**拒绝，不是截断**——悄悄丢掉消息的后半段，
比报一个发送方看得见的错更糟。

## 命令行

全部是子命令、都有 `-h`；**任何操作都不需要直接改数据库**。

```
knockbox serve                 启动服务
knockbox user add <name>       建管理员账号（交互式输入密码）
knockbox user passwd <name>    重设密码
knockbox user list             列出账号
knockbox pair                  签发配对码并在终端打出二维码
```

密码默认交互式隐藏输入，也收管道（`echo 's3cret' | knockbox user passwd admin`）。
用命令行参数传也可以，但那会短暂出现在 `ps` 里。

## 什么是公开契约

`/api/v1` 是：发送，以及 iOS 客户端用的那几个。它们写在这份文档里，不会无故改形状。

`/admin/api/` 下的一切**不是**。它只服务本仓库自带的管理后台，与它共用会话 cookie，
后台改版它就跟着改。不要基于它开发——要用脚本操作这台服务器，请走 `/api/v1` 或命令行。

## 接口清单

不需要鉴权：

| 方法 | 路径 | 作用 |
|---|---|---|
| `GET` | `/api/v1/server` | 服务器名、版本、能力清单。配对前先调它，能分清「地址填错」和「配对码错」。 |
| `POST` | `/api/v1/pair` | 用配对码换设备 token。按 IP 限流。 |
| `GET` | `/api/v1/file/:uid` | 下载附件。只认签名，不认登录态。 |
| `GET` | `/health` | 存活检查，含数据库连通性。 |

频道 token —— 只写，只作用于一个频道：

| 方法 | 路径 | 作用 |
|---|---|---|
| `POST` | `/api/v1/send` | 发送，token 走请求头、query 或表单。 |
| `POST` `GET` | `/api/v1/send/:token` | 发送，token 在路径里。 |
| `POST` | `/api/v1/send/batch` | 发给多个频道，逐条报结果。 |
| `POST` | `/api/v1/upload` | 上传附件。 |
| — | `/mcp/:token` | MCP 端点，streamable HTTP。 |

设备 token —— iOS 客户端用的那套。列在这里是为了让你知道客户端能做什么，
不是邀请你再写一个客户端：

| 方法 | 路径 |
|---|---|
| `POST` | `/api/v1/device/push-token` |
| `POST` | `/api/v1/pair/issue` |
| `GET` `DELETE` | `/api/v1/devices`、`/api/v1/devices/:uuid` |
| `GET` `POST` `PATCH` `DELETE` | `/api/v1/channels`、`/api/v1/channels/:id` |
| `POST` | `/api/v1/channels/:id/rotate`、`/api/v1/channels/:id/purge` |
| `GET` | `/api/v1/sync`、`/api/v1/messages` |
| `POST` | `/api/v1/messages/read`、`/api/v1/messages/delete`、`/api/v1/messages/:uid/reply` |
