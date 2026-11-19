-- 本文件由 make derive 从 db/queries/mysql/security_issues.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- 共享父行锁允许不同主机并发提交，并与取消的排他锁互斥。
-- name: GetSecurityIssueScanForShare :one
SELECT create_time, status FROM security_scan WHERE id = sqlc.arg(id) FOR SHARE;

-- name: LockSecurityIssueHost :one
SELECT id FROM assets_host WHERE id = sqlc.arg(id) FOR UPDATE;

-- 没有问题记录的通过结果也必须参与时序判断，防止较早失败迟到时把已通过的主机标为未解决。
-- name: GetLatestSecurityRuleObservation :one
SELECT r.scan_id, s.create_time AS checked_at, r.status, r.rule_version, r.item_name, r.severity, r.chapter,
       COALESCE(r.expected_value, 'null') AS expected_value,
       COALESCE(r.actual_value, 'null') AS actual_value,
       r.message, COALESCE(r.remediation, '') AS remediation
FROM baseline_scan_result r JOIN security_scan s ON s.id = r.scan_id
WHERE r.host_id = sqlc.arg(host_id) AND r.item_id = sqlc.arg(item_id) AND r.rule_version IS NOT NULL
ORDER BY s.create_time DESC, s.id DESC, r.id DESC LIMIT 1;

-- name: GetSecurityIssueByIdentity :one
-- 已持有主机排他锁；不用不存在行的间隙锁，避免多主机首次建问题互相阻塞。
SELECT * FROM security_issue WHERE host_id = sqlc.arg(host_id) AND item_id = sqlc.arg(item_id);

-- name: CreateSecurityIssue :one
INSERT INTO security_issue (host_id, host_name, host_ip, baseline_id, item_id, rule_key, title, severity, chapter,
                           status, last_result, first_seen, last_seen, last_checked, latest_scan_id,
                           last_rule_version, failure_rule_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, 'open', 'fail', $10, $11, $12, $13, $14, $15)
RETURNING id;

-- name: UpdateSecurityIssue :exec
UPDATE security_issue SET host_name = $1, host_ip = $2, title = $3, severity = $4, chapter = $5,
    status = $6, last_result = $7, first_seen = $8, last_seen = $9, last_checked = $10, latest_scan_id = $11,
    occurrence_count = $12, reopen_count = $13, closed_at = $14,
    last_rule_version = $15, failure_rule_version = $16, closure_reason = $17
WHERE id = $18;

-- 迟到失败仅补全发现时间，不覆盖较新的检查结论。
-- name: UpdateSecurityIssueDiscoveryTimes :exec
UPDATE security_issue SET first_seen = $1, last_seen = $2 WHERE id = $3;

-- name: FailSecurityIssuePersistence :exec
UPDATE security_scan_target SET status = 'failed', error_message = '扫描结果与安全问题保存失败，请重新扫描'
WHERE id = sqlc.arg(id) AND status IN ('pending', 'running');

-- name: SecurityIssueHistoryExists :one
SELECT COUNT(*) FROM security_issue_history WHERE issue_id = $1 AND scan_id = $2;

-- name: CreateSecurityIssueHistory :exec
INSERT INTO security_issue_history (issue_id, scan_id, checked_at, result, transition, applied, expected_value, actual_value, message, remediation, rule_version)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11);

-- name: GetSecurityIssue :one
SELECT sqlc.embed(i), h.environment_id, COALESCE(e.name, '') AS environment_name
FROM security_issue i
LEFT JOIN assets_host h ON h.id = i.host_id
LEFT JOIN assets_business_environment e ON e.id = h.environment_id
WHERE i.id = sqlc.arg(id);

-- name: CountSecurityIssueHistory :one
SELECT COUNT(*) FROM security_issue_history WHERE issue_id = sqlc.arg(issue_id);

-- name: ListSecurityIssueHistory :many
SELECT * FROM security_issue_history WHERE issue_id = sqlc.arg(issue_id)
ORDER BY checked_at DESC, scan_id DESC, id DESC LIMIT $1 OFFSET $2;

-- A persistent failure is a milestone only if it ends a run of failures.
-- Compute this before pagination, ignoring unrelated observations within the run.
-- name: CountSecurityIssueMilestones :one
WITH history_order AS (
    SELECT transition,
           ROW_NUMBER() OVER (ORDER BY checked_at DESC, scan_id DESC) AS recency,
           LEAD(transition) OVER (
               PARTITION BY CASE WHEN transition IN ('discovered', 'persistent', 'reopened', 'closed', 'closed_rule_changed') THEN 1 ELSE 0 END
               ORDER BY checked_at, scan_id
           ) AS next_transition
    FROM security_issue_history AS src WHERE src.issue_id = sqlc.arg(issue_id) AND src.applied = TRUE
)
SELECT COUNT(*) FROM history_order
WHERE transition IN ('discovered', 'reopened', 'closed', 'closed_rule_changed')
   OR (transition = 'persistent' AND (next_transition IS NULL OR next_transition IN ('closed', 'closed_rule_changed')))
   OR (transition = 'check_error' AND recency = 1);

