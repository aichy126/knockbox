-- 区分「从未被任何消息引用过」和「引用过、现在降回 0」。
--
-- 附件回收只看 ref_count <= 0，于是 /upload 之后、消息发出去之前的那段时间里，
-- 文件是 ref_count = 0 的状态。GC 每小时一轮，只要撞上就会把它扫掉，
-- 随后发出去的消息引用到一个磁盘上已经不存在的附件。
-- 而「先上传、再分别发给 N 个频道」正是 /upload 这个接口存在的理由，
-- 两次请求之间的间隔由调用方决定，可以很长。
--
-- 加这一列之后，回收规则变成：引用过的降到 0 立即回收（行为不变），
-- 从未引用过的要等过了宽限期才回收。
ALTER TABLE file ADD COLUMN ever_referenced INTEGER NOT NULL DEFAULT 0;

-- 已有引用的行显然被引用过。当前 ref_count = 0 的历史行无从判断，
-- 保持 0 即可：它们的 created_at 早已超出宽限期，回收行为和过去一致。
UPDATE file SET ever_referenced = 1 WHERE ref_count > 0;
