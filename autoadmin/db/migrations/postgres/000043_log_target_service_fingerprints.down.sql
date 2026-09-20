-- 反向：删掉服务级指纹列（服务级状态记录不可恢复；重跑 up 后等价于"服务视图从未下发"）。
ALTER TABLE monitor_log_collection_target
  DROP COLUMN service_fingerprints;
