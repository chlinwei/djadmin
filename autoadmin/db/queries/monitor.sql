-- name: CountAlertRoutes :one
SELECT COUNT(*) FROM monitor_alert_route;

-- name: ListAlertRoutes :many
SELECT id, create_time, update_time, remark, name, enabled, matchers, notify_on_firing, notify_on_resolved
FROM monitor_alert_route
ORDER BY id
LIMIT ? OFFSET ?;

-- name: GetAlertRoute :one
SELECT id, create_time, update_time, remark, name, enabled, matchers, notify_on_firing, notify_on_resolved
FROM monitor_alert_route
WHERE id = ?
LIMIT 1;