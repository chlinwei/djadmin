-- name: ListLogFormatServices :many
SELECT s.id FROM assets_application_service s
WHERE s.log_collection_enabled = TRUE
  AND NOT EXISTS (SELECT 1 FROM assets_service_deletion deleted WHERE deleted.service_id = s.id)
ORDER BY s.id;

-- name: ListLogFormatTargets :many
SELECT * FROM assets_log_format_target WHERE service_id = sqlc.arg(service_id)
ORDER BY log_definition_id, deployment_id;

-- name: ListDueLogFormatTargets :many
SELECT target.* FROM assets_log_format_target target
JOIN assets_application_service s ON s.id = target.service_id
JOIN assets_application_service_deployment sd ON sd.service_id = s.id AND sd.deployment_id = target.deployment_id
JOIN assets_application_log_definition ld ON ld.id = target.log_definition_id AND ld.deployment_template_id = s.deployment_template_id
LEFT JOIN assets_application_service_log_setting ls ON ls.service_id = s.id AND ls.log_definition_id = ld.id
WHERE target.status IN ('pending', 'waiting', 'running') AND target.attempts < 4
 AND target.next_attempt_at <= sqlc.arg(now) AND s.log_collection_enabled = TRUE
 AND COALESCE(ls.collection_enabled, TRUE) = TRUE AND ld.processing_rule_id IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM assets_service_deletion deleted WHERE deleted.service_id = s.id)
ORDER BY target.next_attempt_at, target.service_id, target.log_definition_id, target.deployment_id
LIMIT 4;

-- name: EnsureLogFormatTarget :exec
-- conflict: service_id, log_definition_id, deployment_id
INSERT INTO assets_log_format_target
 (service_id, log_definition_id, deployment_id, fingerprint, status, next_attempt_at, report, reason, update_time)
VALUES (sqlc.arg(service_id), sqlc.arg(log_definition_id), sqlc.arg(deployment_id), sqlc.arg(fingerprint),
 'pending', sqlc.arg(next_attempt_at), JSON_OBJECT(), '', sqlc.arg(update_time))
ON DUPLICATE KEY UPDATE service_id = VALUES(service_id);

-- name: ResetChangedLogFormatTarget :exec
UPDATE assets_log_format_target SET fingerprint = sqlc.arg(fingerprint), status = 'pending', attempts = 0,
 next_attempt_at = sqlc.arg(next_attempt_at), lease_token = '', report = JSON_OBJECT(), actor = '', reason = '',
 update_time = sqlc.arg(update_time)
WHERE service_id = sqlc.arg(service_id) AND log_definition_id = sqlc.arg(log_definition_id)
 AND deployment_id = sqlc.arg(deployment_id) AND fingerprint <> sqlc.arg(old_fingerprint);

-- name: ClaimLogFormatTarget :execresult
UPDATE assets_log_format_target SET status = 'running', attempts = attempts + 1,
 lease_token = sqlc.arg(lease_token), next_attempt_at = sqlc.arg(lease_until), update_time = sqlc.arg(update_time)
WHERE service_id = sqlc.arg(service_id) AND log_definition_id = sqlc.arg(log_definition_id)
 AND deployment_id = sqlc.arg(deployment_id) AND fingerprint = sqlc.arg(fingerprint)
 AND attempts < 4 AND next_attempt_at <= sqlc.arg(now)
 AND status IN ('pending', 'waiting', 'running');

-- name: FinishLogFormatTarget :execresult
UPDATE assets_log_format_target SET status = sqlc.arg(status), report = sqlc.arg(report),
 actor = sqlc.arg(actor), reason = sqlc.arg(reason), next_attempt_at = sqlc.arg(next_attempt_at),
 lease_token = '', update_time = sqlc.arg(update_time)
WHERE service_id = sqlc.arg(service_id) AND log_definition_id = sqlc.arg(log_definition_id)
 AND deployment_id = sqlc.arg(deployment_id) AND fingerprint = sqlc.arg(fingerprint)
 AND lease_token = sqlc.arg(lease_token);

-- name: RequestLogFormatTarget :exec
UPDATE assets_log_format_target SET status = sqlc.arg(status), attempts = 0,
 next_attempt_at = sqlc.arg(next_attempt_at), lease_token = '', report = JSON_OBJECT(),
 actor = sqlc.arg(actor), reason = sqlc.arg(reason), update_time = sqlc.arg(update_time)
WHERE service_id = sqlc.arg(service_id) AND log_definition_id = sqlc.arg(log_definition_id)
 AND deployment_id = sqlc.arg(deployment_id) AND fingerprint = sqlc.arg(fingerprint);

-- name: DeleteInactiveLogFormatTargets :exec
DELETE FROM assets_log_format_target WHERE NOT EXISTS (
 SELECT 1 FROM assets_application_service s
 JOIN assets_application_service_deployment sd ON sd.service_id = s.id
 JOIN assets_application_log_definition ld ON ld.deployment_template_id = s.deployment_template_id
 LEFT JOIN assets_application_service_log_setting ls ON ls.service_id = s.id AND ls.log_definition_id = ld.id
 WHERE s.id = assets_log_format_target.service_id AND ld.id = assets_log_format_target.log_definition_id
 AND sd.deployment_id = assets_log_format_target.deployment_id AND ld.processing_rule_id IS NOT NULL
 AND NOT EXISTS (SELECT 1 FROM assets_service_deletion deleted WHERE deleted.service_id = s.id)
);

-- name: RecoverExhaustedLogFormatTargets :exec
UPDATE assets_log_format_target SET status = 'waiting', lease_token = '',
 reason = '认证执行中断，自动重试已用尽，请手动重新认证', update_time = sqlc.arg(update_time)
WHERE status = 'running' AND attempts >= 4 AND next_attempt_at <= sqlc.arg(now);