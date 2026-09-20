<p align="center">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/icon-256.png" width="104" alt="">
</p>

<h1 align="center">Knockbox</h1>

<p align="center">
  自建 iOS 消息推送服务端。<br>
  一条 <code>curl</code> 发出去，手机上就收到 —— 还能让人回一句。
</p>

<p align="center">
  <a href="README.md">English</a> · <b>简体中文</b>
</p>

<p align="center">
  <a href="https://github.com/aichy126/knockbox/actions/workflows/ci.yml"><img src="https://github.com/aichy126/knockbox/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/aichy126/knockbox/releases"><img src="https://img.shields.io/github/v/release/aichy126/knockbox?color=4F63EE" alt="最新版本"></a>
  <a href="https://github.com/aichy126/knockbox/pkgs/container/knockbox"><img src="https://img.shields.io/badge/ghcr.io-aichy126%2Fknockbox-4F63EE?logo=github" alt="GitHub Container Registry"></a>
  <a href="https://hub.docker.com/r/aichy66/knockbox"><img src="https://img.shields.io/docker/pulls/aichy66/knockbox?logo=docker&color=4F63EE" alt="Docker Hub"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/licence-MIT-blue.svg" alt="许可：MIT"></a>
</p>

<p align="center">
  <a href="https://apps.apple.com/app/id6811317906"><img src="https://aichy126.github.io/miramiao/knockbox/images/badge-appstore-zh.svg" height="44" alt="从 App Store 下载"></a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://aichy126.github.io/miramiao/knockbox/images/arch-zh-dark.svg">
    <img src="https://aichy126.github.io/miramiao/knockbox/images/arch-zh-light.svg" width="880" alt="你的脚本一条 curl 发到自己的服务器，由它直接推给 Apple 的 APNs，再到你的 iPhone。中间没有中继。">
  </picture>
</p>

Knockbox 是一个单二进制 + 单 SQLite 文件的自建推送服务端。它存消息、直接推给 Apple
的服务器、把历史交给 iOS 客户端。中间没有任何中继，也不需要注册账号——服务器是你自己的。

<p align="center">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/shot-zh-channels.png" width="252" alt="频道列表，一件事一条线">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/shot-zh-markdown.png" width="252" alt="一条带表格、代码和图片的消息">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/shot-zh-selfhost.png" width="252" alt="服务器是你自己的，推送密钥也是">
</p>

## 先花一分钟试试

公共实例在 <https://knockbox.miramiao.com/>。装好 app 后用手机打开它，就有了自己的收件箱：
不用注册，也不用密码。它跑的是下面这个镜像，带配额，消息保留 30 天。

想把消息留在自己的机器上，就照下面自己搭一个。

## 自己搭一个

```bash
docker run -d --name knockbox -p 8080:8080 -v knockbox-data:/app/data \
  -e IGO_SERVER_EXTERNAL_URL=http://localhost:8080 \
  ghcr.io/aichy126/knockbox
docker logs knockbox
```

第一次启动会建好数据库和一个管理员账号，并把这个账号的用户名和密码打印在日志里
——只打印这一次。打开 <http://localhost:8080/login> 用它登录，配对设备页会出一个二维码，
用 iOS 客户端扫一下就接上了。

**`IGO_SERVER_EXTERNAL_URL` 要填手机真正够得着的地址**，例如 `https://push.example.com`。
配对链接和附件链接都按它生成，填 `localhost` 只在同一台机器上试的时候有效。

然后发一条：

```bash
curl -d "构建失败" https://push.example.com/api/v1/send/<频道 token>
```

长期跑的话用仓库里的 [`compose.yaml`](compose.yaml)；每个打过 tag 的版本还会发
Linux、macOS、Windows、FreeBSD 的预编译二进制。**不需要数据库服务，也不需要 Redis**
——整个服务就是一个二进制加一个 `.db` 文件。
→ [部署与运维](docs/deploy.zh-CN.md)

## 它能做什么

