-- PG 侧差异：MySQL 用 MODIFY COLUMN，PG 用 ALTER COLUMN ... TYPE；语义一致。
-- 回滚：说明列恢复 varchar(500)。超过 500 字符的说明会因超长而回滚失败，需先人工清理。
ALTER TABLE monitor_log_processing_rule
  ALTER COLUMN description TYPE varchar(500);

ALTER TABLE monitor_log_collection_filter_rule
  ALTER COLUMN description TYPE varchar(500);
