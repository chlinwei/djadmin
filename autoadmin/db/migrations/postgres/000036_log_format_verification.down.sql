-- 反向：删掉认证状态 4 列（认证历史不可恢复；重跑 up 后所有 (服务×日志定义) 回到"未验证"）。
ALTER TABLE assets_application_service_log_setting
  DROP COLUMN format_verified_at,
  DROP COLUMN format_verified_fingerprint,
  DROP COLUMN format_verified_source,
  DROP COLUMN format_verified_by;
