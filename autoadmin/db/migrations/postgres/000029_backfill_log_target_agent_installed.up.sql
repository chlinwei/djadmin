-- 回填历史数据：Go 版 Filebeat 安装/卸载链路只写 install_status/runtime_status，
-- 从未维护 agent_installed，导致链路体检"主机配置"层、宿主列表 Filebeat 过滤、
-- "采集进程"层把已安装 Filebeat 的主机误判为"未安装"。
-- 按安装终态回填（install 成功 => TRUE，uninstall 成功 => FALSE），
-- unknown/failed/pending 保持原值不动。
UPDATE monitor_log_collection_target
SET agent_installed = TRUE
WHERE install_status = 'success' AND agent_installed = FALSE;

UPDATE monitor_log_collection_target
SET agent_installed = FALSE
WHERE install_status = 'uninstalled' AND agent_installed = TRUE;
