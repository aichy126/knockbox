-- 单个用户豁免配额。
--
-- 公共实例上运营者自己的账号不该被自己设的限额卡住，而他用的是同一套接口。
-- 与其在代码里特判「管理员不限」，不如做成一个可以显式打开的标记——
-- 管理员身份和「不限额」是两件事，将来也可能要给某个普通用户放开。
ALTER TABLE user ADD COLUMN unlimited INTEGER NOT NULL DEFAULT 0;
