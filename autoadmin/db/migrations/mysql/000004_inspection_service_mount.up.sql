-- 单组任务模型：任务绑定唯一巡检组；挂载点支持逻辑服务级（service_id）。

ALTER TABLE `inspection_task_group`
  ADD COLUMN `service_id` bigint DEFAULT NULL;
