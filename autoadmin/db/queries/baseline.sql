-- OS 基线扫描（baseline 域）查询。

-- name: CountBaselines :one
SELECT COUNT(*) FROM baseline
WHERE (sqlc.narg(pattern) IS NULL OR baseline.name LIKE sqlc.narg(pattern) OR baseline.description LIKE sqlc.narg(pattern));

-- name: ListBaselines :many
SELECT id, name, version, description, enabled, create_time, update_time,
       (SELECT COUNT(*) FROM baseline_item i WHERE i.baseline_id = baseline.id) AS item_count,
       (SELECT COUNT(*) FROM security_scan sc WHERE sc.baseline_id = baseline.id AND sc.scan_type='baseline') AS scan_count
FROM baseline
WHERE (sqlc.narg(pattern) IS NULL OR baseline.name LIKE sqlc.narg(pattern) OR baseline.description LIKE sqlc.narg(pattern))
ORDER BY id DESC
LIMIT ? OFFSET ?;

-- name: GetBaseline :one
SELECT id, name, version, description, enabled, create_time, update_time FROM baseline WHERE id = sqlc.arg(id);

-- name: CreateBaseline :execlastid
INSERT INTO baseline(create_time, update_time, name, version, description, enabled)
VALUES (NOW(6), NOW(6), ?, ?, ?, ?);

-- name: UpdateBaseline :exec
UPDATE baseline SET update_time = NOW(6), name = ?, version = ?, description = ?, enabled = ? WHERE id = ?;

-- name: DeleteBaseline :exec
DELETE FROM baseline WHERE id = ?;

-- name: ListBaselineItems :many
SELECT i.id, i.baseline_id, i.category_id, c.name AS category, i.sort, i.name, i.description, i.config, i.severity
FROM baseline_item i
JOIN baseline_category c ON c.id = i.category_id
WHERE i.baseline_id = sqlc.arg(baseline_id) ORDER BY c.sort, c.id, i.sort, i.id;

-- name: DeleteBaselineItems :exec
DELETE FROM baseline_item WHERE baseline_id = sqlc.arg(baseline_id);

-- name: CreateBaselineItem :execlastid
INSERT INTO baseline_item(create_time, update_time, baseline_id, category_id, sort, name, description, config, severity)
VALUES (NOW(6), NOW(6), ?, ?, ?, ?, ?, ?, ?);

-- name: GetBaselineItem :one
SELECT i.id, i.baseline_id, i.category_id, c.name AS category, i.sort, i.name, i.description, i.config, i.severity
FROM baseline_item i
JOIN baseline_category c ON c.id = i.category_id
WHERE i.id = sqlc.arg(id);

-- name: UpdateBaselineItem :execrows
UPDATE baseline_item SET category_id = ?, name = ?, description = ?, config = ?, severity = ?, update_time = NOW(6)
WHERE id = ?;

-- name: DeleteBaselineItem :execrows
DELETE FROM baseline_item WHERE id = ?;

-- name: MaxBaselineItemSort :one
SELECT CAST(COALESCE(MAX(sort), -1) AS SIGNED) AS max_sort FROM baseline_item WHERE category_id = sqlc.arg(category_id);

-- ---- 类目（策略分组实体，迁移 000011 起）----

-- name: ListBaselineCategories :many
SELECT id, baseline_id, sort, name FROM baseline_category
WHERE baseline_id = sqlc.arg(baseline_id) ORDER BY sort, id;

-- name: GetBaselineCategory :one
SELECT id, baseline_id, sort, name FROM baseline_category WHERE id = sqlc.arg(id);

-- name: MaxBaselineCategorySort :one
SELECT COALESCE(MAX(sort), -1) AS max_sort FROM baseline_category WHERE baseline_id = sqlc.arg(baseline_id);

-- name: CreateBaselineCategory :execlastid
INSERT INTO baseline_category(create_time, update_time, name, sort, baseline_id)
VALUES (NOW(6), NOW(6), ?, ?, ?);

-- name: RenameBaselineCategory :execrows
UPDATE baseline_category SET name = ?, update_time = NOW(6) WHERE id = ?;

-- name: UpdateBaselineCategorySort :execrows
UPDATE baseline_category SET sort = ?, update_time = NOW(6) WHERE id = ?;

-- name: DeleteBaselineCategory :execrows
DELETE FROM baseline_category WHERE id = ?;

-- name: GetBaselineForScan :one
SELECT id, name, version, enabled FROM baseline WHERE id = sqlc.arg(id);

-- name: CountBaselineItems :one
SELECT COUNT(*) FROM baseline_item WHERE baseline_id = sqlc.arg(baseline_id);

-- name: CreateBaselineScan :execlastid
INSERT INTO security_scan(create_time, update_time, baseline_id, mount_type, project_id, environment_id, status, summary, requested_username, start_time, end_time)
VALUES (NOW(6), NOW(6), ?, ?, ?, ?, 'pending', JSON_OBJECT(), ?, NULL, NULL);

-- name: ClaimBaselineScan :execrows
UPDATE security_scan SET status = 'running', start_time = NOW(6), update_time = NOW(6)
WHERE id = ? AND status = 'pending';

-- name: CreateBaselineScanTargets :exec
INSERT INTO security_scan_target(scan_id, host_id, host_name, host_ip, agent_id, status, error_message)
VALUES (?, ?, ?, ?, ?, 'pending', '');

-- name: GetBaselineScanTarget :one
SELECT id, host_id, host_name, host_ip, agent_id, status, passed_items, failed_items, compliance_rate, error_message
FROM security_scan_target WHERE scan_id = ? AND host_id = ?;

-- name: UpdateBaselineScanTarget :exec
UPDATE security_scan_target
SET status = ?, passed_items = ?, failed_items = ?, compliance_rate = ?, error_message = ?, scan_id = scan_id
WHERE id = ?;

-- name: FinishBaselineScanTarget :exec
UPDATE security_scan_target
SET status = ?, passed_items = ?, failed_items = ?, compliance_rate = ?, error_message = ?
WHERE id = ?;

-- name: CreateBaselineScanResults :exec
INSERT INTO baseline_scan_result(scan_id, host_id, item_id, item_name, chapter, severity, status, expected_value, actual_value, message)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: FinishBaselineScan :exec
UPDATE security_scan SET status = ?, summary = ?, end_time = NOW(6), update_time = NOW(6) WHERE id = ?;
