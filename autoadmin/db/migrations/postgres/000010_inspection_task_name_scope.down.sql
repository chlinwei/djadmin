-- 回滚恢复全局唯一。注意：若已存在同名跨组任务，需先手工去重再执行。
-- PG 侧差异：ADD UNIQUE KEY → ADD CONSTRAINT（名字按 PG 的全库唯一约定取 inspection_task_name）。
ALTER TABLE inspection_task ADD CONSTRAINT inspection_task_name UNIQUE (name);
