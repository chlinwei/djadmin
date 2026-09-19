-- 退役三条"平台已内置完成"的历史定时任务（2026-09-19）。
--
-- 背景：这三个任务随 Django 后端一起没了实现——定时调度会跳过它们、手动执行只会报错，
-- 但它们的名字还挂在「系统管理 → 定时任务」页上，看着像在用。逐条核对后确认：
-- 它们要做的事在 Go 里已经由别的机制持续做掉了，所以**删行**（而不是补一个重复的 handler——
-- 两套清理同一张表迟早互相打架），并把"谁在做"记在这里与 SCHEDULER_ARCHITECTURE.md 里：
--
--  1) cleanup_inspection_executions（巡检执行记录清理）
--     → 已由巡检自带的清理完成：internal/inspection/scheduler.go 的 cleanupExpiredExecutions，
--       每 24h 跑一次，保留期读 sys.inspection.executions.retention_days（本次同时修掉了一个
--       键名不匹配：Go 原先读 inspection.results.retention_days，那个键根本不存在）。
--  2) reconcile_prometheus_alert_history（历史告警对账）
--     → 已由告警侧的失联对账完成：internal/monitor/alert_notification.go 的 reconcileStaleAlertsLoop
--       是**进程内 ticker**（fireing 且 last_seen_at 超过 10 分钟即判恢复），比"每 5 分钟一条
--       定时任务"更及时，也不依赖队列。
--  3) cleanup_orphan_temp_credentials（WebSSH 临时凭证清理）
--     → 这套机制在 Go 里已不存在：没有任何 Go 代码写 assets_webssh_temp_credential
--       （Django 时代的 WebSSH 每次会话建一条临时凭证、由该任务清孤儿）。既是死机制，
--       顺手把历史残留清掉（见下方 DELETE），任务行退役。
--
-- 删除顺序：任务执行日志先删（scheduler_scheduledtasklog.task_id 有外键、无级联）。
-- 只删这三条；同页其余任务的日志与历史都保留。

DELETE FROM scheduler_scheduledtasklog WHERE task_id IN (
  SELECT id FROM scheduler_scheduledtask
  WHERE code IN ('cleanup_inspection_executions', 'reconcile_prometheus_alert_history', 'cleanup_orphan_temp_credentials')
);

DELETE FROM scheduler_scheduledtask
WHERE code IN ('cleanup_inspection_executions', 'reconcile_prometheus_alert_history', 'cleanup_orphan_temp_credentials');

-- 历史残留的 WebSSH 临时凭证：**先摘关联行、再删凭证**（外键方向是 临时凭证 → 凭证，
-- 反过来删会被外键挡住，整条迁移就失败了）。
-- 哪些凭证可以删：先落到临时表里——只清"没有被任何主机绑定引用"的凭证。
-- 万一某个"临时"凭证当年被挂到了主机上，留着比删错安全。
CREATE TEMPORARY TABLE tmp_webssh_temp_credential AS
  SELECT tc.credential_id AS credential_id FROM assets_webssh_temp_credential AS tc
  WHERE tc.credential_id NOT IN (SELECT hc.credential_id FROM assets_hostcredential AS hc);

-- 全部临时凭证行：这是随 Django 一起消失的机制，没有任何 Go 代码会写它（session_pk 指的
-- 是当年的 Django session 表，那张表也不在了），所以剩下的行只可能是历史残留。
DELETE FROM assets_webssh_temp_credential;

DELETE FROM assets_credential
WHERE id IN (SELECT credential_id FROM tmp_webssh_temp_credential);

DROP TABLE tmp_webssh_temp_credential;
