-- 可回复的消息。
--
-- 发送方在发送时声明「这条能回」并给一个回调地址；用户在通知上或 app 里回一次，
-- 服务端把回复记在这一行上，再 POST 给那个地址。
--
-- 回调地址单独一列、不进 extra：extra 会原样下发给客户端，而回调地址是发送方的
-- 内部端点，没有任何理由让每一台配对过的设备都看到它。

-- 用户回了什么。choice 存选中的那一项，text 存原文。空串 = 还没回。
ALTER TABLE message ADD COLUMN reply TEXT NOT NULL DEFAULT '';
-- 回复的时刻。它和 reply 一起判空：reply 允许是空白文本（用户真的只发了空格），
-- 只看 reply 会把那种情况误判成「还没回」。
ALTER TABLE message ADD COLUMN replied_at INTEGER NOT NULL DEFAULT 0;
-- 回复时限。0 = 不限，这是默认：时限是发送方的可选项，
-- 只有「不回就会自动发生别的事」的消息才需要它。
ALTER TABLE message ADD COLUMN reply_until INTEGER NOT NULL DEFAULT 0;
-- 回调地址。有它才说明这条消息可回。
ALTER TABLE message ADD COLUMN reply_webhook TEXT NOT NULL DEFAULT '';

-- 回调投递队列。
--
-- 和 push_log 一样，表本身就是队列：进程重启后扫 status IN (0,2) 即可续投。
-- ⚠️ 同样只能单实例消费，理由见 service/webhook.go 的注释。
--
-- url 和 payload 在【回复那一刻】就快照进来，不在投递时回查 message。
-- 保留策略随时可能把那条消息物理删掉，而回调该不该送出去，在用户按下按钮时就已经定了；
-- 回查的话，一条等着重试的回调会因为消息过期而永远发不出去，且没有任何报错。
CREATE TABLE IF NOT EXISTS reply_hook (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id  INTEGER NOT NULL,
    url         TEXT    NOT NULL,
    payload     TEXT    NOT NULL,
    -- 签名密钥的快照。用频道 token 当密钥，而频道 token 可以被轮换——
    -- 轮换之后再投递，接收方拿新 token 验不过在旧 token 下签出来的名。
    secret      TEXT    NOT NULL DEFAULT '',
    status      INTEGER NOT NULL DEFAULT 0,  -- 0 待投 1 成功 2 待重试 3 已放弃
    attempt     INTEGER NOT NULL DEFAULT 0,
    next_at     INTEGER NOT NULL DEFAULT 0,
    status_code INTEGER NOT NULL DEFAULT 0,
    error       TEXT    NOT NULL DEFAULT '',
    created_at  INTEGER NOT NULL,
    updated_at  INTEGER NOT NULL
);

-- 取队列只按 status + next_at 扫，加索引免得每轮全表。
CREATE INDEX IF NOT EXISTS idx_reply_hook_claim ON reply_hook(status, next_at);
-- 后台按消息查它的投递状态。
CREATE INDEX IF NOT EXISTS idx_reply_hook_msg ON reply_hook(message_id);
