-- OS 基线扫描（baseline 域）查询。
--
-- 时间列一律由应用层传入（占位符 `?`），不用 NOW(6)/UTC_TIMESTAMP：PG 的 now() 不接受精度参数，
-- 且两方言的会话时区语义不同，应用层统一传 UTC 时间（见 docs/architecture/SQL_DESIGN.md §4.2）。
--
-- 本文件里为双方言可移植做的三处让步（都记在 SQL_DESIGN §4.2 的禁止清单里）：
--   1. 不在库里拼 JSON：原来用 JSON_OBJECT 直接产出响应体，PG 没有该函数，
--      改为取出各列由应用层组装；
--   2. 不写 FIELD()/SUM(布尔)：排序改成 CASE 表达式，状态计数改成 COUNT(CASE WHEN …)；
--   3. 不在库里合并 JSON：取消扫描时在同一事务内 SELECT … FOR UPDATE 读 summary，
--      由应用层合并后再写回。

-- name: CountBaselines :one
SELECT COUNT(*) FROM baseline
WHERE (baseline.name LIKE sqlc.narg(pattern) OR baseline.description LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListBaselines :many
SELECT id, name, version, description, enabled, create_time, update_time,
       (SELECT COUNT(*) FROM baseline_item i WHERE i.baseline_id = baseline.id) AS item_count,
       (SELECT COUNT(*) FROM security_scan sc WHERE sc.baseline_id = baseline.id AND sc.scan_type='baseline') AS scan_count
FROM baseline
WHERE (baseline.name LIKE sqlc.narg(pattern) OR baseline.description LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY id DESC
LIMIT ? OFFSET ?;

-- name: GetBaseline :one
SELECT id, name, version, description, enabled, create_time, update_time FROM baseline WHERE id = sqlc.arg(id);

-- name: CreateBaseline :execlastid
INSERT INTO baseline(create_time, update_time, name, version, description, enabled)
VALUES (?, ?, ?, ?, ?, ?);

-- name: UpdateBaseline :exec
UPDATE baseline SET update_time = ?, name = ?, version = ?, description = ?, enabled = ? WHERE id = ?;

-- name: DeleteBaseline :execrows
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
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetBaselineItem :one
SELECT i.id, i.baseline_id, i.category_id, c.name AS category, i.sort, i.name, i.description, i.config, i.severity
FROM baseline_item i
JOIN baseline_category c ON c.id = i.category_id
WHERE i.id = sqlc.arg(id);

-- name: UpdateBaselineItem :execrows
UPDATE baseline_item SET category_id = ?, name = ?, description = ?, config = ?, severity = ?, update_time = ?
WHERE id = ?;

-- 删除必须带 baseline_id：条目路由形如 /baseline/{id}/items/{itemId}/，
-- 只按 itemId 删会让跨基线的 id 也能被删掉。
-- name: DeleteBaselineItem :execrows
DELETE FROM baseline_item WHERE id = sqlc.arg(id) AND baseline_id = sqlc.arg(baseline_id);

-- name: MaxBaselineItemSort :one
SELECT CAST(COALESCE(MAX(sort), -1) AS SIGNED) AS max_sort FROM baseline_item WHERE category_id = sqlc.arg(category_id);

-- ---- 类目（策略分组实体，迁移 000011 起）----

-- name: ListBaselineCategories :many
SELECT id, baseline_id, sort, name FROM baseline_category
WHERE baseline_id = sqlc.arg(baseline_id) ORDER BY sort, id;

-- name: GetBaselineCategory :one
SELECT id, baseline_id, sort, name FROM baseline_category WHERE id = sqlc.arg(id);

-- name: MaxBaselineCategorySort :one
SELECT CAST(COALESCE(MAX(sort), -1) AS SIGNED) AS max_sort FROM baseline_category WHERE baseline_id = sqlc.arg(baseline_id);

-- name: CreateBaselineCategory :execlastid
INSERT INTO baseline_category(create_time, update_time, name, sort, baseline_id)
VALUES (?, ?, ?, ?, ?);

-- name: RenameBaselineCategory :execrows
UPDATE baseline_category SET name = ?, update_time = ? WHERE id = ?;

-- name: UpdateBaselineCategorySort :execrows
UPDATE baseline_category SET sort = ?, update_time = ? WHERE id = ?;

-- name: DeleteBaselineCategory :execrows
DELETE FROM baseline_category WHERE id = ?;

-- name: GetBaselineForScan :one
SELECT id, name, version, enabled FROM baseline WHERE id = sqlc.arg(id);

-- name: CountBaselineItems :one
SELECT COUNT(*) FROM baseline_item WHERE baseline_id = sqlc.arg(baseline_id);

-- 类目删除前的占用检查（非空禁止删）。
-- name: CountBaselineItemsByCategory :one
SELECT COUNT(*) FROM baseline_item WHERE category_id = sqlc.arg(category_id);

-- name: CreateBaselineScan :execlastid
INSERT INTO security_scan(create_time, update_time, baseline_id, mount_type, project_id, environment_id, status, summary, requested_username, start_time, end_time)
VALUES (?, ?, ?, ?, ?, ?, 'pending', '{}', ?, NULL, NULL);

-- name: ClaimBaselineScan :execrows
UPDATE security_scan SET status = 'running', start_time = ?, update_time = ?
WHERE id = ? AND status = 'pending';

-- name: CreateBaselineScanTargets :exec
INSERT INTO security_scan_target(scan_id, host_id, host_name, host_ip, instance_name_snapshot, status, error_message)
VALUES (?, ?, ?, ?, ?, 'pending', '');

-- name: GetBaselineScanTarget :one
SELECT id, host_id, host_name, host_ip, instance_name_snapshot, status, passed_items, failed_items, compliance_rate, error_message
FROM security_scan_target WHERE scan_id = ? AND host_id = ?;

-- name: FinishBaselineScanTarget :exec
UPDATE security_scan_target
SET status = ?, passed_items = ?, failed_items = ?, compliance_rate = ?, error_message = ?
WHERE id = ?;

-- 仅改状态 + 报错文案（失败/跳过两类终态），不动计数列。
-- name: SetBaselineScanTargetStatus :exec
UPDATE security_scan_target SET status = ?, error_message = ? WHERE id = ?;

-- name: CreateBaselineScanResults :exec
INSERT INTO baseline_scan_result(scan_id, host_id, item_id, item_name, chapter, severity, status, expected_value, actual_value, message, remediation)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: FinishBaselineScan :exec
UPDATE security_scan SET status = ?, summary = ?, end_time = ?, update_time = ? WHERE id = ?;

-- ---- 扫描记录列表 / 详情 / 取消 ----

-- name: CountScansByType :one
SELECT COUNT(*) FROM security_scan WHERE scan_type = sqlc.arg(scan_type);

-- name: ListScans :many
SELECT s.id, s.scan_type, b.name AS baseline, s.mount_type, s.status, s.summary,
       s.requested_username, s.start_time, s.end_time, s.create_time
FROM security_scan s JOIN baseline b ON b.id = s.baseline_id
WHERE s.scan_type = sqlc.arg(scan_type)
ORDER BY s.id DESC
LIMIT ? OFFSET ?;

-- 扫描头（不含目标与明细）。原来用 JSON_OBJECT 在库里拼响应体，PG 没有该函数，
-- 改为取出各列、由应用层组装 JSON（见 docs/architecture/SQL_DESIGN.md §4.2）。
-- name: GetScanHeader :one
SELECT s.id, s.scan_type, b.name AS baseline, s.baseline_id, s.mount_type, s.status, s.summary,
       s.requested_username, s.start_time, s.end_time
FROM security_scan s JOIN baseline b ON b.id = s.baseline_id
WHERE s.id = sqlc.arg(id);

-- name: ListScanTargets :many
SELECT host_name, host_ip, status, passed_items, failed_items, compliance_rate, error_message
FROM security_scan_target WHERE scan_id = sqlc.arg(scan_id) ORDER BY id;

-- 明细排序：原来用 FIELD(status,'fail','pass')（MySQL 专有），改成等价的 CASE 表达式，
-- 两方言都认（FIELD 未命中返回 0，所以 ELSE 分支排在最前）。
-- name: ListBaselineScanResults :many
-- expected_value/actual_value 可空，NULL 无法 Scan 进 json.RawMessage，统一回填 JSON null 字面量。
SELECT r.host_id, t.host_name, t.host_ip, r.item_name, r.chapter, r.severity, r.status,
       COALESCE(r.expected_value, 'null') AS expected_value,
       COALESCE(r.actual_value, 'null') AS actual_value,
       r.message, COALESCE(r.remediation, '') AS remediation
FROM baseline_scan_result r
JOIN security_scan_target t ON t.scan_id = r.scan_id AND t.host_id = r.host_id
WHERE r.scan_id = sqlc.arg(scan_id)
ORDER BY t.id, CASE r.status WHEN 'fail' THEN 1 WHEN 'pass' THEN 2 ELSE 0 END, r.severity, r.id;

-- 终态聚合：不能用 SUM(status='success')——MySQL 把布尔当 0/1，PG 不接受；
-- COUNT(CASE WHEN ... THEN 1 END) 两方言都是 bigint，且零行时为 0（SUM 会返回 NULL）。
-- name: CountScanTargetsByStatus :one
SELECT COUNT(CASE WHEN status = 'success' THEN 1 END) AS success,
       COUNT(CASE WHEN status = 'failed' THEN 1 END) AS failed,
       COUNT(CASE WHEN status = 'skipped' THEN 1 END) AS skipped
FROM security_scan_target WHERE scan_id = sqlc.arg(scan_id);

-- 取消前锁行读状态：同一事务内读改写 summary，避免 JSON_MERGE_PATCH 这类方言函数。
-- name: GetScanStatusForUpdate :one
SELECT status, summary FROM security_scan WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: CancelBaselineScan :execrows
UPDATE security_scan SET status = 'canceled', summary = ?, end_time = ?, update_time = ?
WHERE id = ?;

-- name: CancelBaselineScanTargets :exec
UPDATE security_scan_target SET status = 'canceled', error_message = '扫描被取消'
WHERE scan_id = sqlc.arg(scan_id) AND status IN ('pending', 'running');

-- ---- 基线删除时级联清理扫描历史（外键未设级联，须先删子表：明细 → 目标 → 扫描记录）----

-- name: DeleteBaselineScanResults :exec
DELETE FROM baseline_scan_result
WHERE scan_id IN (SELECT id FROM security_scan WHERE baseline_id = sqlc.arg(baseline_id));

-- name: DeleteBaselineScanTargets :exec
DELETE FROM security_scan_target
WHERE scan_id IN (SELECT id FROM security_scan WHERE baseline_id = sqlc.arg(baseline_id));

-- name: DeleteBaselineScans :exec
DELETE FROM security_scan WHERE baseline_id = sqlc.arg(baseline_id);
