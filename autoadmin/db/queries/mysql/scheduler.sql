-- 不再硬编码排除 cleanup_alert_histories：那条过滤曾把它从列表里藏起来，后果是"历史上从来没被
-- 清理过"这件事没人看得见（2026-09-19 才发现）。要隐藏某个任务请在业务上退役它（删行 + 写文档）。
-- name: CountScheduledTasks :one
SELECT COUNT(*) FROM scheduler_scheduledtask
WHERE (name LIKE sqlc.narg(name_pattern) OR code LIKE sqlc.narg(code_pattern) OR sqlc.narg(name_pattern) IS NULL)
  AND (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_running = sqlc.narg(is_running) OR sqlc.narg(is_running) IS NULL);

-- name: ListScheduledTasks :many
SELECT t.*, m.name AS menu_name, m.path AS menu_path
FROM scheduler_scheduledtask AS t
LEFT JOIN sys_menu AS m ON m.id = t.menu_id
WHERE (t.name LIKE sqlc.narg(name_pattern) OR t.code LIKE sqlc.narg(code_pattern) OR sqlc.narg(name_pattern) IS NULL)
  AND (t.enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (t.is_running = sqlc.narg(is_running) OR sqlc.narg(is_running) IS NULL)
ORDER BY t.id DESC
LIMIT ? OFFSET ?;

-- name: GetScheduledTask :one
SELECT t.*, m.name AS menu_name, m.path AS menu_path
FROM scheduler_scheduledtask AS t
LEFT JOIN sys_menu AS m ON m.id = t.menu_id
WHERE t.id = ? LIMIT 1;

-- name: UpdateScheduledTask :exec
UPDATE scheduler_scheduledtask
SET name = ?, code = ?, description = ?, enabled = ?, cron_expression = ?,
    interval_minutes = NULL, next_run_time = ?, update_time = ?
WHERE id = ?;

-- name: SetScheduledTaskEnabled :exec
UPDATE scheduler_scheduledtask
SET enabled = ?, next_run_time = ?, update_time = ?
WHERE id = ?;

-- name: CountScheduledTaskLogs :one
SELECT COUNT(*) FROM scheduler_scheduledtasklog
WHERE (task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (status = sqlc.narg(exact_status) OR sqlc.narg(exact_status) IS NULL)
  AND (duration_seconds >= sqlc.narg(duration_min) OR sqlc.narg(duration_min) IS NULL)
  AND (duration_seconds <= sqlc.narg(duration_max) OR sqlc.narg(duration_max) IS NULL)
  AND (message LIKE sqlc.narg(message_pattern) OR output LIKE sqlc.narg(output_pattern) OR sqlc.narg(message_pattern) IS NULL);

-- name: ListScheduledTaskLogs :many
SELECT l.id, t.name AS task_name, l.run_time, l.status, l.message, l.duration_seconds, l.output
FROM scheduler_scheduledtasklog AS l
JOIN scheduler_scheduledtask AS t ON t.id = l.task_id
WHERE (l.task_id = sqlc.narg(task_id) OR sqlc.narg(task_id) IS NULL)
  AND (l.status = sqlc.narg(exact_status) OR sqlc.narg(exact_status) IS NULL)
  AND (l.duration_seconds >= sqlc.narg(duration_min) OR sqlc.narg(duration_min) IS NULL)
  AND (l.duration_seconds <= sqlc.narg(duration_max) OR sqlc.narg(duration_max) IS NULL)
  AND (l.message LIKE sqlc.narg(message_pattern) OR l.output LIKE sqlc.narg(output_pattern) OR sqlc.narg(message_pattern) IS NULL)
ORDER BY l.run_time DESC
LIMIT ? OFFSET ?;

-- name: GetScheduledTaskLog :one
SELECT l.id, t.name AS task_name, l.run_time, l.status, l.message, l.duration_seconds, l.output
FROM scheduler_scheduledtasklog AS l
JOIN scheduler_scheduledtask AS t ON t.id = l.task_id
WHERE l.id = ? LIMIT 1;

-- name: ClaimScheduledTask :execresult
UPDATE scheduler_scheduledtask
SET is_running = TRUE, last_run_time = ?, update_time = ?
WHERE id = ? AND enabled = TRUE AND is_running = FALSE;

-- name: CompleteScheduledTask :exec
UPDATE scheduler_scheduledtask
SET is_running = FALSE, last_status = ?, last_message = ?, last_run_time = ?, update_time = ?
WHERE id = ?;

-- name: CreateScheduledTaskLog :exec
INSERT INTO scheduler_scheduledtasklog (
  create_time, update_time, remark, run_time, status, message,
  duration_seconds, task_id, output
) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?);

-- name: DeleteLoginAuditsBefore :execresult
DELETE FROM audit_login_log WHERE login_time < ?;

-- name: DeleteOperationAuditsBefore :execresult
DELETE FROM audit_operation_log WHERE created_at < ?;
-- ---- 四类保留期清理（2026-09-19 从 Django 时代的历史任务迁到 Go）----
--
-- 语义统一：**只删早于保留期的行，绝不动还在进行中的**（运行中的作业 / 没结束的会话都必须留着）；
-- 保留期一律从 sys_config 读（键见各 handler 注释），与 Django 时代用的是同一个键，升级后行为不变。

-- name: DeleteWebSSHSessionLogsBefore :execresult
-- assets_webssh_session_log 没有子表；按 start_time 判年龄：这条记录本身的"保留天数"，
-- 未正常结束（end_time 为空）的记录也要能清掉，否则崩溃留下的记录永远删不掉。
DELETE FROM assets_webssh_session_log WHERE start_time < sqlc.arg(before);

-- name: DeleteAutomationExecutionHostLogsBefore :execresult
-- 子表先删（外键无级联）：主机明细 → 作业字节块 → 作业行。只清 end_time 已过保留期的作业，
-- 运行中（end_time 为空）的一律不碰。
DELETE FROM automation_execution_host_log
WHERE job_id IN (
  SELECT id FROM automation_execution_job WHERE end_time IS NOT NULL AND end_time < sqlc.arg(before)
);

-- name: DeleteAutomationExecutionJobLogsBefore :execresult
DELETE FROM automation_execution_job_log
WHERE job_id IN (
  SELECT id FROM automation_execution_job WHERE end_time IS NOT NULL AND end_time < sqlc.arg(before)
);

-- name: DeleteAutomationExecutionJobsBefore :execresult
DELETE FROM automation_execution_job WHERE end_time IS NOT NULL AND end_time < sqlc.arg(before);

-- name: DeleteMonitorInstallHistoriesBefore :execresult
-- 每个纳管目标**至少保留最新一条**：清完也要能回答"这台机器最近一次装/卸是什么结果"
-- （sys_config 里这条配置的说明就是这么写的）。
-- target_id 为空的行（没有"目标"可留）按年龄删。
-- 子查询再套一层派生表：MySQL 不允许 DELETE 的目标表直接出现在子查询里（错误 1093）；
-- 外层与里层都必须给这张表起别名（SQL 里出现两次同名关系时，PG 解析 create_time 会报 ambiguous，
-- sqlc 的 PG 解析器同样卡在这一步）。
DELETE FROM monitor_target_install_history AS hist
WHERE hist.create_time < sqlc.arg(before)
  AND (
    hist.target_id IS NULL
    OR hist.id NOT IN (
      SELECT keep_id FROM (
        SELECT MAX(latest.id) AS keep_id FROM monitor_target_install_history AS latest
        WHERE latest.target_id IS NOT NULL GROUP BY latest.target_id
      ) AS keep_latest
    )
  );

-- name: DeleteAlertNotificationDeliveriesForHistoriesBefore :execresult
-- 告警历史**有子记录**（通知事件 → 投递记录，外键都无级联），所以清理必须带上它们、且先子后父。
-- 语义：投递记录是"这条告警的通知轨迹"，告警本身过期被清时它们失去归属；保留期跟着父记录走，
-- 而不是让父记录因为"有子记录"永远清不掉（那样这条保留期配置就形同虚设）。
DELETE FROM monitor_alert_notification_delivery
WHERE event_id IN (
  SELECT e.id FROM monitor_alert_notification_event AS e
  JOIN monitor_alert_history AS h ON h.id = e.alert_id
  WHERE h.state = 'resolved' AND COALESCE(h.resolved_at, h.started_at) < sqlc.arg(before)
);

-- name: DeleteAlertNotificationEventsForHistoriesBefore :execresult
DELETE FROM monitor_alert_notification_event
WHERE alert_id IN (
  SELECT h.id FROM monitor_alert_history AS h
  WHERE h.state = 'resolved' AND COALESCE(h.resolved_at, h.started_at) < sqlc.arg(before)
);

-- name: DeleteResolvedAlertHistoriesBefore :execresult
-- 只清**已恢复**的历史告警：仍在 firing 的记录是"当前状态"，删了会让告警页凭空少一条正在发的告警。
-- 年龄取 COALESCE(resolved_at, started_at)：恢复时间才是"这条记录还能留多久"的起点，
-- 而历史数据里有 state=resolved 但 resolved_at 为空的行（Django 时代的），回落到 started_at 才不会漏清。
DELETE FROM monitor_alert_history
WHERE state = 'resolved' AND COALESCE(resolved_at, started_at) < sqlc.arg(before);
