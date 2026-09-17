-- 管理员账号。
--
-- 不单独建 admin 表：自建场景下「服务器的主人」和「收消息的人」通常就是同一个，
-- 分成两张表会逼出「管理员要不要也有频道」这种没有好答案的问题。
-- 一个 user 行既可以有 channel 和 device，也可以有密码用来登录管理界面。
ALTER TABLE user ADD COLUMN username      TEXT NOT NULL DEFAULT '';
ALTER TABLE user ADD COLUMN password_hash TEXT NOT NULL DEFAULT '';
-- admin 能进管理界面看所有人的东西；member 只是个收消息的身份，登录不了。
ALTER TABLE user ADD COLUMN role          TEXT NOT NULL DEFAULT 'member';
ALTER TABLE user ADD COLUMN last_login_at INTEGER NOT NULL DEFAULT 0;

-- 部分唯一索引：没设用户名的行（纯收消息的用户）不参与唯一性约束。
CREATE UNIQUE INDEX IF NOT EXISTS ux_user_username ON user(username) WHERE username <> '';

-- 管理界面的登录会话。
-- 存库而不是签 JWT：要能在界面上「踢掉其它设备」，也要能在改密码后立刻让旧会话失效。
CREATE TABLE IF NOT EXISTS session (
  id         TEXT    PRIMARY KEY,          -- 随机 token 的 sha256，明文只在 cookie 里
  user_id    INTEGER NOT NULL,
  user_agent TEXT    NOT NULL DEFAULT '',
  ip         TEXT    NOT NULL DEFAULT '',
  expires_at INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  last_seen_at INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS ix_session_user ON session(user_id);
CREATE INDEX IF NOT EXISTS ix_session_exp  ON session(expires_at);