| | |
|---|---|
| **怎么写都能发** | 整个 body 当正文、query string、multipart、JSON 四种都收，标量参数一律也能从 query 取，一行 `curl` 就够。[→](docs/api.zh-CN.md#发送) |
| **正文没有长度上限** | 正文不进推送 payload，所以能远超一条通知装得下的长度。Markdown、表格、代码、图片。[→](docs/api.zh-CN.md#消息字段) |
| **可以要一个答复** | 点一个选项、勾几个、拖滑块、或者打一行字。答复由服务端 POST 到你给的地址，带签名。[→](docs/api.zh-CN.md#要一个答复) |
| **一次调用发给很多人** | `send/batch` 收一组 token 并逐条报结果，不需要一把能写所有频道的广播凭据。[→](docs/api.zh-CN.md#一次发给多个频道) |
| **给 agent 的接入面** | 同一个凭据也是 MCP 端点（streamable HTTP），只暴露一件工具。[→](docs/api.zh-CN.md#接给-ai-agent) |
| **一件事一个频道** | 铃声、打扰级别、静音都存在服务端，脚本不必判断什么算急。[→](docs/api.zh-CN.md#开始之前) |
| **每台设备都有全部历史** | 新设备扫一下码就接上，然后把历史全部拉下来。 |

## 它自带页面

服务端在 `/docs` 上有一份发送说明，中英双语，地址已经填好，每个例子都带「发这条」按钮。
公共实例上的那份在 <https://knockbox.miramiao.com/docs>。

<p align="center">
  <img src="https://aichy126.github.io/miramiao/knockbox/images/web-docs-zh.png" width="760" alt="自带的发送说明页，给出发送地址和可直接运行的 curl 示例">
</p>

## 文档

| | |
|---|---|
| [**部署与运维**](docs/deploy.zh-CN.md) | Compose、Docker、二进制、源码 · 反向代理 · 升级 · 备份 · 全部配置项 |
| [**发送与集成**](docs/api.zh-CN.md) | 字段 · 附件 · 答复与回调 · 批量 · MCP · 限流 · 命令行 |
| [**安全**](SECURITY.zh-CN.md) | 内置的那把 APNs 密钥能碰到什么、碰不到什么，以及怎么报告问题 |
| [**参与开发**](CONTRIBUTING.zh-CN.md) | 怎么编、怎么测、怎么提一个改动 |

## 你应该知道的几个取舍

这些是选择，不是遗漏。部署前请读完。

- **不做端到端加密。** 没有中继，消息只经过你自己的服务器；而服务端还必须存历史、生成
  推送摘要，这两件事都要求能读明文。自持服务器的前提下，E2E 主要防的是服务器管理员。
- **服务端长期持有全部明文，默认永久保留。** 完整归档是刻意的：新设备要能拉到全部历史。
  要设上限就配 `retention.days`。**安全边界就是那台机器。**
  [→](docs/deploy.zh-CN.md#备份)
- **仓库里内置了一把公开的 APNs 密钥**，为的是让你不花 $99/年也能自建。它是
  topic-specific 的——只推得动这一个 app——但它是公开的，要诚实说明的代价是：
  任何**同时**拿到某台设备 APNs token 的人，都能给那台设备推任意内容。
  换成你自己的密钥就没有这条路。[→](docs/deploy.zh-CN.md#用你自己的-apns-密钥) ·
  [SECURITY.zh-CN.md](SECURITY.zh-CN.md)
- **频道 token 明文存库。** 因为产品要求「在 app 里随时复制 curl」，哈希存储做不到这件事。
  它是只写凭据、只作用于一个频道、受速率限制，且随时可以轮换。
- **管理后台只有中文。** 陌生人会走到的那几页——接入页、发送说明页，以及它们的错误落地页
  ——是中英双语且默认英文；`/login` 与 `/admin/*` 不是。后台里没有独占的能力：每一件事在
  命令行和 HTTP 接口上都做得到，那两处是英文。
- **`/admin/api/*` 不是公开契约。** `/api/v1` 才是。`/admin/api/` 下的一切只服务本仓库
  自带的后台，后台改版它就跟着改。[→](docs/api.zh-CN.md#什么是公开契约)

**删除是真的删除。** 清空频道走的是物理删除：服务端开着 `secure_delete`，删完还会
checkpoint WAL 并做增量 vacuum。附件按引用计数管理，最后一条引用它的消息没了才删 blob。
不承诺的部分：`os.Remove` 之后 SSD 上的物理块因为损耗均衡可能仍然存在，那不是应用层能保证的事。

## 许可

MIT，见 [LICENSE](LICENSE)。
