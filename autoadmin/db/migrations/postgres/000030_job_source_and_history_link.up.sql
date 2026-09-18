-- PG 侧差异：多表 UPDATE ... JOIN 改写为 UPDATE ... SET ... FROM；
-- CONCAT 改用 ||；CREATE INDEX 语句不带 ON 表后的 USING 差异。
-- 运行记录中心两个 tab（自动化任务运行记录 / 监控安装历史）是同一批安装作业的两个视图：
-- 1) 监控安装历史行记录对应的 automation_execution_job.id，前端据此从历史跳转到作业；
-- 2) 自动化作业打来源标记，通用作业列表可按来源过滤/排除监控安装作业。
-- 来源取值：manual=普通任务、agent_install=Agent 安装/更新、monitor_target=exporter/filebeat 安装。

ALTER TABLE monitor_target_install_history
  ADD COLUMN automation_job_id_snapshot bigint DEFAULT NULL;

ALTER TABLE automation_execution_job
  ADD COLUMN source varchar(32) NOT NULL DEFAULT 'manual';

-- 回填历史->作业关联：派发时 job 与 history 用同一个 now，create_time 精确相等；
-- task_name_snapshot = "<type> <action>"，且 template_name_snapshot = exporter_type_snapshot = <type>。
-- Filebeat 作业名首字母大写（"Filebeat install"），故用 LOWER 比较。
UPDATE monitor_target_install_history h
SET automation_job_id_snapshot = j.id
FROM automation_execution_job j
WHERE j.create_time = h.create_time
  AND j.template_name_snapshot = h.exporter_type_snapshot
  AND LOWER(j.task_name_snapshot) = LOWER(h.exporter_type_snapshot || ' ' || h.action)
  AND h.automation_job_id_snapshot IS NULL;

-- 按历史关联把监控安装作业的来源回填为 monitor_target。
UPDATE automation_execution_job j
SET source = 'monitor_target'
FROM monitor_target_install_history h
WHERE h.automation_job_id_snapshot = j.id
  AND j.source = 'manual';

-- Agent 安装/更新作业按主机日志里的 agent_job_id 回填来源。
UPDATE automation_execution_job j
SET source = 'agent_install'
WHERE j.source = 'manual'
  AND EXISTS (
    SELECT 1 FROM automation_execution_host_log l
    JOIN assets_agent_job a ON a.job_id = l.agent_job_id
    WHERE l.job_id = j.id
  );

CREATE INDEX monitor_hist_auto_job_id_idx ON monitor_target_install_history (automation_job_id_snapshot);
