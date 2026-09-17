-- message.file_id 从来没被写入过，只被读取。
--
-- 删除单条、清空频道、保留策略这三条路都靠它定位「该给哪个 blob 减引用」，
-- 于是全部静默空转：历史消息删掉了，它们的附件永远留在磁盘上、
-- file 表里的 ref_count 也永远停在当初加上去的那个数。
--
-- 写入端已修（service/send.go 落库时填 file_id）。这里把历史数据补回来：
-- 附件 uid 一直存在 extra 的 "file" 字段里，能对应回 file.id。
UPDATE message
SET file_id = (
  SELECT f.id FROM file f
  WHERE f.uid = json_extract(message.extra, '$.file')
)
WHERE file_id = 0
  AND extra LIKE '%"file"%'
  AND EXISTS (
    SELECT 1 FROM file f
    WHERE f.uid = json_extract(message.extra, '$.file')
  );
