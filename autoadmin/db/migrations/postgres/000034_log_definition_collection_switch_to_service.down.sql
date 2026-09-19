-- 反向：恢复列结构（up 搬迁出的服务级覆盖行**不撤回**——它们与"模板关闭"在旧逻辑里等价）。
-- 值为近似重建：既然开关只剩服务级，一个整模板的布尔值只能二选一——
--   所有引用该模板的服务都显式覆盖为 FALSE => FALSE；否则 TRUE（含"没有任何覆盖行"= 采）。
-- 分三步（先加可空列再补 NOT NULL）是为了能在**有数据**的表上执行，PG 不支持
-- 直接 ADD COLUMN NOT NULL 到非空表。
ALTER TABLE assets_application_log_definition ADD COLUMN collection_enabled boolean;

UPDATE assets_application_log_definition ld
SET collection_enabled = CASE WHEN EXISTS (
      SELECT 1 FROM assets_application_service s
      LEFT JOIN assets_application_service_log_setting ls
        ON ls.service_id = s.id AND ls.log_definition_id = ld.id
      WHERE s.deployment_template_id = ld.deployment_template_id
        AND (ls.id IS NULL OR ls.collection_enabled IS NULL OR ls.collection_enabled = TRUE)
    ) THEN TRUE ELSE FALSE END;

ALTER TABLE assets_application_log_definition ALTER COLUMN collection_enabled SET NOT NULL;
