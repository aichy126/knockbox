# Knockbox 服务端 · 给 AI 助手的项目说明

自建 iOS 消息推送服务端。Go + igo + SQLite，一个二进制一个库文件，直连 APNs 不走中继。
配套 iOS 客户端在另一个仓库。

**先读这三份，不要在这里重复它们的内容**：
[README.zh-CN.md](README.zh-CN.md)（做什么、取舍）、
[CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md)（约定、提交规范）、
[SECURITY.zh-CN.md](SECURITY.zh-CN.md)（内置 APNs 密钥、服务端持有什么）。
本文只写「动手改代码之前必须知道、而读单个文件看不出来」的东西。

> ⚠️ **这个仓库会开源。** 注释、文档、提交信息里不要出现内部项目名、人名、
> 内网地址、公司内部推演。已经清理过一轮，不要再带进来。

## 常用命令

```bash
make build      # 出二进制
make test       # go test ./... -race -cover
make lint       # go vet + go mod tidy -diff + gofmt + golangci-lint
make cover      # 覆盖率总数 + HTML 报告
make run        # 启动服务（= ./knockbox serve -c config.toml）
```

提交前 `make lint && make test` 必须都过。CI 跑同样的检查，另加
`CGO_ENABLED=0 go build`（发布产物是静态二进制）和一次 Docker 冒烟测试。

首次跑起来：`cp config.toml.example config.toml` → 改 `server.external_url` →
`make run`。库里一个账号都没有时，启动会自己建出管理员并把凭据打印一次
（`service.EnsureFirstAdmin`，非程序员不该被要求先去敲一条建号命令），
拿它登录 `/login`；配对二维码在后台「配对设备」页，`./knockbox pair` 也能在终端出一个。

## 分层

```
main.go → internal/cli/        cobra 子命令，装配依赖（serve.go 是唯一读配置的地方）
          internal/api/        HTTP 处理函数；router.go 是唯一的路由注册处
          internal/api/mcp.go  MCP（Streamable HTTP）接入面，与 /send 同权同路
          internal/api/web/    服务端直出 HTML，无前端构建步骤
          internal/middleware/ 三套鉴权 + 限流
          internal/service/    业务编排，事务边界在这一层
          internal/dao/        只做 SQL，不含业务判断
          internal/models/     列映射 + 常量，不含 DDL
          internal/migrate/    版本化迁移，DDL 的唯一来源
          internal/library/    apns / idgen / urlsign，不依赖上面任何一层
```

约 10500 行非测试代码，40 个测试文件，总覆盖率约 40%。
分布是刻意不均的：`urlsign` 100%、`idgen` 95%、`dao` 81%、`migrate` 74%、
`service` 63%，而 `api/web` 那堆拼 HTML 的是 0%。
要提覆盖率，提承载逻辑的那几个包，不要给字符串拼接补测试。

**三套鉴权的边界是刻意分开的，别合并**（见 `middleware/auth.go` 的包注释）：
`SendAuth` 频道 token 只写、`DeviceAuth` 设备 token 读本人全部、
`AdminAuth` 管理员会话看全站。

## 不能破坏的约定

这些都是**破坏之后不报错、只是悄悄不对**的地方。改到相关代码时先确认自己没有踩到。

**rev 是增量同步的唯一游标。** `dao.NextRev` 必须在调用方的事务里执行——
分配 rev 和用它写的那一行要么一起成功要么一起回滚。留下空洞的话，
游标越过去的客户端永远拉不回那一段，而且没有任何报错。
硬删消息之后必须写 `gc_watermark`，否则离线很久的设备会永久缺一块数据且毫无察觉。

**附件靠 `message.file_id` 定位。** 删除单条、清空频道、保留策略三条路全靠它。
写入端不填的话三条路一起静默空转，磁盘无声地涨。

**附件回收看 `ref_count` 和 `ever_referenced` 两列。** 引用降回 0 的立即回收；
从未被引用过的要过 `orphanGrace`（24 小时）——`/upload` 和随后的 `/send` 是两次请求，
没有宽限期的话中间撞上一轮 GC 就把 blob 扫掉了。
回收时**先删行再删文件**，反过来的话两步之间 `Store` 会按 sha256 命中那一行。

**清空是物理删除。** 依赖 DSN 里的 `secure_delete`，删完还要
`wal_checkpoint(TRUNCATE)` + `incremental_vacuum`。少了 checkpoint 等于没删，
WAL 里还留着旧页镜像，grep 数据库目录能把「删掉」的正文搜出来。

**DSN 的 pragma 串只有 `migrate.DSN` 一份定义**，`migrate.bootstrap()` 启动时硬断言。
igo 只在 DSN 里完全没有 `_pragma=` 时才自动补 busy_timeout 和 journal_mode，
写了任意一条就不再补其余的，而且是静默不补。测试里也引用这个常量，不要手抄。

**DDL 只在 `internal/migrate/sql/NNNN_名字.sql`。** 已应用的迁移永远不改内容
（改注释无害，因为已应用的会被跳过），要改就加下一个编号。
`internal/models` 的结构体只负责列映射。

