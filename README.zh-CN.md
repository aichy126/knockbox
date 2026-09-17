# Knockbox

[English](README.md) · **简体中文**

[![CI](https://github.com/aichy126/knockbox/actions/workflows/ci.yml/badge.svg)](https://github.com/aichy126/knockbox/actions/workflows/ci.yml)
[![Licence: MIT](https://img.shields.io/badge/licence-MIT-blue.svg)](LICENSE)

自建 iOS 消息推送服务端。一条 `curl` 发出去，手机上就收到。

Knockbox 是一个单二进制 + 单 SQLite 文件的自建推送服务端。它存消息、直接推给 Apple
的服务器、把历史交给 [iOS 客户端](https://apps.apple.com/app/id6811317906)。
中间没有任何中继，也不需要注册账号——服务器是你自己的。

### 快速开始

```bash
docker run -d --name knockbox -p 8080:8080 -v knockbox-data:/app/data \
  -e IGO_SERVER_EXTERNAL_URL=http://localhost:8080 \
  ghcr.io/aichy126/knockbox
docker logs knockbox
```

第一次启动会建好数据库和一个管理员账号，并把这个账号的用户名和密码打印在日志里
——只打印这一次。打开 <http://localhost:8080/login> 用它登录，配对设备页会出一个二维码，
用 iOS 客户端扫一下就接上了。密码在同一个后台的设置页里可以改。

**`IGO_SERVER_EXTERNAL_URL` 要填手机真正够得着的地址**，例如
`https://push.example.com`。配对链接和附件链接都按它生成，填 `localhost`
只在同一台机器上试的时候有效。

镜像同时发到 `ghcr.io/aichy126/knockbox` 和 Docker Hub 的 `aichy66/knockbox`，
有 `linux/amd64` 和 `linux/arm64` 两种架构。打过 tag 的版本另外会发预编译二进制；
不想用容器就直接编：`make build`（需要 Go 1.26+）。

然后发一条：

```bash
curl -d "构建失败" https://push.example.com/api/v1/send/<频道 token>
```

### 发送

收四种 Content-Type，标量参数也一律能从 query string 取，所以 `curl` 怎么写都行：

```bash
# 整个 body 就是正文
curl -d "磁盘 92%" .../api/v1/send/<token>
# 纯 GET，给不方便发 POST 的场景
curl ".../api/v1/send/<token>?title=CI&text=构建失败"
# 带图
curl -F "title=磁盘告警" -F "text=/data 92%" -F "image=@chart.png" .../api/v1/send/<token>
# 完整形态
curl -H 'Content-Type: application/json' -d '{
  "type": "markdown",
  "title": "Build #1832 failed",
  "body": "**main** · 42s\n\n```go\nfunc Execute() {}\n```",
  "idem_key": "drone-build-1832"
}' .../api/v1/send/<token>
```

`POST /api/v1/send/batch` 收一组 token 并**逐条**报告结果——一个坏 token 不会把整批打回，
上游一次调用就能打到很多人，而且不需要一把「能写所有频道」的广播凭据。

### 要一个答复

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
}' .../api/v1/send/<token>
```

点了「不要打开」之后，你的地址收到：

```json
{"uid":"01J…","channel":"…","title":"窗帘 30 秒后自动打开","reply":"不要打开","replied_at":1726…}
```

请求头带 `X-Knockbox-Signature: sha256=<HMAC-SHA256(请求体, 频道 token)>`，可以用它验来源。
另外三种：`multi` 勾几个再提交，`number` 是一个范围（`min` `max`，可选 `step` 与 `unit`），
`text` 出输入框。回调里 `reply` 的类型随形态变——`multi` 是字符串数组，`number` 是数字，其余是字符串。
`timeout` 不填就一直可以回，只有「不回就会自动发生别的事」才需要设它。**必须给 `webhook`** —— 没有地址的答复无处可去，与其让人白点一下，不如发送时就报错。

平铺写法同样支持，`curl` 一行也能用：

```bash
curl ".../api/v1/send/<token>?title=窗帘&choices=打开,不要打开&reply_timeout=30&reply_webhook=https://…"
```

一条消息只回一次，先到先得；多台设备之间靠同步看到同一个答案。
答复【不会】从发送接口回来 —— 那条连接在消息存下的那一刻就结束了，
而人可能几分钟后才看手机。

### 接给 AI agent

同一个发送凭据也可以当 [MCP](https://modelcontextprotocol.io) 端点用（streamable HTTP），
只暴露一件工具：发一条通知。

```bash
claude mcp add --transport http knockbox https://你的服务器/mcp/<token>
```

用配置文件的客户端填这一段：

```json
{
  "mcpServers": {
    "knockbox": {
      "type": "http",
      "url": "https://你的服务器/mcp/<token>"
    }
  }
}
```

自己配不了的助手，可以直接把「加一个 MCP 服务器，Streamable HTTP，地址是……」这句话连地址发给它。
这三种写法在 `/docs` 和频道的发送页上都有，地址已经填好，复制即可。

它和 `/api/v1/send` 是同一条路，权限、速率限制、配额完全一样，能执行命令的 agent 改用它
并不多拿到什么。它换来的是三件事：执行不了命令的客户端也能接；带换行和引号的正文不必过
shell 转义；工具常驻在 agent 的工具表里，不用先粘一段提示词告诉它有这么个地址。

### 通知是怎么响的

频道相当于群机器人的收件线。它的**铃声、打扰级别、静音存在服务端**，因为这三样要在推送那一刻
写进 APNs payload——客户端的通知扩展**可能被系统跳过**，跳过时手机用的就是 payload 里的原值。
频道的其余属性（名字、图标、颜色）对服务端是一个不透明 blob，只存不读。

**正文不进 payload。** payload 只带摘要和消息 id，正文留在库里由 app 去取。所以正文可以远超一条通知
装得下的长度（上限由 `limit.body_max_kb` 决定，默认 1 MB），而且推送永远发得出去——
payload 装不下时会逐级降级，直到塞得进 4KB。

公共实例默认不允许把回调发到内网地址（这是 SSRF 面）；自建实例默认允许，
因为服务器就在你自己的网里，要够得着的正是那些地址。见 `config.toml` 的 `[reply]`。

### 你应该知道的几个取舍

这些是选择，不是遗漏。部署前请读完。

**不做端到端加密。** 没有中继，消息只经过你自己的服务器；而服务端还必须存历史、生成推送摘要，
这两件事都要求能读明文。自持服务器的前提下，E2E 主要防的是服务器管理员——那就是你自己。

**服务端长期持有全部明文，默认永久保留。** 完整归档是刻意的：新设备要能拉到全部历史。
要设上限就配 `retention.days`（实例级，对所有人生效）；公共实例还可以用
`public.retention_days` 单独约束每个成员，两者都设时按更严的算。
**安全边界就是那台机器**，备份和加固请照此对待。

**仓库里内置了一把公开的 APNs 密钥。** APNs 的 device token 绑定在 bundle id 上，
只有该 app 所属 Team 的密钥才推得动 App Store 上那个客户端。为了让你不必花 $99/年
也能自建，这里放了一把 **topic-specific** 密钥——它只能推 `com.miramiao.knockbox`，推不了别的。

要诚实说明的代价：任何**同时**拿到某台设备 APNs token 的人，都能给那台设备推任意内容。
这些 token 明文躺在设备上和你自己的数据库里，所以现实中的路径是数据库泄露——
而密钥既然已经公开，这条路上就没有别的关卡了。这确实是一次降级。
两条出路：

- 把 `apns.production.key_file` 指向你自己的密钥并改 `apns.topic`，然后自己编译签名 iOS 客户端。
- 如果这把共享密钥被大规模滥用、Apple 对该 Team 限流，它会被轮换，
  **届时所有自建实例都要升级才能恢复。** 轮换会同时发在本仓库的 GitHub Security
  Advisory 和携带新密钥那个版本的 release notes 上；如果你在为别人运行它，
  请点 **Watch → Custom → Releases and Security advisories** 订阅。

这把密钥能碰到什么、碰不到什么，以及怎么报告问题，见
[SECURITY.zh-CN.md](SECURITY.zh-CN.md)。

**管理后台只有中文。** 陌生人会走到的那几页——接入页、发送说明页，以及它们的
错误落地页——是中英双语且默认英文；`/login` 与 `/admin/*` 不是，它们给运行这台
服务器的人看，而维护者用中文 review。后台里没有独占的能力：每一件事在命令行和
HTTP 接口上都做得到，那两处是英文。

**频道 token 明文存库。** 因为产品要求「在 app 里随时复制 curl」，哈希存储做不到这件事。
它是只写凭据、只作用于一个频道，`last_used_at` / `ip` 会留痕，频道随时可以轮换或删掉。
每个 token 还受发送速率限制（`limit.send_qps` / `send_burst`，默认每秒 20 条、可攒 60 条），
所以泄露的 token 刷不爆数据库——**这条限制在自建模式下同样生效**，那里本来没有任何配额。

### 删除是真的删除

清空频道走的是物理删除。SQLite 的 `DELETE` 只标记页空闲、内容仍留在文件里直到被覆盖，
所以服务端开着 `secure_delete`，删完还会 `wal_checkpoint(TRUNCATE)` 加 `incremental_vacuum`。
附件按引用计数管理，最后一条引用它的消息没了才删 blob。

**不承诺的部分**：`os.Remove` 之后，SSD 上的物理块因为损耗均衡可能仍然存在——那不是应用层能保证的事。

### 命令行

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

### 配置

复制 `config.toml.example` 改。每个配置项都能用环境变量覆盖
（`server.external_url` → `IGO_SERVER_EXTERNAL_URL`），密钥这类东西就该这么传，
比如 `IGO_APNS_PRODUCTION_KEY_BASE64`。

### 许可

MIT，见 [LICENSE](LICENSE)。
