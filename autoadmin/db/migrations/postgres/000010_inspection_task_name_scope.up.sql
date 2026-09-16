-- 任务名称唯一性从全局收窄为"同巡检组内唯一"：
-- 任务列表/执行记录都会展示所属组，歧义只存在于同组内；不同组允许复用通用名（如"日巡检"）。
--
-- PG 侧差异：MySQL 的 UNIQUE KEY 在 PG 里是 UNIQUE 约束，删除用 DROP CONSTRAINT；
-- 该约束名两侧同名（inspection_task 在 MySQL 里叫 name），但在 PG 里约束名全库唯一，
-- 与其他表的 name 冲突，按 db/schema/postgres 的 <表名>_name 约定命名为 inspection_task_name。
ALTER TABLE inspection_task DROP CONSTRAINT inspection_task_name;
