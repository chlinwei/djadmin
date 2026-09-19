-- 反向只能恢复列结构：被删的覆盖值不可恢复，重建出的列一律为 NULL（= 继承模板日志定义，
-- 与升级后的语义一致）。
-- 该列在 Django 时代的真库上原本还挂着一个指向 monitor_log_processing_rule 的外键，
-- 这里**不重建**——与 000005.down 同一处理（折叠快照里本就没有那个外键，重建会与快照不一致）；
-- 需要它的话由 up 的"现查现删"逻辑保证幂等。
ALTER TABLE `assets_application_service_log_setting` ADD COLUMN `processing_rule_id` bigint DEFAULT NULL;
