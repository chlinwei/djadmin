-- 不可无损回滚（legacy 列与数据已删）；回滚仅恢复列结构。
ALTER TABLE `inspection_task`
  ADD COLUMN `logical_service_id` bigint DEFAULT NULL,
  ADD COLUMN `selected_host_ids` json NOT NULL;

ALTER TABLE `inspection_group` ADD COLUMN `scope` varchar(24) NOT NULL DEFAULT 'per_deployment';
