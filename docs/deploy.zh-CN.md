# 部署与运维

[English](deploy.md) · **简体中文**

Knockbox 是一个 Go 二进制加一个 SQLite 文件。**不需要数据库服务，不需要 Redis，
不需要消息队列**——限流在内存里计数，推送重试队列就是那个 SQLite 文件里的一张表。
能跑容器的机器就能跑它。

跑起来之后怎么发消息，见 [api.zh-CN.md](api.zh-CN.md)。

- [选一种装法](#选一种装法)
- [Docker Compose](#docker-compose)
- [Docker，一条命令](#docker一条命令)
- [预编译二进制](#预编译二进制)
- [从源码编译](#从源码编译)
- [镜像](#镜像)
- [第一次启动](#第一次启动)
- [放到公网上](#放到公网上)
- [升级与回滚](#升级与回滚)
- [备份](#备份)
- [出问题的时候](#出问题的时候)
- [配置](#配置)
- [给别人用的那种形态](#给别人用的那种形态)
- [用你自己的 APNs 密钥](#用你自己的-apns-密钥)

## 选一种装法

| | 适合 | 升级方式 |
|---|---|---|
| [Docker Compose](#docker-compose) | 大多数人。一个文件跟数据放在一起。 | `docker compose pull && docker compose up -d` |
| [Docker，一条命令](#docker一条命令) | 一分钟先试试。 | 停掉、删掉、重新 `docker run` |
| [预编译二进制](#预编译二进制) | 没有容器运行时，或者机器本来就用 systemd 管。 | 换掉二进制，重启 |
| [从源码编译](#从源码编译) | 想改点什么。需要 Go 1.26+。 | `git pull && make build` |

下面所有例子里的 `push.example.com` 都是「手机够得着的那个地址」，换成你自己的。

## Docker Compose

把仓库里的 [compose.yaml](../compose.yaml) 拿过去，把 `IGO_SERVER_EXTERNAL_URL`
改成你的地址，然后：

```bash
docker compose up -d
docker compose logs
```

整个文件短到可以直接读：

```yaml
services:
  knockbox:
    image: ghcr.io/aichy126/knockbox:latest
    container_name: knockbox
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    environment:
      IGO_SERVER_EXTERNAL_URL: "https://push.example.com"
      IGO_SERVER_NAME: "My Push"
    volumes:
      - knockbox-data:/app/data
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 30s
      timeout: 5s
      start_period: 10s
      retries: 3

volumes:
  knockbox-data:
```

有两行值得多看一眼：

- **`127.0.0.1:8080:8080`** 只把端口开在回环上，因为容器里说的是明文 HTTP，
  TLS 该由前面的反向代理来终结。只在本机试的话，写成 `"8080:8080"` 就行。
- **`knockbox-data:/app/data`** 是命名卷。想让数据落在看得见的地方，换成路径：
  `- ./data:/app/data`。

日常：

```bash
docker compose logs -f           # 跟日志
docker compose restart           # 重启
docker compose pull && docker compose up -d   # 升级
docker compose down              # 停掉（卷还在）
```

## Docker，一条命令

```bash
docker run -d --name knockbox -p 8080:8080 -v knockbox-data:/app/data \
  -e IGO_SERVER_EXTERNAL_URL=http://localhost:8080 \
  ghcr.io/aichy126/knockbox
docker logs knockbox
```

适合先看一眼。要长期跑就用 Compose——那样配置留在一个文件里，下个月还找得到。

## 预编译二进制

每个打过 tag 的版本都带 Linux、macOS、Windows、FreeBSD 的二进制，
架构覆盖 amd64、arm64、386 和 arm。从
[releases 页](https://github.com/aichy126/knockbox/releases)下对应的那个：

```bash
tar xzf knockbox_*_linux_amd64.tar.gz
cp config.toml.example config.toml   # 改 server.external_url
./knockbox serve -c config.toml
```

压缩包里是二进制、`config.toml.example`、两份 README 和两份 SECURITY。
没有别的东西要装：不需要运行时，也不依赖任何动态库。

想要一份 systemd unit：

```ini
[Unit]
Description=Knockbox
After=network-online.target

[Service]
ExecStart=/opt/knockbox/knockbox serve -c /opt/knockbox/config.toml
WorkingDirectory=/opt/knockbox
User=knockbox
Restart=always
RestartSec=3

[Install]
WantedBy=multi-user.target
```

## 从源码编译

```bash
git clone https://github.com/aichy126/knockbox.git
cd knockbox
make build          # 产出 ./knockbox
```

需要 Go 1.26 或更新，另外构建管理界面要 Node 20+（`make build` 会自己先跑）。
Go 这边是 `CGO_ENABLED=0`，而且必须一直是——SQLite 驱动是纯 Go 的，
静态二进制和两阶段镜像都建立在这件事上。

**交付物没有变**：仍然是一个二进制加一个 SQLite 文件。管理界面构建到 `admin/dist`
再 `go:embed` 进去，所以 release 的二进制和 Docker 镜像里本来就带着它——
`npm` 是贡献者的成本，不是使用者的。跳过它照样编得过，只是后台会说它还没构建，
其余功能一概照常。

## 镜像

| Registry | 地址 |
|---|---|
| GitHub Container Registry | [`ghcr.io/aichy126/knockbox`](https://github.com/aichy126/knockbox/pkgs/container/knockbox) |
| Docker Hub | [`aichy66/knockbox`](https://hub.docker.com/r/aichy66/knockbox) |

两边都有 `linux/amd64` 和 `linux/arm64`，而且**是同一个镜像**——发布流程只构建一次，
另一边是把 manifest 复制过去的，所以 digest 完全一致。哪边拉得快用哪边。

tag 是 `latest` 和确切版本号 `v1.0.3`，**没有 `v1` / `1.0` 这种会漂的 tag**：
升级应该是你决定做的事。

```bash
docker pull ghcr.io/aichy126/knockbox:v1.0.3   # 钉版本
docker pull aichy66/knockbox                   # 或者从 Docker Hub 拉 latest
```

## 第一次启动

第一次启动会建好数据库和一个管理员账号，并把这个账号的用户名和密码打印在日志里，
**只打印这一次，别处没有**：

```bash
docker compose logs | grep -A2 -i password
```

然后：

1. 打开 `https://push.example.com/login`，用它登录。
2. 进配对设备页，那里有一个二维码。
3. 装上 [iOS 客户端](https://apps.apple.com/app/id6811317906)，扫它。
4. 在同一个后台的设置页里把密码改掉。

密码没记下来就没了？从命令行重设，不需要动数据库：

```bash
docker compose exec -it knockbox /app/knockbox user passwd admin
```

### 最要紧的是 `IGO_SERVER_EXTERNAL_URL`

配对链接和附件链接都按它生成，所以它必须是**手机真正够得着的地址**，不是容器自己看到的
那个。填错的两种症状，而且都长得像别的毛病：

- 配对成功了，但时间线里每张图都是空白。
- 二维码扫上了，然后 app 根本连不上服务器。

这个地址在配对之前就能验：`GET /api/v1/server` 会返回服务器名和版本，
客户端正是靠它分清「地址填错」和「配对码错」。

## 放到公网上

服务端说的是明文 HTTP，不终结 TLS。前面放一个反向代理，容器留在回环上。

**Caddy** —— 短的那种，证书一起管了：

```caddyfile
push.example.com {
	reverse_proxy 127.0.0.1:8080
	request_body {
		max_size 25MB
	}
}
```

**nginx**：

```nginx
server {
    listen 443 ssl http2;
    server_name push.example.com;

    ssl_certificate     /etc/letsencrypt/live/push.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/push.example.com/privkey.pem;

    # 至少要等于 storage.max_upload_mb，否则别的都正常、
    # 只有附件会 413。
    client_max_body_size 25m;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

`X-Forwarded-For` 值得配上：配对和注册的限制是按 IP 算的，不配的话所有请求看起来
都来自代理本身。

然后把 `IGO_SERVER_EXTERNAL_URL` 设成 `https://push.example.com` 并重启。

## 升级与回滚

```bash
docker compose pull && docker compose up -d
```

表结构迁移在启动时按顺序跑，并且记录下来——升级就是换一个更新的镜像。
不过还是先读一眼 release notes：需要你留意的改动写在那里。

回滚就是把旧 tag 钉回去：

```yaml
image: ghcr.io/aichy126/knockbox:v1.0.2
```

迁移不会自动回退。回退一个版本不用担心；跨好几个版本，请恢复升级之前的备份。

## 备份

要备份的全在 `/app/data` 下：

```
data/
  knockbox.db        数据库——消息、频道、设备、账号
  knockbox.db-wal    预写日志
  files/             附件，按内容寻址
  logs/              轮转的日志文件
  backup/            自动快照，见下
```

服务端每 `retention.backup_every_hours`（默认 24）小时给数据库做一次快照，
保留 `retention.backup_keep` 份（默认 7）。快照走 SQLite 的 `VACUUM INTO`，
所以即便服务端正在写，每一份也是一个一致的单文件——它不是文件拷贝。

**快照里没有附件。** 要备份全部，就停掉容器整个目录拷走，或者把 `data/files/`
和最新那份快照一起拷。

恢复：

```bash
docker compose down
# 把快照放回 data/knockbox.db，并删掉它旁边残留的 -wal / -shm
docker compose up -d
```

唯一要避免的是趁服务端运行时直接拷 `knockbox.db`：没有旁边那个 `-wal` 文件，
你拿到的是一个缺了最近写入的库。

## 出问题的时候

**还活着吗？**

```bash
curl -fsS https://push.example.com/health          # 含数据库连通性
curl -fsS https://push.example.com/api/v1/server   # 名字、版本、能力
```

**app 什么都看不到 / 配不上对。** 先查 `IGO_SERVER_EXTERNAL_URL`，见上。
配对码是一次性的，`server.pair_ttl`（默认 10 分钟）后过期，重来要签一个新的，
拿旧的重试没用。

**发送返回 `"code":1`。** 原因在 `msg` 里。注意被拒绝的发送**仍然是 HTTP 200**，
见[读响应](api.zh-CN.md#读响应)。

**发送返回 401。** 频道 token 不对，或者频道已经被删了。不存在和已停用给同一句话，
这是刻意的。

**发送成功但手机没响。** 按这个顺序查：

1. 频道是不是静音了？静音存在服务端，消息照样存下来了。
2. iOS 设置里这个 app 的通知权限还在吗，设备在 app 的设备列表里还在吗？
3. `data/logs/knockbox.log` 记录每一次推送尝试和 APNs 的回答。
   返回 `410` 的设备是被 Apple 注销了，重新打开 app 就会回来。

**消息正常，只有附件发不出去。** 代理上的 `client_max_body_size` 小于
`storage.max_upload_mb`。

需要更多细节时把 `IGO_LOCAL_LOGGER_LEVEL` 设成 `DEBUG`。

## 配置

复制 `config.toml.example` 改，或者用环境变量——每一项都有对应的变量。
规则是 `IGO_` 加上配置路径的大写形式，点换成下划线：

```
server.external_url         ->  IGO_SERVER_EXTERNAL_URL
apns.production.key_base64  ->  IGO_APNS_PRODUCTION_KEY_BASE64
limit.send_qps              ->  IGO_LIMIT_SEND_QPS
```

密钥这类东西就该用环境变量传：写进 `config.toml` 的值，是一个别人读得到的文件里的值。

`[public]` 那一段不一样。**在后台里保存过一次之后，以存下来的值为准**，
再改环境变量不再有效——配置文件只是首次启动的默认值。

<details>
<summary><b>全部配置项</b></summary>

### `[server]`

| 键 | 默认值 | 作用 |
|---|---|---|
| `name` | `"My Push"` | app 里显示的服务器名。 |
| `external_url` | `"http://localhost:8080"` | 手机够得着的地址。配对链接和附件链接按它生成。 |
| `pair_ttl` | `600` | 配对码有效期，秒。一次性。 |

### `[local]` —— HTTP 服务

| 键 | 默认值 | 作用 |
|---|---|---|
| `address` | `":8080"` | 监听地址。不写主机名就是监听所有网卡。 |
| `debug` | `false` | Gin 的 debug 模式：启动时打印全部路由、请求日志更啰嗦。它是噪音，不是额外的安全。 |

### `[local.logger]`

| 键 | 默认值 | 作用 |
|---|---|---|
| `dir` | `"./data/logs"` | 日志目录，启动时自动建。 |
| `name` | `"knockbox.log"` | 文件名。 |
| `access` | `true` | 每个 HTTP 请求记一行。 |
| `level` | `"INFO"` | `DEBUG`、`INFO`、`WARN`、`ERROR`。 |
| `max_size` | `50` | 超过多少 MB 轮转。 |
| `max_backups` | `5` | 保留几个轮转文件。 |
| `max_age` | `14` | 保留多少天。 |

### `[sqlite.knockbox]`

| 键 | 默认值 | 作用 |
|---|---|---|
| `data_source` | `./data/knockbox.db?…` | 路径加一串 pragma。**整条 pragma 链不要拆**——WAL、`busy_timeout`、`secure_delete`、`auto_vacuum` 每一项都是有用的，服务端启动时会断言其中关键的几项。 |
| `max_open` / `max_idle` | `4` / `4` | 连接池。 |
| `is_debug` | `false` | 打印每条 SQL。 |

### `[bootstrap]` —— 第一个账号

| 键 | 默认值 | 作用 |
|---|---|---|
| `username` | `"admin"` | 库里一个账号都没有时自动创建。 |
| `password` | `""` | 留空表示随机生成一个，并打印到启动日志里一次。 |

### `[apns]`

| 键 | 默认值 | 作用 |
|---|---|---|
| `enabled` | `true` | 关掉就完全不推（消息照常存）。 |
| `topic` | `"com.miramiao.knockbox"` | 推送投向哪个 bundle id。只有换成你自己的密钥、并自己编译客户端时才改它。 |
| `team_id` | `"258X46W652"` | 密钥所属的 Team。 |
| `concurrency` | `8` | 并发推送数。 |
| `timeout_ms` | `8000` | 单次请求超时。 |
| `max_attempts` | `5` | 重试次数，退避 5s / 30s / 2m / 10m / 30m。 |
| `default_expiration` | `86400` | 让 APNs 尝试投递多少秒。**别设 0**——那是「现在投不到就丢掉」。 |

`[apns.production]` 和 `[apns.sandbox]` 各有 `key_id`、`key_file`、`key_base64`。
production 全留空就用仓库里内置的那把 topic-specific 密钥，见
[用你自己的 APNs 密钥](#用你自己的-apns-密钥)。sandbox 只有你自己用 Xcode Debug
编客户端时才需要。

### `[storage]` —— 附件

| 键 | 默认值 | 作用 |
|---|---|---|
| `dir` | `"./data/files"` | blob 放哪。按内容寻址。 |
| `max_upload_mb` | `20` | 单个附件上限。前面的代理至少要放行这么大。 |
| `url_ttl` | `604800` | 签名下载地址的有效期，秒。通知可能延迟投递，设太短会出现「点开是空白图」。 |
| `image_max_px` | `1600` | 图片会被重新编码到这个范围内。 |
| `thumb_max_px` | `600` | 缩略图尺寸。通知扩展只有 24MB 内存预算。 |
| `jpeg_quality` | `82` | 重编码质量。 |

### `[retention]` —— 保留与备份

| 键 | 默认值 | 作用 |
|---|---|---|
| `days` | `0` | 删除多少天之前的消息，实例级，对所有人生效。0 表示永久保留。 |
| `backup_every_hours` | `24` | 快照间隔。0 表示关掉。 |
| `backup_keep` | `7` | 保留几份快照。 |
| `backup_dir` | `"./data/backup"` | 快照放哪。 |

### `[reply]` —— 答复回调

| 键 | 默认值 | 作用 |
|---|---|---|
| `webhook_timeout_ms` | `5000` | 单次尝试的超时。 |
| `webhook_max_attempts` | `3` | 重试次数，退避 5s / 30s / 2m。 |
| `public_allow_private` | `false` | 回调能不能打到内网地址。只对公共实例有意义；自建实例默认允许，因为要够得着的正是那些地址。 |

### `[limit]` —— 防滥用，哪种模式都生效

| 键 | 默认值 | 作用 |
|---|---|---|
| `send_qps` | `20` | 每个频道 token 每秒多少条。0 表示不限。 |
| `send_burst` | `60` | 一次最多攒多少条。 |
| `pair_per_min` | `5` | 每个 IP 每分钟能提交几次配对。配对码只有 8 个字符——短到能念出来，所以也就只有 40 位——单次使用、10 分钟过期、加上这条限流，三样合起来才让猜它变得不现实。 |
| `body_max_kb` | `1024` | 单条正文上限。超了是拒绝，不是截断。 |

### `[public]` —— 见[给别人用的那种形态](#给别人用的那种形态)

| 键 | 默认值 | 作用 |
|---|---|---|
| `enabled` | `false` | 打开后首页变成接入页。 |
| `register_per_hour` | `3` | 每个 IP 每小时能开几个身份。 |
| `max_channels` | `20` | 每个成员的频道数上限。 |
| `max_messages_per_day` | `500` | 每个成员滚动 24 小时内的条数上限。 |
| `retention_days` | `30` | 每个成员的保留期。它和 `retention.days` 同时设时按更严的算。 |
| `site_name` | `"Knockbox"` | 公开页面上显示的名字。 |
| `docs_url` | `"/docs"` | 接入页上「怎么用」指向哪。 |

</details>

## 给别人用的那种形态

同一个二进制还有第二种形态。`public.enabled = true` 时，首页变成接入页：
陌生人打开它，扫一下就拿到一个身份和第一个频道，全程不注册、不设密码。
<https://knockbox.miramiao.com/> 就是这种形态。

`[public]` 那一段配置之所以存在，是因为这种形态需要一些私有实例不需要的限制：
一个 IP 能开几个身份、每个成员能有几个频道、每天能发多少条、他的消息留多久。

关掉时——也就是默认——首页跳到 `/login`，接入相关的接口一律 404。
谁摸到这个地址都不能在你的服务器上开账号。

## 用你自己的 APNs 密钥

仓库里内置了一把 APNs 密钥，为的是让你不必有 Apple 开发者账号也能自建。
它是 **topic-specific** 的：只能推 `com.miramiao.knockbox`，也就是 App Store 上那个
客户端，推不了别的。

它是公开的，要诚实说明的代价写在 [SECURITY.zh-CN.md](../SECURITY.zh-CN.md) 里：
任何**同时**拿到某台设备 APNs token 的人，都能给那台设备推任意内容。
要去掉这条路，换成你自己的密钥：

1. 在你的 Apple 开发者账号里创建一把 APNs Auth Key。
2. 把 `apns.production.key_file` 指向它（或者用 `IGO_APNS_PRODUCTION_KEY_BASE64` 传），
   并填好 `apns.production.key_id` 和 `apns.team_id`。
3. 把 `apns.topic` 改成你自己的 bundle id。
4. 用那个 bundle id 自己编译并签名 iOS 客户端。

第 4 步不是可选的：APNs 的 device token 绑定在 bundle id 上，
你的密钥推不动别人签名的 app。
