-- 回滚：说明列恢复 varchar(500)。超过 500 字符的说明会被截断（MySQL 非严格模式下），
-- 严格模式下回滚本身会失败——只能先人工清理超长说明。
ALTER TABLE `monitor_log_processing_rule`
  MODIFY COLUMN `description` varchar(500) NOT NULL;

ALTER TABLE `monitor_log_collection_filter_rule`
  MODIFY COLUMN `description` varchar(500) NOT NULL;
