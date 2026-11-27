-- name: RecordMaintenanceSuppression :exec
-- conflict: event_id, window_id
INSERT INTO monitor_maintenance_suppression (event_id,window_id,title,ends_at,decided_at)
VALUES (sqlc.arg(event_id),sqlc.arg(window_id),sqlc.arg(title),sqlc.arg(ends_at),sqlc.arg(decided_at))
ON DUPLICATE KEY UPDATE window_id=VALUES(window_id);

-- name: ListMaintenanceWindowSuppressionCounts :many
SELECT window_id, COUNT(*) AS suppressed_count FROM monitor_maintenance_suppression
WHERE window_id IN (sqlc.slice(window_ids)) GROUP BY window_id;

-- name: ListEventMaintenanceDecisions :many
SELECT window_id, title, ends_at, decided_at FROM monitor_maintenance_suppression
WHERE event_id = sqlc.arg(event_id) ORDER BY decided_at,window_id;

-- name: CountFiringMaintenanceDecisions :one
SELECT COUNT(*) FROM monitor_maintenance_suppression s JOIN monitor_alert_notification_event e ON e.id=s.event_id
WHERE e.alert_id=sqlc.arg(alert_id) AND e.event_type='firing';

-- name: ListMaintenanceRecoveryRecipients :many
SELECT d.user_id,d.media_id,d.address
FROM monitor_alert_notification_delivery d JOIN monitor_alert_notification_event e ON e.id=d.event_id
WHERE e.alert_id=sqlc.arg(alert_id) AND e.event_type='firing' AND d.status='success'
ORDER BY d.id;

-- name: MarkMaintenanceDecision :exec
UPDATE monitor_alert_notification_event SET status=sqlc.arg(status),error_message=sqlc.arg(error_message),
update_time=sqlc.arg(update_time) WHERE id=sqlc.arg(id);

-- name: SuppressOutstandingDeliveries :exec
UPDATE monitor_alert_notification_delivery SET status='suppressed',error_message='',update_time=sqlc.arg(update_time)
WHERE event_id=sqlc.arg(event_id) AND status IN ('pending','failed');

-- name: CountSuccessfulEventDeliveries :one
SELECT COUNT(*) FROM monitor_alert_notification_delivery WHERE event_id=sqlc.arg(event_id) AND status='success';

-- name: ListUnfinishedNotificationIDs :many
SELECT id FROM monitor_alert_notification_event
WHERE id>sqlc.arg(after_id) AND (status IN ('suppressed','partial_muted') OR
  (status IN ('pending','sending') AND update_time<=sqlc.arg(stale_before)))
ORDER BY id LIMIT 200;

-- name: RecordNotificationIntent :exec
INSERT INTO monitor_notification_intent (alert_id,event_type) VALUES (sqlc.arg(alert_id),sqlc.arg(event_type));

-- name: CompleteNotificationIntent :exec
DELETE FROM monitor_notification_intent WHERE alert_id=sqlc.arg(alert_id) AND event_type=sqlc.arg(event_type);

-- name: ListNotificationIntents :many
SELECT i.alert_id,i.event_type,h.alertname,h.severity,h.instance,h.labels FROM monitor_notification_intent i
JOIN monitor_alert_history h ON h.id=i.alert_id
WHERE i.alert_id>sqlc.arg(after_id) OR (i.alert_id=sqlc.arg(after_id) AND i.event_type>sqlc.arg(after_type))
ORDER BY i.alert_id,i.event_type LIMIT 200;