-- name: ListSecurityIssueMilestones :many
WITH history_order AS (
    SELECT id, transition,
           ROW_NUMBER() OVER (ORDER BY checked_at DESC, scan_id DESC) AS recency,
           LEAD(transition) OVER (
               PARTITION BY CASE WHEN transition IN ('discovered', 'persistent', 'reopened', 'closed', 'closed_rule_changed') THEN 1 ELSE 0 END
               ORDER BY checked_at, scan_id
           ) AS next_transition
    FROM security_issue_history AS src WHERE src.issue_id = sqlc.arg(issue_id) AND src.applied = TRUE
)
SELECT h.* FROM security_issue_history h INNER JOIN history_order o ON h.id = o.id
WHERE o.transition IN ('discovered', 'reopened', 'closed', 'closed_rule_changed')
   OR (o.transition = 'persistent' AND (o.next_transition IS NULL OR o.next_transition IN ('closed', 'closed_rule_changed')))
   OR (o.transition = 'check_error' AND o.recency = 1)
ORDER BY h.checked_at DESC, h.scan_id DESC, h.id DESC LIMIT $1 OFFSET $2;

-- name: CountSecurityIssues :one
SELECT COUNT(*) FROM security_issue i
LEFT JOIN assets_host h ON h.id = i.host_id
WHERE (i.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (i.severity = sqlc.narg(severity) OR sqlc.narg(severity) IS NULL)
  AND (i.host_id = sqlc.narg(host_id) OR sqlc.narg(host_id) IS NULL)
  AND (i.rule_key = sqlc.narg(rule_key) OR sqlc.narg(rule_key) IS NULL)
  AND (i.title LIKE sqlc.narg(pattern) OR i.host_name LIKE sqlc.narg(pattern) OR i.host_ip LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (h.environment_id = sqlc.narg(environment_id) OR sqlc.narg(environment_id) IS NULL)
  AND (sqlc.arg(environment_unset) = FALSE OR h.environment_id IS NULL);

-- name: ListSecurityIssues :many
SELECT sqlc.embed(i), h.environment_id, COALESCE(e.name, '') AS environment_name
FROM security_issue i
LEFT JOIN assets_host h ON h.id = i.host_id
LEFT JOIN assets_business_environment e ON e.id = h.environment_id
WHERE (i.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (i.severity = sqlc.narg(severity) OR sqlc.narg(severity) IS NULL)
  AND (i.host_id = sqlc.narg(host_id) OR sqlc.narg(host_id) IS NULL)
  AND (i.rule_key = sqlc.narg(rule_key) OR sqlc.narg(rule_key) IS NULL)
  AND (i.title LIKE sqlc.narg(pattern) OR i.host_name LIKE sqlc.narg(pattern) OR i.host_ip LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (h.environment_id = sqlc.narg(environment_id) OR sqlc.narg(environment_id) IS NULL)
  AND (sqlc.arg(environment_unset) = FALSE OR h.environment_id IS NULL)
ORDER BY i.last_checked DESC, i.id DESC LIMIT $1 OFFSET $2;

-- name: CountSecurityIssueGroups :one
SELECT COUNT(DISTINCT i.rule_key) FROM security_issue i
LEFT JOIN assets_host h ON h.id = i.host_id
WHERE (i.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (i.severity = sqlc.narg(severity) OR sqlc.narg(severity) IS NULL)
  AND (i.host_id = sqlc.narg(host_id) OR sqlc.narg(host_id) IS NULL)
  AND (i.rule_key = sqlc.narg(rule_key) OR sqlc.narg(rule_key) IS NULL)
  AND (i.title LIKE sqlc.narg(pattern) OR i.host_name LIKE sqlc.narg(pattern) OR i.host_ip LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (h.environment_id = sqlc.narg(environment_id) OR sqlc.narg(environment_id) IS NULL)
  AND (sqlc.arg(environment_unset) = FALSE OR h.environment_id IS NULL);

-- name: ListSecurityIssueGroups :many
SELECT i.rule_key, MAX(i.title) AS title,
       CASE WHEN COUNT(CASE WHEN i.severity = 'high' THEN 1 END) > 0 THEN 'high'
            WHEN COUNT(CASE WHEN i.severity = 'medium' THEN 1 END) > 0 THEN 'medium'
            ELSE 'low' END AS severity,
       COUNT(*) AS host_count,
       COUNT(CASE WHEN i.status = 'open' THEN 1 END) AS open_count,
       COUNT(CASE WHEN i.status = 'closed' THEN 1 END) AS closed_count,
       COUNT(CASE WHEN i.last_result = 'error' THEN 1 END) AS unverified_count,
       MIN(i.first_seen) AS first_seen, MAX(i.last_checked) AS last_checked
FROM security_issue i
LEFT JOIN assets_host h ON h.id = i.host_id
WHERE (i.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (i.severity = sqlc.narg(severity) OR sqlc.narg(severity) IS NULL)
  AND (i.host_id = sqlc.narg(host_id) OR sqlc.narg(host_id) IS NULL)
  AND (i.rule_key = sqlc.narg(rule_key) OR sqlc.narg(rule_key) IS NULL)
  AND (i.title LIKE sqlc.narg(pattern) OR i.host_name LIKE sqlc.narg(pattern) OR i.host_ip LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
  AND (h.environment_id = sqlc.narg(environment_id) OR sqlc.narg(environment_id) IS NULL)
  AND (sqlc.arg(environment_unset) = FALSE OR h.environment_id IS NULL)
GROUP BY i.rule_key ORDER BY MAX(i.last_checked) DESC, i.rule_key LIMIT $1 OFFSET $2;

-- 环境候选取全部台账主机，不受分页或当前筛选影响；仅需安全问题查看权限。
-- name: ListSecurityIssueEnvironments :many
SELECT DISTINCT h.environment_id, COALESCE(e.name, '') AS environment_name
FROM security_issue i
LEFT JOIN assets_host h ON h.id = i.host_id
LEFT JOIN assets_business_environment e ON e.id = h.environment_id
ORDER BY h.environment_id;
