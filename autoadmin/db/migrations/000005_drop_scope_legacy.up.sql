-- 开发阶段清理：挂载点模型已完全取代 scope/静态范围。
-- 1) 巡检组删除 scope 列（目标解析由挂载点决定，变量校验由 category 决定）；
-- 2) 任务删除 logical_service_id / selected_host_ids（挂载点动态解析，任务不再保存目标）；
-- 3) legacy_static 绑定与对应任务无法迁移到挂载点，直接删除（执行历史的 task_id 置空保留记录）。

ALTER TABLE `inspection_group` DROP COLUMN `scope`;

DELETE tg FROM `inspection_task_group` tg WHERE tg.mount_type = 'legacy_static';

UPDATE `inspection_execution` e
LEFT JOIN `inspection_task_group` tg ON tg.task_id = e.task_id
SET e.task_id = NULL
WHERE e.task_id IS NOT NULL AND tg.task_id IS NULL;

DELETE t FROM `inspection_task` t
WHERE NOT EXISTS (SELECT 1 FROM `inspection_task_group` tg WHERE tg.task_id = t.id);

ALTER TABLE `inspection_task` DROP FOREIGN KEY `inspection_task_logical_service_id_58a03b12_fk_assets_ap`;

ALTER TABLE `inspection_task`
  DROP COLUMN `logical_service_id`,
  DROP COLUMN `selected_host_ids`;
