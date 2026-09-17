-- 定时静音。
--
-- muted 是「一直静音」，mute_until 是「静音到某个时刻」——两者分开而不是用一个
-- mute_until=极大值 表示永久：前者是用户的长期意图，后者会自然过期，
-- 混在一起之后就没法在界面上区分「我关掉了这个频道」和「我现在忙一小时」。
ALTER TABLE channel ADD COLUMN mute_until INTEGER NOT NULL DEFAULT 0;
