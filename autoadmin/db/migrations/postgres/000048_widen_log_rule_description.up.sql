-- PG 侧差异：MySQL 用 MODIFY COLUMN，PG 用 ALTER COLUMN ... TYPE；语义与本目录同版本号 mysql 文件一致。
-- 日志规则「说明」放宽：处理规则与采集过滤规则的 description 从 varchar(500) 提到 varchar(2000)。
ALTER TABLE monitor_log_processing_rule
  ALTER COLUMN description TYPE varchar(2000);

ALTER TABLE monitor_log_collection_filter_rule
  ALTER COLUMN description TYPE varchar(2000);
