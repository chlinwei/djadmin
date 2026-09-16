-- 回滚：删除基线域全部表与数据。
-- PG 侧差异：无（DROP TABLE IF EXISTS 两侧同形；索引随表一并删除）。
DROP TABLE IF EXISTS baseline_scan_result;
DROP TABLE IF EXISTS security_scan_target;
DROP TABLE IF EXISTS security_scan;
DROP TABLE IF EXISTS baseline_item;
DROP TABLE IF EXISTS baseline;
