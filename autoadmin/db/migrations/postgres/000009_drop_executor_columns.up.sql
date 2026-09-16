-- 唯一执行器 OPA、唯一执行位置 Agent 端，字段失去区分意义，物理删除。
-- PG 侧差异：无（多动作 ALTER TABLE ... DROP COLUMN, DROP COLUMN 两侧同形）。
ALTER TABLE inspection_check
  DROP COLUMN executor,
  DROP COLUMN execution_location;

ALTER TABLE baseline_item
  DROP COLUMN executor;
