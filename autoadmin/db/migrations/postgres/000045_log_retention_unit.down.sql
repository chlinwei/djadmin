-- 回滚：丢掉单位列、把值列改回 `retention_days`。
--
-- 注意：若已存在"以小时为单位"的档位，这些行的 `retention_value` 是小时数，
-- 改回 `retention_days` 后语义会变成"天"——回滚前请先把小时档位改回天，
-- 或确认这批档位可以废弃（这是"降级丢语义"的固有代价，不是本迁移的疏漏）。
ALTER TABLE monitor_log_retention_tier DROP COLUMN retention_unit;

ALTER TABLE monitor_log_retention_tier RENAME COLUMN retention_value TO retention_days;
