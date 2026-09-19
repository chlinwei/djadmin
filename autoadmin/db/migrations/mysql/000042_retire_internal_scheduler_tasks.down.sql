-- 回滚只把三条任务行放回去（用它们在原库里的 id 与 cron），**不还原**已删的执行日志与
-- 已清掉的临时凭证残留：那些是历史垃圾，回滚它们没有意义。
-- 放回去的任务仍然是"没有 Go 实现"的状态（页面会标「未迁移」），这正是回滚后的真实处境。
INSERT INTO scheduler_scheduledtask
  (id, create_time, update_time, remark, name, code, description, enabled, interval_minutes,
   last_run_time, last_status, last_message, menu_id, next_run_time, is_running, cron_expression)
VALUES
  (8, NOW(6), NOW(6), '', 'WebSSH 临时凭证清理', 'cleanup_orphan_temp_credentials', '清理 WebSSH 会话遗留的临时凭证', TRUE, NULL, NULL, '', '', NULL, NULL, FALSE, '*/15 * * * *'),
  (10, NOW(6), NOW(6), '', '历史告警对账', 'reconcile_prometheus_alert_history', '对账 Prometheus 与平台的历史告警状态', TRUE, NULL, NULL, '', '', 108, NULL, FALSE, '*/5 * * * *'),
  (12, NOW(6), NOW(6), '', '巡检执行记录清理', 'cleanup_inspection_executions', '清理超过保留期的巡检执行记录', TRUE, NULL, NULL, '', '', 114, NULL, FALSE, '45 0 * * *');
