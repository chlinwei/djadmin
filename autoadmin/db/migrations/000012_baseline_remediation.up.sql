-- 扫描结果快照增加修复建议：随扫描落库（快照语义，与 chapter 一致），
-- 文案本体存于 baseline_item.config.remediation（JSON 内字段，无需改策略表）。

ALTER TABLE `baseline_scan_result` ADD COLUMN `remediation` longtext NULL AFTER `message`;
