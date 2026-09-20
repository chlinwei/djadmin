-- 逻辑服务 code 的唯一域从「全局唯一」收窄为 (business_system_id, environment_id)，与 name 对齐。
--
-- 背景：code 的定位是 name 的受控标识（name 可任意取，code 不是），所以二者的唯一域应当一致。
-- 旧的全局唯一会迫使同一逻辑服务在不同业务/环境下改用人为后缀（如 artemis-aos-poc），
-- 反而丢掉了「跨环境按同一 code 关联」的能力。同域唯一在库里已有先例：业务系统 code 就是
-- 「项目内唯一」（assets_business_system 的 unique_project_business_system_code）。
--
-- environment_id 可空时不参与唯一（SQL 唯一索引对 NULL 不去重），与既有 name 约束口径一致；
-- 服务层已强制 environment 必填，NULL 只是历史/防御性兜底。
--
-- 真库上的全局 code 唯一名可能是 Django 生成的（`code` 或带哈希后缀），不按名字删、按列现查现删，
-- 与 000040 同一手法；两类库（真库 / 按折叠快照建的库）都要能跑。
SET @code_unique_index := (
  SELECT INDEX_NAME FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'assets_application_service'
    AND COLUMN_NAME = 'code'
    AND NON_UNIQUE = 0
  GROUP BY INDEX_NAME
  HAVING COUNT(*) = 1
  LIMIT 1
);
SET @drop_code_unique := IF(
  @code_unique_index IS NULL,
  'SELECT 1',
  CONCAT('ALTER TABLE `assets_application_service` DROP INDEX `', @code_unique_index, '`')
);
PREPARE drop_code_unique_stmt FROM @drop_code_unique;
EXECUTE drop_code_unique_stmt;
DEALLOCATE PREPARE drop_code_unique_stmt;

-- 幂等加新约束：真库（Django 基线）需要新建；按折叠快照建的库可能已带同名索引，跳过即可。
SET @has_scoped := (
  SELECT COUNT(*) FROM information_schema.STATISTICS
  WHERE TABLE_SCHEMA = DATABASE()
    AND TABLE_NAME = 'assets_application_service'
    AND INDEX_NAME = 'unique_business_environment_service_code'
);
SET @add_scoped := IF(
  @has_scoped > 0,
  'SELECT 1',
  'ALTER TABLE `assets_application_service` ADD UNIQUE KEY `unique_business_environment_service_code` (`business_system_id`,`environment_id`,`code`)'
);
PREPARE add_scoped_stmt FROM @add_scoped;
EXECUTE add_scoped_stmt;
DEALLOCATE PREPARE add_scoped_stmt;

-- service_fingerprints 的 key 由「裸 service code」改为 service id：同一 code 的服务现在可以跨业务
-- 共存于同一主机，旧 key 不再唯一。整体清空（存量行本就允许为空，见 000043），下发一次后即恢复；
-- 读取侧对空 map 也会回退整机指纹，不会误报。
UPDATE `monitor_log_collection_target` SET `service_fingerprints` = JSON_OBJECT();
