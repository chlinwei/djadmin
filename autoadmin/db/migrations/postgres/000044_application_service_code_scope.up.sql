-- 逻辑服务 code 的唯一域从「全局唯一」收窄为 (business_system_id, environment_id)，与 name 对齐。
-- 完整背景与取舍见同版本 MySQL 迁移头注释。
--
-- PG 侧差异：不拼约束名（Django 截断命名规则与 MySQL 不同），直接按列查 pg_constraint 现查现删；
-- 约束类型 `u`（UNIQUE），列集合恰好等于 {code} 才删。与 000040 的按列动态删除同一手法。
DO $$
DECLARE con record;
BEGIN
  FOR con IN
    SELECT c.conname
    FROM pg_constraint c
    JOIN pg_class rel ON rel.oid = c.conrelid
    WHERE rel.relname = 'assets_application_service'
      AND c.contype = 'u'
      AND (
        SELECT array_agg(a.attname ORDER BY a.attname)
        FROM pg_attribute a
        WHERE a.attrelid = c.conrelid AND a.attnum = ANY (c.conkey)
      ) = ARRAY['code']
  LOOP
    EXECUTE format('ALTER TABLE assets_application_service DROP CONSTRAINT %I', con.conname);
  END LOOP;
END $$;

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'unique_business_environment_service_code'
  ) THEN
    ALTER TABLE assets_application_service
      ADD CONSTRAINT unique_business_environment_service_code UNIQUE (business_system_id, environment_id, code);
  END IF;
END $$;

-- service_fingerprints 的 key 由「裸 service code」改为 service id（见 MySQL 迁移说明），整体清空。
UPDATE monitor_log_collection_target SET service_fingerprints = '{}'::jsonb;
