-- 扫描结果快照增加修复建议：随扫描落库（快照语义，与 chapter 一致），
-- 文案本体存于 baseline_item.config.remediation（JSON 内字段，无需改策略表）。
--
-- PG 侧差异：PG 不支持 MySQL 的 AFTER 子句，新列固定加在表末尾（列顺序不影响 sqlc 与查询）。
ALTER TABLE baseline_scan_result ADD COLUMN remediation text NULL;
