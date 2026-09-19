-- 采集开关下沉到逻辑服务：部署模板的日志定义不再有"是否采集"（列随后删除）。
--
-- 背景：模板上的 collection_enabled 是**整模板**的开关，同一模板被多个逻辑服务复用时，
-- 无法只关某个服务的某条日志；而且渲染查询把它写在 JOIN 条件里
-- （`ld.collection_enabled = TRUE`），服务级覆盖选"强制开启"也不生效——开关语义是坏的。
-- 删列后唯一的每日志开关是服务级覆盖行 ls.collection_enabled：
-- **无覆盖行或覆盖为 NULL = 采（默认采、按需关）**，显式 FALSE = 不采。
--
-- 语义映射（必须保持"升级前后采集范围不变"）：
--   模板 collection_enabled = FALSE  => 该模板下**所有**逻辑服务的所有实例都不采。
--   所以这里先把这些组合逐一搬成服务级覆盖行 FALSE，再删列。
-- 已有覆盖行的组合不动：覆盖行本来就是权威（旧逻辑里 COALESCE(覆盖, 模板)）。
-- 用 ON CONFLICT DO NOTHING 吸收 (service_id, log_definition_id) 唯一冲突（MySQL 侧是 INSERT IGNORE）。
INSERT INTO assets_application_service_log_setting
  (create_time,update_time,remark,collection_enabled,log_definition_id,retention_tier_id,service_id,processing_rule_id,collection_filter_rule_id)
SELECT now(),now(),NULL,FALSE,ld.id,NULL,s.id,NULL,NULL
FROM assets_application_log_definition ld
JOIN assets_application_service s ON s.deployment_template_id = ld.deployment_template_id
WHERE ld.collection_enabled = FALSE
ON CONFLICT (service_id, log_definition_id) DO NOTHING;

ALTER TABLE assets_application_log_definition DROP COLUMN collection_enabled;