**payload 的降级永远不能让推送发不出去。** 正文本来就在库里，payload 只是一张通知条；
丢内容好过发不出去。但最小骨架也要校验长度，压不进去就报错，
发一个 Apple 必然拒收的 payload 不比报错好。

**推送队列只能单实例。** `claim` 取记录时没有租约，靠「一个进程一条 Run 循环」
保证不重复投递。它对三张关联表都用 **LEFT JOIN** 而不是 INNER——
关联记录缺失的那一行仍要返回，才有机会置终态；用 INNER 的话它压根不出现在结果里，
于是永远停在 pending。

**config.toml 是首次启动的默认值，之后以 kv 表里的为准**（`service/settings.go`）。
规则是「某个键在 kv 里存过就用 kv 的，从没存过用配置文件的」。
读配置时注意 `GetInt` 分不出「键不存在」和「显式写了 0」，
需要区分时用 `GetIntWithDefault`（它内部走 `IsSet`）——发送限流就靠这个默认开启。

**每一个配置键都必须被代码读取。** 曾经有 12 个键只存在于示例文件里，
用户改了没反应。加键就要同步加读取，删功能就要同步删键。

**路由在启动时注册一次，而设置可以在后台改。** 按启动时的配置决定「挂不挂某条路由」
会出问题——公共模式在后台打开之后，接入页会去轮询一个并不存在的接口。
正确做法是路由常驻、在处理函数里做运行时判断。

**MCP 是发送接口的另一个接入面，不是第二条发送路径。** `api/mcp.go` 里那件工具必须照
`api/send.go` 的顺序走「限流 → 配额 → Deliver」，少一步它就成了绕过限流的后门。
工具的参数名不是手写的，来自 `service.SendInput` 的 JSON tag（整个参数对象 unmarshal 进去）——
改 tag 等于改 MCP 的参数名，schema 里的 `mcp.WithString("...")` 要跟着改，
否则 agent 按 schema 传的字段会被静默丢掉。`api/mcp_test.go` 盯着这两条。

**回复的三条不变量。** 回复是改一条【已有】的消息行，三件事必须在同一个事务里：
判定「还没回」、写入答案并 `dao.NextRev` 推高 rev、把回调排进 `reply_hook`。
少了 rev 那一步，别的设备永远同步不到「已回复」，界面上按钮一直亮着；
回调入队要是挪出事务，进程在两步之间挂掉就会出现「用户看到回复成功、发送方永远等不到」——
这是这个功能唯一不能出的错。过期判定必须在服务端，客户端那次只是为了不让用户白点。

**回复的任何东西都不进 APNs payload。** 选项、时限、形态一样都不下发：通知只负责把人叫进来，
回复在 app 内完成。客户端从 `/sync` 拿这些（`extra.reply` + `reply_until`），那里没有 4KB 的限制。
把它们塞回 payload 不会报错——客户端只是忽略，多出来的字节谁都看不见，
所以由 `TestPayloadNeverCarriesReply` 盯着。

**回调地址不下发给客户端。** `message.reply_webhook` 单独一列、不进 `extra`：
extra 是原样同步给每一台配对设备的，而回调地址是发送方的内部端点。
加字段到 `MessageView` 时留意别把它带上。

**公共实例的回调不许打内网。** 真正的边界在 `net.Dialer.Control`（拨号那一刻拿真实 IP 判），
不是发送时那次 `LookupIP`——后者能被 DNS rebinding 绕过，它存在只是为了让发送方早点知道。
同理 `CheckRedirect` 必须拒绝跟随：跟随的话一个 302 就绕过了前面所有检查。

**配额与限流出错时一律放行。** 它们是运营手段，不该因为自己查不出来就挡住消息。
同一个文件里不要一半放行一半收紧。

## 写代码的约定

细则在 CONTRIBUTING，这里只强调最容易被忽略的四条：

1. **注释用中文，写「为什么」不写「是什么」。** 不写开发过程、日期、人名、
   竞品比较。读的人要的是此刻成立的约束，其余的 git 里都有。
2. **错误包一层但绝不吞掉。** 用户看到「发生了什么 + 能做什么」，一句话；
   原始错误原样进日志。分类依据是「用户的下一步不同」，不是「异常类型不同」。
3. **界面上每一句话按对外产品的标准写。** 先问这句有没有必要存在。
   不写俏皮话、不写实现细节、不用口语词尾。
4. **修 bug 带一个「不打补丁就会失败」的测试。** 这个库里被找出来的几个 bug
   从外面完全看不出来，只有测试直接断言不变量时才暴露。

## 工具层面的坑

- `xorm` 的 `QueryString()` 返回的全是字符串，需要再转数字。
  `SQL().Get(&int64)` 和 `Find(&[]struct{})` 更好用，新代码优先用后两者
  （`service/sync.go`、`purge.go` 里还有旧写法，属于已知的技术债）。
- xorm 的 `Exec` 签名是 `(...any)`，SQL 本身也是其中一个元素，动态拼 SQL 时要整体拼进切片。
- 迁移文件用 `embed.FS` 打包，改完直接重新编译即可，不需要额外生成步骤。
- 新增依赖后记得 `go mod tidy`，CI 会用 `go mod tidy -diff` 卡住。
- `golangci-lint` 会报 errcheck，故意丢弃返回值请显式写 `_ =` 并说明原因。
