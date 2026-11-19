-- 升级时暂停基线扫描与旧 API；同一主机/条目的旧版本问题合并到最早的问题 ID。
-- 合并不可逆，升级前备份 security_issue、security_issue_history、baseline_scan_result。
ALTER TABLE security_issue
  ADD COLUMN last_rule_version varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN failure_rule_version varchar(64) NOT NULL DEFAULT '',
  ADD COLUMN closure_reason varchar(32) NOT NULL DEFAULT '';
ALTER TABLE security_issue_history ADD COLUMN rule_version varchar(64) NOT NULL DEFAULT '';
UPDATE security_issue SET last_rule_version = rule_key, failure_rule_version = rule_key,
  closure_reason = CASE WHEN status = 'closed' THEN 'verified' ELSE '' END;
UPDATE security_issue_history h JOIN security_issue i ON i.id = h.issue_id
SET h.rule_version = i.rule_key;

ALTER TABLE baseline_scan_result RENAME COLUMN rule_key TO rule_version;
ALTER TABLE baseline_scan_result DROP INDEX baseline_result_rule,
  ADD INDEX baseline_result_rule (host_id, item_id, scan_id);

CREATE TEMPORARY TABLE security_issue_merge_totals AS
SELECT host_id, item_id, MIN(id) AS keep_id, MIN(first_seen) AS first_seen, MAX(last_seen) AS last_seen,
       SUM(occurrence_count) AS occurrence_count, SUM(reopen_count) AS reopen_count
FROM security_issue GROUP BY host_id, item_id;
ALTER TABLE security_issue_merge_totals ADD PRIMARY KEY (keep_id), ADD UNIQUE (host_id, item_id);

CREATE TEMPORARY TABLE security_issue_merge_map AS
SELECT i.id AS old_id, t.keep_id
FROM security_issue i JOIN security_issue_merge_totals t ON t.host_id = i.host_id AND t.item_id = i.item_id;
ALTER TABLE security_issue_merge_map ADD PRIMARY KEY (old_id);

CREATE TEMPORARY TABLE security_issue_merge_heads AS
SELECT ranked.* FROM (
  SELECT i.*, ROW_NUMBER() OVER (PARTITION BY host_id, item_id
    ORDER BY last_checked DESC, latest_scan_id DESC, id DESC) AS observation_rank
  FROM security_issue i
) ranked WHERE observation_rank = 1;
ALTER TABLE security_issue_merge_heads ADD PRIMARY KEY (host_id, item_id);

CREATE TEMPORARY TABLE security_issue_merge_failures AS
SELECT host_id, item_id, rule_version AS failure_rule_version FROM (
  SELECT i.host_id, i.item_id, h.rule_version,
    ROW_NUMBER() OVER (PARTITION BY i.host_id, i.item_id
      ORDER BY h.checked_at DESC, h.scan_id DESC, h.id DESC) AS observation_rank
  FROM security_issue_history h JOIN security_issue i ON i.id = h.issue_id WHERE h.result = 'fail'
) ranked WHERE observation_rank = 1;
ALTER TABLE security_issue_merge_failures ADD PRIMARY KEY (host_id, item_id);

-- error 只更新检查新鲜度；跨版本开关状态仍取最近一次明确结论。
CREATE TEMPORARY TABLE security_issue_merge_decisions AS
SELECT host_id, item_id, result, checked_at, rule_version FROM (
  SELECT i.host_id, i.item_id, h.result, h.checked_at, h.rule_version,
    ROW_NUMBER() OVER (PARTITION BY i.host_id, i.item_id
      ORDER BY h.checked_at DESC, h.scan_id DESC, h.id DESC) AS observation_rank
  FROM security_issue_history h JOIN security_issue i ON i.id = h.issue_id WHERE h.result IN ('fail', 'pass')
) ranked WHERE observation_rank = 1;
ALTER TABLE security_issue_merge_decisions ADD PRIMARY KEY (host_id, item_id);

UPDATE security_issue i
JOIN security_issue_merge_totals t ON t.keep_id = i.id
JOIN security_issue_merge_heads h ON h.host_id = i.host_id AND h.item_id = i.item_id
JOIN security_issue_merge_failures f ON f.host_id = i.host_id AND f.item_id = i.item_id
JOIN security_issue_merge_decisions d ON d.host_id = i.host_id AND d.item_id = i.item_id
SET i.host_name = h.host_name, i.host_ip = h.host_ip, i.title = h.title, i.severity = h.severity,
    i.chapter = h.chapter, i.status = CASE WHEN d.result = 'pass' THEN 'closed' ELSE 'open' END,
    i.last_result = h.last_result,
    i.first_seen = t.first_seen, i.last_seen = t.last_seen, i.last_checked = h.last_checked,
    i.latest_scan_id = h.latest_scan_id, i.occurrence_count = t.occurrence_count,
    i.reopen_count = t.reopen_count, i.closed_at = CASE WHEN d.result = 'pass' THEN d.checked_at ELSE NULL END,
    i.last_rule_version = h.last_rule_version, i.failure_rule_version = f.failure_rule_version,
    i.closure_reason = CASE
      WHEN d.result = 'pass' AND d.rule_version <> f.failure_rule_version THEN 'rule_changed'
      WHEN d.result = 'pass' THEN 'verified' ELSE '' END;

-- 不删除、去重或重写历史证据；异常的同扫描重复历史由唯一约束显式阻止迁移。
UPDATE security_issue_history h JOIN security_issue_merge_map m ON m.old_id = h.issue_id
SET h.issue_id = m.keep_id WHERE m.old_id <> m.keep_id;
DELETE i FROM security_issue i JOIN security_issue_merge_map m ON m.old_id = i.id
WHERE m.old_id <> m.keep_id;

ALTER TABLE security_issue DROP INDEX security_issue_identity,
  ADD UNIQUE KEY security_issue_identity (host_id, item_id);
UPDATE security_issue SET rule_key = LPAD(LOWER(HEX(item_id)), 64, '0');
ALTER TABLE security_issue ALTER COLUMN last_rule_version DROP DEFAULT,
  ALTER COLUMN failure_rule_version DROP DEFAULT;
ALTER TABLE security_issue_history ALTER COLUMN rule_version DROP DEFAULT;

DROP TEMPORARY TABLE security_issue_merge_failures;
DROP TEMPORARY TABLE security_issue_merge_decisions;
DROP TEMPORARY TABLE security_issue_merge_heads;
DROP TEMPORARY TABLE security_issue_merge_map;
DROP TEMPORARY TABLE security_issue_merge_totals;
