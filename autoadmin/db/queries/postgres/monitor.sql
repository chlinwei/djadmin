-- 本文件由 make derive 从 db/queries/mysql/monitor.sql 派生，禁止手改。
-- 修改请改 MySQL 源后重新派生；规则见 internal/platform/database/derive 与
-- docs/architecture/SQL_DESIGN.md §4.6。

-- name: ListMonitorTargetsByHost :many
-- managed_enabled 是 monitor_target 表里 TINYINT(1) 列，schema 里已按约定标成 BOOLEAN，
-- 这里生成的 struct 字段就是真正的 Go bool，取代 monitor 包里手写的 map[string]any 扫描。
SELECT id, exporter_type, scrape_port, managed_enabled, install_status, install_message, last_scrape_status
FROM monitor_target
WHERE host_id = sqlc.arg(host_id)
  AND (exporter_type = sqlc.narg(exporter_type) OR sqlc.narg(exporter_type) IS NULL)
ORDER BY exporter_type;

-- name: CountMonitorTargets :one
SELECT COUNT(*)
FROM monitor_target t
JOIN assets_host h ON h.id = t.host_id
WHERE (t.exporter_type = sqlc.narg(exporter_type) OR sqlc.narg(exporter_type) IS NULL)
  AND (t.managed_enabled = sqlc.narg(managed_enabled) OR sqlc.narg(managed_enabled) IS NULL)
  AND (t.install_status = sqlc.narg(install_status) OR sqlc.narg(install_status) IS NULL)
  AND (t.last_scrape_status = sqlc.narg(last_scrape_status) OR sqlc.narg(last_scrape_status) IS NULL)
  AND (h.instance_name LIKE sqlc.narg(search_pattern) OR h.ip LIKE sqlc.narg(search_pattern) OR t.exporter_type LIKE sqlc.narg(search_pattern) OR sqlc.narg(search_pattern) IS NULL);

-- name: ListMonitorTargets :many
SELECT t.id, t.create_time, t.update_time, t.remark, t.exporter_type, t.managed_enabled,
       t.install_status, t.install_message, t.last_scrape_status, t.last_scrape_at, t.labels,
       t.host_id, t.retry_count, t.last_dispatch_manual, t.scrape_port,
       'exporter' AS target_type, h.instance_name AS host_name, h.ip AS host_ip,
       h.agent_online AS host_agent_online
FROM monitor_target t
JOIN assets_host h ON h.id = t.host_id
WHERE (t.exporter_type = sqlc.narg(exporter_type) OR sqlc.narg(exporter_type) IS NULL)
  AND (t.managed_enabled = sqlc.narg(managed_enabled) OR sqlc.narg(managed_enabled) IS NULL)
  AND (t.install_status = sqlc.narg(install_status) OR sqlc.narg(install_status) IS NULL)
  AND (t.last_scrape_status = sqlc.narg(last_scrape_status) OR sqlc.narg(last_scrape_status) IS NULL)
  AND (h.instance_name LIKE sqlc.narg(search_pattern) OR h.ip LIKE sqlc.narg(search_pattern) OR t.exporter_type LIKE sqlc.narg(search_pattern) OR sqlc.narg(search_pattern) IS NULL)
ORDER BY t.id DESC
LIMIT $1 OFFSET $2;

-- name: GetMonitorTarget :one
SELECT t.id, t.create_time, t.update_time, t.remark, t.exporter_type, t.managed_enabled,
       t.install_status, t.install_message, t.last_scrape_status, t.last_scrape_at, t.labels,
       t.host_id, t.retry_count, t.last_dispatch_manual, t.scrape_port,
       'exporter' AS target_type, h.instance_name AS host_name, h.ip AS host_ip,
       h.agent_online AS host_agent_online
FROM monitor_target t
JOIN assets_host h ON h.id = t.host_id
WHERE t.id = sqlc.arg(id)
LIMIT 1;

-- name: ListExporterPackagePorts :many
SELECT name, default_port
FROM monitor_software_package
WHERE package_type = 'exporter' AND enabled = TRUE
ORDER BY name, default_port;

