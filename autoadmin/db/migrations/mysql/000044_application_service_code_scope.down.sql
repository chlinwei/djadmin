-- 回滚：恢复 code 全局唯一。若已存在跨业务/环境的重复 code，需先手工去重再执行。
-- service_fingerprints 的清空不可逆（属一次性口径变更，不做反向数据恢复）。
ALTER TABLE `assets_application_service` DROP INDEX `unique_business_environment_service_code`;
ALTER TABLE `assets_application_service` ADD UNIQUE KEY `code` (`code`);
