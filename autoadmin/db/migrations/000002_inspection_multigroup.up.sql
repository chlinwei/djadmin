-- 巡检中心多组组合 + 组分类 + 结果按组分区（INSPECTION_DESIGN_IDEAS.md 构想 1/2/3）。
-- inspection_task.group_id 保留并继续写入第一个组（兼容旧查询），Go 侧以 inspection_task_group 为准。

ALTER TABLE `inspection_group`
  ADD COLUMN `category` varchar(16) NOT NULL DEFAULT 'general',
  ADD COLUMN `application_id` bigint DEFAULT NULL;

CREATE TABLE `inspection_task_group` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `task_id` bigint NOT NULL,
  `group_id` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `unique_inspection_task_group` (`task_id`,`group_id`)
);

-- 存量任务的单组关系迁入关联表。
INSERT INTO `inspection_task_group` (`task_id`, `group_id`)
SELECT `id`, `group_id` FROM `inspection_task` WHERE `group_id` IS NOT NULL;

ALTER TABLE `inspection_result`
  ADD COLUMN `group_id` bigint DEFAULT NULL,
  ADD COLUMN `group_name` varchar(128) NOT NULL DEFAULT '';
