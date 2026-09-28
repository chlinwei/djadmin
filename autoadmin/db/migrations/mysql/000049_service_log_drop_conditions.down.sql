-- 反向：删掉按字段丢弃条件列。
-- 注意 CLI 只有 `migrate`（up）与 `migrate force`，没有 down 子命令；要在真库上清掉这一列，
-- 需要手工执行本语句并把 schema_migrations 的版本号退回 48（或直接用 `migrate force 48`
-- 清记录后手工 DROP COLUMN）。
ALTER TABLE `assets_application_service_log_setting`
  DROP COLUMN `drop_conditions`;
