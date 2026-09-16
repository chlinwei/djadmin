ALTER TABLE inspection_result
  DROP COLUMN group_id,
  DROP COLUMN group_name;

-- PG 侧差异：MySQL 的多表 DELETE（DELETE tg FROM inspection_task_group tg）改为单表 DELETE；
-- 原语句紧接着就 DROP TABLE，语义等价。
DELETE FROM inspection_task_group;

DROP TABLE inspection_task_group;

ALTER TABLE inspection_group
  DROP COLUMN application_id,
  DROP COLUMN category;
