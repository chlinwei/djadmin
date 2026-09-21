-- 日志规则「说明」放宽：处理规则与采集过滤规则的 description 从 varchar(500) 提到 varchar(2000)。
-- 前端已改为多行 textarea（可写更长说明），500 字符在需要写清"适用日志格式 / 保留或排除的理由"时不够用。
ALTER TABLE `monitor_log_processing_rule`
  MODIFY COLUMN `description` varchar(2000) NOT NULL;

ALTER TABLE `monitor_log_collection_filter_rule`
  MODIFY COLUMN `description` varchar(2000) NOT NULL;
