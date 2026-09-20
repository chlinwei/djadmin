-- 回滚：丢掉单位列、把值列改回 `retention_days`。
--
-- 注意：若已存在"以小时为单位"的档位，这些行的 `retention_value` 是小时数，
-- 改回 `retention_days` 后语义会变成"天"——回滚前请先把小时档位改回天，
-- 或确认这批档位可以废弃（这是"降级丢语义"的固有代价，不是本迁移的疏漏）。
--
-- 另外：up 摘掉的 Django 生成 CHECK（`<表>_chk_1`）**不重建**（与 000005.down / 000035.down
-- 同一处理：折叠快照里本就没有它，重建会与折叠态不一致；up 的"现查现删"保证幂等）。
ALTER TABLE `monitor_log_retention_tier`
  DROP COLUMN `retention_unit`;

ALTER TABLE `monitor_log_retention_tier`
  CHANGE COLUMN `retention_value` `retention_days` int unsigned NOT NULL;
