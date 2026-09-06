-- 构想 5（组挂载点模型）：任务↔巡检组绑定行携带挂载点，目标由挂载点在执行时动态解析。
-- 存量绑定行标为 legacy_static，继续走旧的目标解析路径（静态主机列表/逻辑服务）。

ALTER TABLE `inspection_task_group`
  ADD COLUMN `mount_type` varchar(16) NOT NULL DEFAULT 'legacy_static',
  ADD COLUMN `project_id` bigint DEFAULT NULL,
  ADD COLUMN `environment_id` bigint DEFAULT NULL,
  ADD COLUMN `business_system_id` bigint DEFAULT NULL,
  ADD COLUMN `instance_mode` varchar(8) DEFAULT NULL;