-- name: CountLogRetentionTiers :one
SELECT COUNT(*) FROM monitor_log_retention_tier
WHERE (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_default = sqlc.narg(is_default) OR sqlc.narg(is_default) IS NULL)
  AND (code LIKE sqlc.narg(pattern) OR name LIKE sqlc.narg(pattern) OR remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListLogRetentionTiers :many
SELECT id, create_time, update_time, code, name, daily_size_gb, retention_days, rollover_min_index_age, enabled, is_default, remark
FROM monitor_log_retention_tier
WHERE (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_default = sqlc.narg(is_default) OR sqlc.narg(is_default) IS NULL)
  AND (code LIKE sqlc.narg(pattern) OR name LIKE sqlc.narg(pattern) OR remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY retention_days, id
LIMIT $1 OFFSET $2;

-- name: GetLogRetentionTier :one
SELECT id, create_time, update_time, code, name, daily_size_gb, retention_days, rollover_min_index_age, enabled, is_default, remark
FROM monitor_log_retention_tier
WHERE id = sqlc.arg(id);

-- name: CountOpenSearchClusters :one
SELECT COUNT(*) FROM monitor_opensearch_cluster
WHERE (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_default = sqlc.narg(is_default) OR sqlc.narg(is_default) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR hosts LIKE sqlc.narg(pattern) OR remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListOpenSearchClustersTyped :many
SELECT id, create_time, update_time, name, hosts, username, password, verify_tls, ca_cert, index_prefix,
       request_timeout, enabled, is_default, last_check_time, last_check_success, last_check_message,
       remark, storage_sync_error, storage_sync_status, storage_sync_time
FROM monitor_opensearch_cluster
WHERE (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_default = sqlc.narg(is_default) OR sqlc.narg(is_default) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR hosts LIKE sqlc.narg(pattern) OR remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY is_default DESC, id ASC
LIMIT $1 OFFSET $2;

-- name: GetOpenSearchClusterTyped :one
SELECT id, create_time, update_time, name, hosts, username, password, verify_tls, ca_cert, index_prefix,
       request_timeout, enabled, is_default, last_check_time, last_check_success, last_check_message,
       remark, storage_sync_error, storage_sync_status, storage_sync_time
FROM monitor_opensearch_cluster
WHERE id = sqlc.arg(id);

-- name: CountLogProcessingRules :one
SELECT COUNT(*) FROM monitor_log_processing_rule
WHERE (cluster_id = sqlc.narg(cluster_id) OR sqlc.narg(cluster_id) IS NULL)
  AND (application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND (input_format = sqlc.narg(input_format) OR sqlc.narg(input_format) IS NULL)
  AND (multiline_enabled = sqlc.narg(multiline_enabled) OR sqlc.narg(multiline_enabled) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListLogProcessingRules :many
SELECT id, create_time, update_time, remark, name, description, input_format, multiline_enabled, start_pattern,
       continuation_pattern, flush_timeout, pipeline_body, cluster_id, application_id
FROM monitor_log_processing_rule
WHERE (cluster_id = sqlc.narg(cluster_id) OR sqlc.narg(cluster_id) IS NULL)
  AND (application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND (input_format = sqlc.narg(input_format) OR sqlc.narg(input_format) IS NULL)
  AND (multiline_enabled = sqlc.narg(multiline_enabled) OR sqlc.narg(multiline_enabled) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY name, id
LIMIT $1 OFFSET $2;

-- name: GetLogProcessingRule :one
SELECT id, create_time, update_time, remark, name, description, input_format, multiline_enabled, start_pattern,
       continuation_pattern, flush_timeout, pipeline_body, cluster_id, application_id
FROM monitor_log_processing_rule
WHERE id = sqlc.arg(id);

-- name: CountLogCollectionFilterRules :one
SELECT COUNT(*) FROM monitor_log_collection_filter_rule
WHERE (application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (name LIKE sqlc.narg(search_pattern) OR description LIKE sqlc.narg(search_pattern) OR pattern LIKE sqlc.narg(search_pattern) OR sqlc.narg(search_pattern) IS NULL);

-- name: ListLogCollectionFilterRules :many
SELECT id, create_time, update_time, remark, name, description, pattern, enabled, application_id
FROM monitor_log_collection_filter_rule
WHERE (application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (name LIKE sqlc.narg(search_pattern) OR description LIKE sqlc.narg(search_pattern) OR pattern LIKE sqlc.narg(search_pattern) OR sqlc.narg(search_pattern) IS NULL)
ORDER BY name, id
LIMIT $1 OFFSET $2;

-- name: GetLogCollectionFilterRule :one
SELECT id, create_time, update_time, remark, name, description, pattern, enabled, application_id
FROM monitor_log_collection_filter_rule
WHERE id = sqlc.arg(id);

-- name: CountAlertMedia :one
SELECT COUNT(*) FROM monitor_alert_media
WHERE (media_type = sqlc.narg(media_type) OR sqlc.narg(media_type) IS NULL)
  AND (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListAlertMedia :many
SELECT id, create_time, update_time, remark, name, media_type, config, enabled, recipients
FROM monitor_alert_media
WHERE (media_type = sqlc.narg(media_type) OR sqlc.narg(media_type) IS NULL)
  AND (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY id DESC
LIMIT $1 OFFSET $2;

-- name: GetAlertMediaTyped :one
SELECT id, create_time, update_time, remark, name, media_type, config, enabled, recipients
FROM monitor_alert_media
WHERE id = sqlc.arg(id);

-- name: CountSoftwarePackages :one
SELECT COUNT(*) FROM monitor_software_package p
WHERE (p.package_type = sqlc.narg(package_type) OR sqlc.narg(package_type) IS NULL)
  AND (p.name = sqlc.narg(name) OR sqlc.narg(name) IS NULL)
  AND (p.version = sqlc.narg(version) OR sqlc.narg(version) IS NULL)
  AND (p.os = sqlc.narg(os) OR sqlc.narg(os) IS NULL)
  AND (p.arch = sqlc.narg(arch) OR sqlc.narg(arch) IS NULL)
  AND (p.enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (p.name LIKE sqlc.narg(pattern) OR p.version LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListSoftwarePackages :many
SELECT p.id, p.create_time, p.update_time, p.remark, p.name, p.version, p.os, p.arch, p.file, p.sha256,
       p.size_bytes, p.enabled, p.service_file_content, p.service_run_as_group, p.service_run_as_user,
       p.install_playbook_template_id, p.uninstall_playbook_template_id, p.work_directory, p.default_port,
       p.package_format, p.platform_family, p.platform_major, p.package_type,
       COALESCE(i.name,'') AS install_playbook_template_name, COALESCE(i.content,'') AS install_playbook_content,
       COALESCE(u.name,'') AS uninstall_playbook_template_name, COALESCE(u.content,'') AS uninstall_playbook_content
FROM monitor_software_package p
LEFT JOIN automation_playbook_template i ON i.id = p.install_playbook_template_id
LEFT JOIN automation_playbook_template u ON u.id = p.uninstall_playbook_template_id
WHERE (p.package_type = sqlc.narg(package_type) OR sqlc.narg(package_type) IS NULL)
  AND (p.name = sqlc.narg(name) OR sqlc.narg(name) IS NULL)
  AND (p.version = sqlc.narg(version) OR sqlc.narg(version) IS NULL)
  AND (p.os = sqlc.narg(os) OR sqlc.narg(os) IS NULL)
  AND (p.arch = sqlc.narg(arch) OR sqlc.narg(arch) IS NULL)
  AND (p.enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (p.name LIKE sqlc.narg(pattern) OR p.version LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY p.id DESC
LIMIT $1 OFFSET $2;

-- name: GetSoftwarePackageTyped :one
SELECT p.id, p.create_time, p.update_time, p.remark, p.name, p.version, p.os, p.arch, p.file, p.sha256,
       p.size_bytes, p.enabled, p.service_file_content, p.service_run_as_group, p.service_run_as_user,
       p.install_playbook_template_id, p.uninstall_playbook_template_id, p.work_directory, p.default_port,
       p.package_format, p.platform_family, p.platform_major, p.package_type,
       COALESCE(i.name,'') AS install_playbook_template_name, COALESCE(i.content,'') AS install_playbook_content,
       COALESCE(u.name,'') AS uninstall_playbook_template_name, COALESCE(u.content,'') AS uninstall_playbook_content
FROM monitor_software_package p
LEFT JOIN automation_playbook_template i ON i.id = p.install_playbook_template_id
LEFT JOIN automation_playbook_template u ON u.id = p.uninstall_playbook_template_id
WHERE p.id = sqlc.arg(id);

-- name: CountInstallHistories :one
SELECT COUNT(*) FROM monitor_target_install_history ih
LEFT JOIN assets_host h ON h.id = ih.host_id
LEFT JOIN monitor_target mt ON mt.id = ih.target_id
WHERE (ih.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (ih.target_id = sqlc.narg(target_id) OR sqlc.narg(target_id) IS NULL)
  AND (ih.log_collection_target_id = sqlc.narg(log_collection_target_id) OR sqlc.narg(log_collection_target_id) IS NULL)
  AND (ih.action = sqlc.narg(action) OR sqlc.narg(action) IS NULL)
  AND (ih.trigger_type = sqlc.narg(trigger_type) OR sqlc.narg(trigger_type) IS NULL)
  AND (ih.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (ih.host_name_snapshot LIKE sqlc.narg(keyword) OR ih.host_ip_snapshot LIKE sqlc.narg(keyword) OR ih.exporter_type_snapshot LIKE sqlc.narg(keyword) OR ih.summary_message LIKE sqlc.narg(keyword) OR sqlc.narg(keyword) IS NULL)
  AND (ih.create_time >= sqlc.narg(start_time) OR sqlc.narg(start_time) IS NULL)
  AND (ih.create_time <= sqlc.narg(end_time) OR sqlc.narg(end_time) IS NULL);

-- name: ListInstallHistories :many
SELECT ih.id, ih.create_time, ih.update_time, ih.remark, ih.action, ih.trigger_type, ih.status,
       ih.host_id_snapshot, ih.host_name_snapshot, ih.host_ip_snapshot, ih.exporter_type_snapshot,
       ih.summary_message, ih.stdout_snapshot, ih.stderr_snapshot, ih.error_message_snapshot,
       ih.result_summary_snapshot, ih.requested_user_id_snapshot, ih.requested_username_snapshot,
       ih.start_time, ih.end_time, ih.duration_seconds, ih.host_id, ih.target_id, ih.log_collection_target_id,
       COALESCE(h.instance_name, ih.host_name_snapshot) AS host_name,
       COALESCE(h.ip, ih.host_ip_snapshot) AS host_ip,
       COALESCE(mt.exporter_type, ih.exporter_type_snapshot) AS target_exporter_type,
       COALESCE(ih.target_id, ih.log_collection_target_id) AS managed_target_id,
       CASE WHEN ih.log_collection_target_id IS NULL THEN 'exporter' ELSE 'fluent_bit' END AS target_type
FROM monitor_target_install_history ih
LEFT JOIN assets_host h ON h.id = ih.host_id
LEFT JOIN monitor_target mt ON mt.id = ih.target_id
WHERE (ih.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (ih.target_id = sqlc.narg(target_id) OR sqlc.narg(target_id) IS NULL)
  AND (ih.log_collection_target_id = sqlc.narg(log_collection_target_id) OR sqlc.narg(log_collection_target_id) IS NULL)
  AND (ih.action = sqlc.narg(action) OR sqlc.narg(action) IS NULL)
  AND (ih.trigger_type = sqlc.narg(trigger_type) OR sqlc.narg(trigger_type) IS NULL)
  AND (ih.status = sqlc.narg(status) OR sqlc.narg(status) IS NULL)
  AND (ih.host_name_snapshot LIKE sqlc.narg(keyword) OR ih.host_ip_snapshot LIKE sqlc.narg(keyword) OR ih.exporter_type_snapshot LIKE sqlc.narg(keyword) OR ih.summary_message LIKE sqlc.narg(keyword) OR sqlc.narg(keyword) IS NULL)
  AND (ih.create_time >= sqlc.narg(start_time) OR sqlc.narg(start_time) IS NULL)
  AND (ih.create_time <= sqlc.narg(end_time) OR sqlc.narg(end_time) IS NULL)
ORDER BY ih.id DESC
LIMIT $1 OFFSET $2;

-- name: CountAlertHistories :one
SELECT COUNT(*) FROM monitor_alert_history ah
WHERE (ah.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (ah.state = sqlc.narg(state) OR sqlc.narg(state) IS NULL)
  AND (ah.severity = sqlc.narg(severity) OR sqlc.narg(severity) IS NULL)
  AND (ah.alertname LIKE sqlc.narg(keyword) OR ah.instance LIKE sqlc.narg(keyword) OR sqlc.narg(keyword) IS NULL)
  AND (ah.started_at >= sqlc.narg(start_time) OR sqlc.narg(start_time) IS NULL)
  AND (ah.started_at <= sqlc.narg(end_time) OR sqlc.narg(end_time) IS NULL)
  AND (ah.labels->>CAST(sqlc.arg(label_key) AS text) = CAST(sqlc.narg(label_value) AS text) OR sqlc.narg(label_value) IS NULL);

-- name: ListAlertHistories :many
SELECT ah.id, ah.create_time, ah.update_time, ah.remark, ah.fingerprint, ah.alertname, ah.severity, ah.instance,
       ah.labels, ah.annotations, ah.generator_url, ah.state, ah.started_at, ah.resolved_at, ah.last_seen_at,
       ah.resolved_by_reconciliation, ah.rule_group, ah.rule_snapshot, ah.source,
       (SELECT COUNT(*) FROM monitor_alert_notification_event ne WHERE ne.alert_id = ah.id) AS notification_count,
       (SELECT COUNT(*) FROM monitor_alert_notification_delivery nd JOIN monitor_alert_notification_event ne ON ne.id = nd.event_id WHERE ne.alert_id = ah.id) AS notification_delivery_count,
       (SELECT COUNT(*) FROM monitor_alert_notification_event ne WHERE ne.alert_id = ah.id AND ne.status = 'failed') AS notification_failed_count,
       (SELECT COUNT(*) FROM monitor_alert_notification_event ne WHERE ne.alert_id = ah.id AND ne.status IN ('pending','sending')) AS notification_active_count
FROM monitor_alert_history ah
WHERE (ah.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (ah.state = sqlc.narg(state) OR sqlc.narg(state) IS NULL)
  AND (ah.severity = sqlc.narg(severity) OR sqlc.narg(severity) IS NULL)
  AND (ah.alertname LIKE sqlc.narg(keyword) OR ah.instance LIKE sqlc.narg(keyword) OR sqlc.narg(keyword) IS NULL)
  AND (ah.started_at >= sqlc.narg(start_time) OR sqlc.narg(start_time) IS NULL)
  AND (ah.started_at <= sqlc.narg(end_time) OR sqlc.narg(end_time) IS NULL)
  AND (ah.labels->>CAST(sqlc.arg(label_key) AS text) = CAST(sqlc.narg(label_value) AS text) OR sqlc.narg(label_value) IS NULL)
ORDER BY ah.started_at DESC, ah.id DESC
LIMIT $1 OFFSET $2;

-- name: GetAlertHistoryAlertnameInstance :one
SELECT alertname, instance FROM monitor_alert_history WHERE id = sqlc.arg(id);

-- name: ListAlertNotificationEvents :many
SELECT id, create_time, update_time, remark, event_type, deduplication_key, status, attempt_count,
       error_message, sent_at, alert_id
FROM monitor_alert_notification_event
WHERE alert_id = sqlc.arg(alert_id)
ORDER BY create_time DESC, id DESC;

-- name: ListAlertNotificationDeliveries :many
SELECT d.id, d.user_id, COALESCE(u.username,'-') AS username, d.media_id, COALESCE(m.name,'-') AS media_name,
       COALESCE(m.media_type,'-') AS media_type, d.address, d.status, d.attempt_count, d.error_message,
       d.sent_at, d.create_time
FROM monitor_alert_notification_delivery d
LEFT JOIN sys_user u ON u.id = d.user_id
LEFT JOIN monitor_alert_media m ON m.id = d.media_id
WHERE d.event_id = sqlc.arg(event_id)
ORDER BY d.id;