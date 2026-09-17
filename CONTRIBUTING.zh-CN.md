# 参与贡献

[English](CONTRIBUTING.md) · **简体中文**

感谢你看到这里。这是一个人维护的小项目，下面这些是为了让补丁不要卡在本可避免的地方。

发现安全问题请不要开 issue，见 [SECURITY.zh-CN.md](SECURITY.zh-CN.md)。

## 这个项目做什么

Knockbox 只负责消息：存储、推送、历史、附件。谁该收哪条消息是上游系统的事——
这里没有「订阅」这个概念，加一个也不在范围内。

大多数决定受两条约束：

- **一个二进制、一个 SQLite 文件。** 不引 Redis，不引消息队列，不依赖外部服务。
  自建的人应该只用一条 `docker run` 就能跑起来。
- **除了 Apple 不连任何外网。** 服务端只和 APNs 通信。

会破坏这两条的改动，请先开 issue 讨论。

## 跑起来

需要 Go 1.26 或更高。

```bash
cp config.toml.example config.toml   # 至少要改 server.external_url
make build
make run                             # 第一次启动会自动建管理员，密码打在日志里
./knockbox pair                      # 在终端打出二维码给 iOS 客户端扫
```

`config.toml` 在 gitignore 里。里面每一个键都在 `config.toml.example` 有说明；
新增键的话，那边也要补上说明，并且确认代码真的会读它。

## 提 PR 之前

```bash
make lint    # go vet、go mod tidy -diff、gofmt、golangci-lint
make test    # go test ./... -race -cover
```

CI 跑同样的检查，另外还有一次 `CGO_ENABLED=0` 构建，以及启动镜像等 `/health`
的冒烟测试。SQLite 用纯 Go 驱动就是为了保住这个前提：发布产物是静态二进制、
镜像基于 alpine。

## 测试

修 bug 请带一个「不打这个补丁就会失败」的测试。这不是形式要求——
这个代码库里被找出来的几个 bug，从外面完全看不出来：引用计数从来没减过、
队列里的记录永远停在待推、正文在落库路上被静默截断。
这类问题只有在测试直接断言那条不变量时才会暴露。

`make cover` 会打印总覆盖率并打开 HTML 报告。值得提的是承载逻辑的那几个包
（`internal/service`、`internal/dao`、`internal/library/*`）；
`internal/api/web` 里拼 HTML 的部分不值得。

## 约定

**注释用中文写。** 这是刻意的——维护者用中文做 review。
issue 和 PR 用中文或英文都可以。请不要在无关的改动里顺手翻译已有注释。

**注释写「为什么」，不写「是什么」。** 不写开发过程、不写日期、不写人名、
不和别的项目做比较。「引用计数必须和插入同一个事务」是有用的；
「原来放在事务外面，11 号修了」没有用——读的人要的是此刻成立的约束，
其余的 git 里都有。

**改表结构就加一个新的迁移。** DDL 只存在于
`internal/migrate/sql/NNNN_名字.sql`；`internal/models` 里的结构体只负责列映射。
已经发布过的迁移永远不要改，往后加一个编号。

**SQLite 的 pragma 串只有一份定义。** 就是 `migrate.DSN`，
`migrate.bootstrap()` 会在启动时硬断言其中关键的几条。不要在测试里手抄一份。

**错误要包一层，但绝不吞掉。** 用户看到的是发生了什么、他能做什么，一句话，
不带类型名和堆栈；原始错误原样进日志。两半都要有：
把 `typeMismatch(Swift.Int64, …)` 弹给用户没有意义，
而「操作失败」同样让人无从排查。

**界面上的每一句话都按对外产品的标准写。** 动笔前先问这句有没有必要存在；
要写就说清「这是什么、该做什么、会发生什么」。不写俏皮话、不写自嘲、
不写实现细节。语气是克制的、陈述性的。

**每一个配置键都必须被代码读取。** 只存在于示例文件里的键比没有这个键更糟——
总会有人去改它，然后困惑为什么没反应。

**依赖。** 必须保持能在 CGO_ENABLED=0 下构建。新增模块请在 PR 描述里说明理由；
三十行的小工具函数通常好过一个新 import。

**推送器只能单实例运行。** `claim` 从 `push_log` 取记录时没有加租约，
同一个库上跑两个进程会把每条消息推两遍。要横向扩展得先加租约列，
那是设计变更，值得先开 issue。

## 加一门语言

陌生人会走到的那几页——接入页、发送说明页，以及它们的错误落地页——是有翻译的。
加一门语言就是**加一个文件**：

1. 把 `internal/api/web/locales/en.json` 复制成 `<代码>.json`，`<代码>` 是语言子标签
   （`ja`、`de`、`pt-br`）。文件名【就是】那门语言，别处没有一张需要同步登记的清单。
2. 翻译里面的值。`lang_name` 写这门语言自己的名字（`日本語` 而不是 `Japanese`）——
   语言开关上显示的就是它。`html_lang` 只在与文件名不同时才写（`zh` → `zh-CN`）。
3. `%s` 和 `%d` 占位符要留着，顺序也不能换。

没翻的 key 会回落成英文，所以翻一半也值得提。而 `en.json` 里【没有】的 key 会让测试
失败——那种情况一定是拼错了，或者某个 key 改了名而译文没跟上。

管理界面（`/login`、`/admin/*`）不在这套体系里，原因见 README 的取舍那一节。

## 提交信息

用 conventional commit 前缀，和现有历史一致：`feat:`、`fix:`、`docs:`、
`perf:`、`chore:`、`style:`。发版的 changelog 会过滤掉 `docs:`、`test:`、`chore:`，
所以用户应该看到的内容请放在其余几类下。

写清楚改了什么、以及原来为什么是错的。一段讲明白故障表现的正文，
比一份文件清单有价值得多。

## 不要提交

`config.toml`、`data/`、`logs/`，以及任何 `.p8` 文件。唯一的例外是
`internal/library/apns/embedded/` 下那把已经入库的公开密钥，
为什么放在那里见 [SECURITY.zh-CN.md](SECURITY.zh-CN.md)。
