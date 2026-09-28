-- 反向：删掉服务级后处理器列。
ALTER TABLE `assets_application_service_log_setting`
  DROP COLUMN `extra_processors`;
