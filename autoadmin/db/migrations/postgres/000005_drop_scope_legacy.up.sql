-- 开发阶段清理：挂载点模型已完全取代 scope/静态范围。
-- 1) 巡检组删除 scope 列（目标解析由挂载点决定，变量校验由 category 决定）；
-- 2) 任务删除 logical_service_id / selected_host_ids（挂载点动态解析，任务不再保存目标）；
-- 3) legacy_static 绑定与对应任务无法迁移到挂载点，直接删除（执行历史的 task_id 置空保留记录）。

ALTER TABLE inspection_group DROP COLUMN scope;

-- PG 侧差异：MySQL 多表 DELETE（DELETE tg FROM inspection_task_group tg WHERE ...）→ 单表 DELETE。
DELETE FROM inspection_task_group WHERE mount_type = 'legacy_static';

-- PG 侧差异：MySQL 的多表 UPDATE ... LEFT JOIN ... SET（左连接未命中者）改写为
-- UPDATE + NOT EXISTS；另外 PG 的 SET 目标列不能带表别名（SET e.task_id 是语法错误）。
UPDATE inspection_execution e
SET task_id = NULL
WHERE e.task_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM inspection_task_group tg WHERE tg.task_id = e.task_id);

-- PG 侧差异：MySQL 多表 DELETE（DELETE t FROM inspection_task t WHERE ...）→ 单表 DELETE。
DELETE FROM inspection_task t
WHERE NOT EXISTS (SELECT 1 FROM inspection_task_group tg WHERE tg.task_id = t.id);

-- PG 侧差异：MySQL 的 DROP FOREIGN KEY 在 PG 是 DROP CONSTRAINT（Django 时代的外键名两侧同名）。
ALTER TABLE inspection_task DROP CONSTRAINT inspection_task_logical_service_id_58a03b12_fk_assets_ap;

ALTER TABLE inspection_task
  DROP COLUMN logical_service_id,
  DROP COLUMN selected_host_ids;
