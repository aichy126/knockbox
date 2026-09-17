-- Knockbox 初始 schema。
--
-- 两条只能在这里做、以后补不回来的设置：
--   1. auto_vacuum 只能在建表【之前】设置，而 PRAGMA 在事务里不生效——所以它由 migrate.bootstrap()
--      在本文件执行【之前】跑，不要往迁移文件里写 PRAGMA。
--   2. 所有时间列一律 INTEGER unix 秒。SQLite 没有原生时间类型，存整数让游标比较变成裸整数比较。

-- ---------------------------------------------------------------- 基础设施

CREATE TABLE IF NOT EXISTS schema_migration (
  version    INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  applied_at INTEGER NOT NULL
);

-- 全局单调序列。message.rev 从这里取：SQLite 单写者，UPDATE ... RETURNING 一次拿到即可。
CREATE TABLE IF NOT EXISTS seq (
  name TEXT PRIMARY KEY,
  val  INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO seq(name, val) VALUES ('rev', 0);

-- 服务端自持的小状态：file_sign_secret / gc_watermark / admin_key_hash ...
CREATE TABLE IF NOT EXISTS kv (
  k          TEXT PRIMARY KEY,
  v          TEXT    NOT NULL,
  updated_at INTEGER NOT NULL
);

-- ---------------------------------------------------------------- 用户与设备

CREATE TABLE IF NOT EXISTS user (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  name        TEXT    NOT NULL DEFAULT 'owner',
  -- 已读水位线：id <= 此值的消息视为已读。
  -- 挂在 user 而不是 device 上：手机上读了，iPad 的角标也该清。
  -- 「全部已读」因此是一次 UPDATE，而不是把几千行 message 的 rev 全部推高、把同步流量炸掉。
  read_cursor INTEGER NOT NULL DEFAULT 0,
  status      INTEGER NOT NULL DEFAULT 1,     -- 1 正常  2 停用
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS device (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  uuid         TEXT    NOT NULL,              -- 客户端生成、存 Keychain，app 重装后仍稳定
  user_id      INTEGER NOT NULL,
  name         TEXT    NOT NULL DEFAULT '',   -- "aichy 的 iPhone"
  platform     TEXT    NOT NULL DEFAULT 'ios',
  model        TEXT    NOT NULL DEFAULT '',
  os_version   TEXT    NOT NULL DEFAULT '',
  app_version  TEXT    NOT NULL DEFAULT '',
  apns_token   TEXT    NOT NULL DEFAULT '',   -- hex；未授权通知时为空
  apns_env     TEXT    NOT NULL DEFAULT 'production',
  auth_hash    TEXT    NOT NULL,              -- sha256(device_token)。token 只在配对时出现一次，永不回显
  auth_prefix  TEXT    NOT NULL DEFAULT '',   -- 明文前 8 位，仅用于设备列表展示
  sync_rev     INTEGER NOT NULL DEFAULT 0,    -- 这台设备同步到哪了
  badge        INTEGER NOT NULL DEFAULT 0,
  status       INTEGER NOT NULL DEFAULT 1,    -- 1 正常  2 主动登出  3 APNs 失效(410)
  fail_count   INTEGER NOT NULL DEFAULT 0,
  last_push_at INTEGER NOT NULL DEFAULT 0,
  last_seen_at INTEGER NOT NULL DEFAULT 0,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_device_uuid ON device(uuid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_device_auth ON device(auth_hash);
-- 同一个 APNs token 绝不能挂在两台设备上，否则一条消息推两遍。
-- 换机 / 从备份恢复确实会把 token 带到新设备行上，冲突时「新行接管，老行清空 token 并置 status=3」。
CREATE UNIQUE INDEX IF NOT EXISTS ux_device_apns ON device(apns_token) WHERE apns_token <> '';
CREATE INDEX IF NOT EXISTS ix_device_live ON device(user_id, status);

-- ---------------------------------------------------------------- 频道
--
-- 频道是 app 的概念，服务端只存它【必须知道】的那几样：
--   token  —— 鉴权凭据，校验只能在服务端做
--   muted  —— NSE 拦不住通知展示，所以静音只能是「服务端干脆不推」
--   sound / level —— 要写进 APNs payload，而 payload 是服务端在推送那一刻组装的
-- 名字 / 图标 / 颜色 / 排序全部塞进 meta，服务端只存不读（多设备同步用的不透明 blob）。

CREATE TABLE IF NOT EXISTS channel (
  id           TEXT    PRIMARY KEY,           -- app 生成的 ULID；进 APNs payload，app 靠它认频道
  user_id      INTEGER NOT NULL,
  token        TEXT    NOT NULL,              -- 发送凭据。与 id 分开：payload 会经过 Apple 并留在
                                              -- userInfo 里，id 即 token 等于把发送权限广播出去
  muted        INTEGER NOT NULL DEFAULT 0,
  sound        TEXT    NOT NULL DEFAULT 'default',  -- '' = 静音
  level        TEXT    NOT NULL DEFAULT 'active',   -- passive|active|time-sensitive|critical
  meta         TEXT    NOT NULL DEFAULT '',         -- app 写的不透明 JSON，服务端不解析
  msg_count    INTEGER NOT NULL DEFAULT 0,
  last_msg_id  INTEGER NOT NULL DEFAULT 0,
  last_msg_at  INTEGER NOT NULL DEFAULT 0,
  last_used_at INTEGER NOT NULL DEFAULT 0,    -- token 最后一次被用来发消息
  last_used_ip TEXT    NOT NULL DEFAULT '',
  status       INTEGER NOT NULL DEFAULT 1,    -- 1 正常  2 停用
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_channel_token ON channel(token);
CREATE INDEX IF NOT EXISTS ix_channel_user ON channel(user_id, status);

-- ---------------------------------------------------------------- 消息

CREATE TABLE IF NOT EXISTS message (
  id          INTEGER PRIMARY KEY AUTOINCREMENT,
  uid         TEXT    NOT NULL,              -- ULID，对外 ID；推送 payload 里带它
  -- 全局修订号，增量同步的唯一游标。
  -- 不能用 id：id 只对「新增」单调，但「标记已读 / 删除」会改老行，只靠 id > cursor 拉不到变更。
  rev         INTEGER NOT NULL,
  user_id     INTEGER NOT NULL,
  channel_id  TEXT    NOT NULL,
  type        TEXT    NOT NULL DEFAULT 'text',  -- text|markdown|image|file|link|card
  title       TEXT    NOT NULL DEFAULT '',
  -- 推送 body 用的截断摘要，服务端落库时算好。
  -- 必须非空：NSE 被系统跳过时，payload 里的它就是用户能看到的全部。
  summary     TEXT    NOT NULL DEFAULT '',
  body        TEXT    NOT NULL DEFAULT '',   -- 正文全文，无长度限制（正文不进 payload）
  extra       TEXT    NOT NULL DEFAULT '',   -- JSON：card items / link 元数据 / actions / copy
  file_id     INTEGER NOT NULL DEFAULT 0,
  collapse_id TEXT    NOT NULL DEFAULT '',   -- apns-collapse-id，<= 64 字节
  idem_key    TEXT    NOT NULL DEFAULT '',   -- 幂等键，防上游重试造重复推送
  from_name   TEXT    NOT NULL DEFAULT '',
  from_ip     TEXT    NOT NULL DEFAULT '',
  read_at     INTEGER NOT NULL DEFAULT 0,    -- 越过 user.read_cursor 的单条已读
  -- 软删。内容在删除那一刻就被物理抹掉（title/body/extra 置空），
  -- 这一行只作为不含任何内容的空壳墓碑供同步；GC 在所有活跃设备同步过之后连它一起删。
  deleted_at  INTEGER NOT NULL DEFAULT 0,
  created_at  INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_msg_uid    ON message(uid);
CREATE INDEX        IF NOT EXISTS ix_msg_rev    ON message(user_id, rev);          -- 增量同步
CREATE INDEX        IF NOT EXISTS ix_msg_ch     ON message(channel_id, id DESC);   -- 频道内倒序分页
CREATE INDEX        IF NOT EXISTS ix_msg_unread ON message(user_id, id) WHERE deleted_at = 0;
CREATE INDEX        IF NOT EXISTS ix_msg_gc     ON message(created_at);
-- 幂等：同一频道 + 同一 idem_key 只落一条
CREATE UNIQUE INDEX IF NOT EXISTS ux_msg_idem   ON message(channel_id, idem_key) WHERE idem_key <> '';

-- ---------------------------------------------------------------- 附件

CREATE TABLE IF NOT EXISTS file (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  uid        TEXT    NOT NULL,
  user_id    INTEGER NOT NULL,
  sha256     TEXT    NOT NULL,               -- 内容寻址，天然去重
  size       INTEGER NOT NULL,
  mime       TEXT    NOT NULL DEFAULT '',
  name       TEXT    NOT NULL DEFAULT '',
  width      INTEGER NOT NULL DEFAULT 0,
  height     INTEGER NOT NULL DEFAULT 0,
  thumb_path TEXT    NOT NULL DEFAULT '',    -- 通知扩展下载的小图，长边由 storage.thumb_max_px 决定
  storage    TEXT    NOT NULL DEFAULT 'local',
  path       TEXT    NOT NULL,
  -- 同一张图可能被多个频道的消息引用（sha256 去重的后果），
  -- 所以清空频道时必须靠 ref_count 判断，不能看到就删。
  ref_count  INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_file_uid ON file(uid);
CREATE UNIQUE INDEX IF NOT EXISTS ux_file_sha ON file(user_id, sha256);

-- ---------------------------------------------------------------- 配对

CREATE TABLE IF NOT EXISTS pair_code (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  code       TEXT    NOT NULL,               -- 8 位 Crockford Base32，单次使用，默认 10 分钟过期
  user_id    INTEGER NOT NULL,
  issued_by  TEXT    NOT NULL DEFAULT 'admin',  -- admin | device:<uuid> | bootstrap
  expires_at INTEGER NOT NULL,
  used_at    INTEGER NOT NULL DEFAULT 0,
  used_by    TEXT    NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS ux_pair_code ON pair_code(code);
CREATE INDEX        IF NOT EXISTS ix_pair_exp  ON pair_code(expires_at);

-- ---------------------------------------------------------------- 推送日志（兼做重试队列）
--
-- 服务重启后扫 status IN (0,2) 就能续推，不丢消息，也不用引入 Redis/NATS。
-- 这是 SQLite 在单机自建场景的最大红利。

CREATE TABLE IF NOT EXISTS push_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  message_id    INTEGER NOT NULL,
  device_id     INTEGER NOT NULL,
  apns_id       TEXT    NOT NULL DEFAULT '',
  status        INTEGER NOT NULL DEFAULT 0,  -- 0 待推 1 成功 2 可重试失败 3 终态失败 4 已放弃
  http_status   INTEGER NOT NULL DEFAULT 0,
  reason        TEXT    NOT NULL DEFAULT '', -- APNs reason 原样保留，排障全靠它
  attempts      INTEGER NOT NULL DEFAULT 0,
  next_retry_at INTEGER NOT NULL DEFAULT 0,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_push_retry ON push_log(status, next_retry_at) WHERE status IN (0, 2);
CREATE INDEX IF NOT EXISTS ix_push_msg   ON push_log(message_id);

-- ---------------------------------------------------------------- 频道清空事件
--
-- 清空是物理删除，删掉的行不存在了，别的设备永远学不到「这些没了」。
-- 所以把删除事件从「每条一个墓碑」上升为「频道级一条记录」——一条顶掉 N 个墓碑，反而更省。
-- 记录极小，长期保留；离线超过保留期的设备靠 sync 响应里的 reset 兜底。

CREATE TABLE IF NOT EXISTS purge_log (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id       INTEGER NOT NULL,
  channel_id    TEXT    NOT NULL,
  -- 删的是「id <= 此值」而不是「全部」：清空过程中恰好到达的新消息不会被静默吃掉。
  before_msg_id INTEGER NOT NULL,
  deleted_count INTEGER NOT NULL DEFAULT 0,
  rev           INTEGER NOT NULL,            -- 走同一个 seq，进增量同步流
  created_at    INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS ix_purge_rev ON purge_log(user_id, rev);
