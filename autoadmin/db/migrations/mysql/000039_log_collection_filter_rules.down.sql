-- 回滚采集过滤的结构改动（与 up 严格互逆）。
--
-- 注意：回滚会**丢掉**服务级的 exclude 覆盖与模板级的两个规则引用（列被删），
-- 也会丢掉规则的方向类型（列被删，重新 up 后存量规则一律回到 include）。

ALTER TABLE `assets_application_service_log_setting`
  DROP COLUMN `collection_exclude_filter_rule_id`;

ALTER TABLE `assets_application_log_definition`
  DROP COLUMN `filter_exclude_rule_id`,
  DROP COLUMN `filter_include_rule_id`;

ALTER TABLE `monitor_log_collection_filter_rule`
  DROP COLUMN `rule_type`;
