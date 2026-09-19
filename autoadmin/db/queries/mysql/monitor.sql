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
LIMIT ? OFFSET ?;

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
LIMIT ? OFFSET ?;

-- name: GetLogRetentionTier :one
SELECT id, create_time, update_time, code, name, daily_size_gb, retention_days, rollover_min_index_age, enabled, is_default, remark
FROM monitor_log_retention_tier
WHERE id = sqlc.arg(id);

-- name: CountElasticsearchClusters :one
SELECT COUNT(*) FROM monitor_elasticsearch_cluster
WHERE (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_default = sqlc.narg(is_default) OR sqlc.narg(is_default) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR hosts LIKE sqlc.narg(pattern) OR remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL);

-- name: ListElasticsearchClustersTyped :many
SELECT id, create_time, update_time, name, hosts, username, password, verify_tls, ca_cert, index_prefix,
       request_timeout, enabled, is_default, last_check_time, last_check_success, last_check_message,
       remark, storage_sync_error, storage_sync_status, storage_sync_time
FROM monitor_elasticsearch_cluster
WHERE (enabled = sqlc.narg(enabled) OR sqlc.narg(enabled) IS NULL)
  AND (is_default = sqlc.narg(is_default) OR sqlc.narg(is_default) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR hosts LIKE sqlc.narg(pattern) OR remark LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY is_default DESC, id ASC
LIMIT ? OFFSET ?;

-- name: GetElasticsearchClusterTyped :one
SELECT id, create_time, update_time, name, hosts, username, password, verify_tls, ca_cert, index_prefix,
       request_timeout, enabled, is_default, last_check_time, last_check_success, last_check_message,
       remark, storage_sync_error, storage_sync_status, storage_sync_time
FROM monitor_elasticsearch_cluster
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
       continuation_pattern, sample_log, flush_timeout, pipeline_body, cluster_id, application_id
FROM monitor_log_processing_rule
WHERE (cluster_id = sqlc.narg(cluster_id) OR sqlc.narg(cluster_id) IS NULL)
  AND (application_id = sqlc.narg(application_id) OR sqlc.narg(application_id) IS NULL)
  AND (input_format = sqlc.narg(input_format) OR sqlc.narg(input_format) IS NULL)
  AND (multiline_enabled = sqlc.narg(multiline_enabled) OR sqlc.narg(multiline_enabled) IS NULL)
  AND (name LIKE sqlc.narg(pattern) OR description LIKE sqlc.narg(pattern) OR sqlc.narg(pattern) IS NULL)
ORDER BY name, id
LIMIT ? OFFSET ?;

-- name: GetLogProcessingRule :one
SELECT id, create_time, update_time, remark, name, description, input_format, multiline_enabled, start_pattern,
       continuation_pattern, sample_log, flush_timeout, pipeline_body, cluster_id, application_id
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
LIMIT ? OFFSET ?;

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
LIMIT ? OFFSET ?;

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
LIMIT ? OFFSET ?;

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
       ih.automation_job_id_snapshot,
       COALESCE(h.instance_name, ih.host_name_snapshot) AS host_name,
       COALESCE(h.ip, ih.host_ip_snapshot) AS host_ip,
       COALESCE(mt.exporter_type, ih.exporter_type_snapshot) AS target_exporter_type,
       COALESCE(ih.target_id, ih.log_collection_target_id) AS managed_target_id,
       CASE WHEN ih.log_collection_target_id IS NULL THEN 'exporter' ELSE 'filebeat' END AS target_type
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
LIMIT ? OFFSET ?;

-- name: CountAlertHistories :one
SELECT COUNT(*) FROM monitor_alert_history ah
WHERE (ah.id = sqlc.narg(id) OR sqlc.narg(id) IS NULL)
  AND (ah.state = sqlc.narg(state) OR sqlc.narg(state) IS NULL)
  AND (ah.severity = sqlc.narg(severity) OR sqlc.narg(severity) IS NULL)
  AND (ah.alertname LIKE sqlc.narg(keyword) OR ah.instance LIKE sqlc.narg(keyword) OR sqlc.narg(keyword) IS NULL)
  AND (ah.started_at >= sqlc.narg(start_time) OR sqlc.narg(start_time) IS NULL)
  AND (ah.started_at <= sqlc.narg(end_time) OR sqlc.narg(end_time) IS NULL)
  AND (JSON_UNQUOTE(JSON_EXTRACT(ah.labels, CONCAT('$.', sqlc.arg(label_key)))) = CAST(sqlc.narg(label_value) AS CHAR) OR sqlc.narg(label_value) IS NULL);

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
  AND (JSON_UNQUOTE(JSON_EXTRACT(ah.labels, CONCAT('$.', sqlc.arg(label_key)))) = CAST(sqlc.narg(label_value) AS CHAR) OR sqlc.narg(label_value) IS NULL)
ORDER BY ah.started_at DESC, ah.id DESC
LIMIT ? OFFSET ?;

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
-- ---- P2-3：监控软件包管理（读取复用 GetSoftwarePackageTyped；写路径见下）----
-- 原实现有运行时拼列名的地方（`SET `+role+`_playbook_template_id=…`），改成按角色分派的显式语句。

-- name: CountSoftwarePackageSyncConflict :one
SELECT COUNT(*) FROM monitor_software_package
WHERE name=sqlc.arg(name) AND version=sqlc.arg(version) AND os=sqlc.arg(os) AND arch=sqlc.arg(arch)
  AND platform_family=sqlc.arg(platform_family) AND platform_major=sqlc.arg(platform_major)
  AND id<>sqlc.arg(exclude_id);

-- name: CountSoftwarePackageVariantConflict :one
SELECT COUNT(*) FROM monitor_software_package
WHERE package_type=sqlc.arg(package_type) AND name=sqlc.arg(name) AND version=sqlc.arg(version)
  AND os=sqlc.arg(os) AND arch=sqlc.arg(arch) AND platform_family=sqlc.arg(platform_family)
  AND platform_major=sqlc.arg(platform_major) AND id<>sqlc.arg(exclude_id);

-- name: CreateSoftwarePackage :execlastid
INSERT INTO monitor_software_package(create_time,update_time,remark,package_type,name,version,default_port,
                                     os,arch,platform_family,platform_major,package_format,file,sha256,size_bytes,
                                     enabled,work_directory,service_file_content,service_run_as_user,service_run_as_group)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(package_type),sqlc.arg(name),sqlc.arg(version),
        sqlc.arg(default_port),sqlc.arg(os),sqlc.arg(arch),sqlc.arg(platform_family),sqlc.arg(platform_major),
        sqlc.arg(package_format),'', '', 0, TRUE, '/tmp', sqlc.narg(service_file_content), sqlc.arg(service_run_as_user), 'dj-agent');

-- name: UpdateSoftwarePackageSource :exec
UPDATE monitor_software_package
SET version=sqlc.arg(version),file=sqlc.arg(file),sha256=sqlc.arg(sha256),size_bytes=sqlc.arg(size_bytes),
    update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: UpdateSoftwarePackageFile :exec
UPDATE monitor_software_package
SET file=sqlc.arg(file),sha256=sqlc.arg(sha256),size_bytes=sqlc.arg(size_bytes),update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: UpdateSoftwarePackageFilePath :exec
UPDATE monitor_software_package SET file=sqlc.arg(file) WHERE id=sqlc.arg(id);

-- COALESCE(?, col) 表示"传 NULL 就保留原值"（服务文件内容/运行组/工作目录三项）。
-- name: SetSoftwarePackageServiceFileContent :exec
UPDATE monitor_software_package
SET service_file_content=sqlc.arg(service_file_content), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: UpdateSoftwarePackageConfig :exec
UPDATE monitor_software_package
SET default_port=sqlc.arg(default_port),
    service_file_content=COALESCE(sqlc.narg(service_file_content), service_file_content),
    service_run_as_user=sqlc.arg(service_run_as_user),
    service_run_as_group=COALESCE(sqlc.narg(service_run_as_group), service_run_as_group),
    work_directory=COALESCE(sqlc.narg(work_directory), work_directory),
    package_format=sqlc.arg(package_format), platform_family=sqlc.arg(platform_family),
    platform_major=sqlc.arg(platform_major), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: DeleteSoftwarePackage :exec
DELETE FROM monitor_software_package WHERE id=sqlc.arg(id);

-- name: ClearSoftwarePackageInstallTemplate :exec
UPDATE monitor_software_package SET install_playbook_template_id=NULL WHERE id=sqlc.arg(id);

-- name: ClearSoftwarePackageUninstallTemplate :exec
UPDATE monitor_software_package SET uninstall_playbook_template_id=NULL WHERE id=sqlc.arg(id);

-- name: SetSoftwarePackageInstallTemplate :exec
UPDATE monitor_software_package SET install_playbook_template_id=sqlc.arg(template_id) WHERE id=sqlc.arg(id);

-- name: SetSoftwarePackageUninstallTemplate :exec
UPDATE monitor_software_package SET uninstall_playbook_template_id=sqlc.arg(template_id) WHERE id=sqlc.arg(id);

-- ---- P2-3：监控目标域（目标 CRUD/服务控制、安装与卸载下发、安装历史、宿主总览）----
-- 三处方言/结构改写：① 批量建目标用 `INSERT IGNORE`（已纳管则跳过），派生改写成 PG 的
-- `ON CONFLICT DO NOTHING`；② PATCH 的字段白名单从"运行时拼 SET"改成"读回+应用层合并+整行写"；
-- ③ 宿主总览的四种过滤（有目标/指定 exporter/日志已装/未装）改成 narg + CASE，三条查询共用。

-- name: GetExporterPackageDefaultPort :one
SELECT default_port FROM monitor_software_package
WHERE name = sqlc.arg(name) AND package_type = 'exporter' AND enabled = TRUE ORDER BY id LIMIT 1;

-- name: GetHostTargetIdentity :one
SELECT COALESCE(instance_name, ''), COALESCE(ip, ''), is_deleted_in_cloud FROM assets_host WHERE id = sqlc.arg(id);

-- 已纳管（host_id, exporter_type 唯一键冲突）时跳过：MySQL 的 INSERT IGNORE 影响行数为 0，
-- PG 侧派生为 ON CONFLICT DO NOTHING（被跳过时 RETURNING 不返回行）。
-- name: CreateMonitorTargetIfAbsent :execresult
INSERT IGNORE INTO monitor_target(create_time,update_time,remark,host_id,exporter_type,scrape_port,managed_enabled,
                                  install_status,install_message,retry_count,last_scrape_status,labels,last_dispatch_manual)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(host_id),sqlc.arg(exporter_type),
        sqlc.arg(scrape_port),TRUE,'unknown','',0,'unknown','{}',FALSE);

-- name: UpdateMonitorTargetPatch :exec
UPDATE monitor_target
SET exporter_type=sqlc.arg(exporter_type), scrape_port=sqlc.arg(scrape_port),
    managed_enabled=sqlc.arg(managed_enabled), labels=sqlc.arg(labels), remark=sqlc.narg(remark),
    update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: GetMonitorTargetState :one
SELECT managed_enabled, install_status FROM monitor_target WHERE id = sqlc.arg(id);

-- name: DeleteMonitorTarget :exec
DELETE FROM monitor_target WHERE id = sqlc.arg(id);

-- name: GetLatestTargetInstallHistory :one
SELECT id, status, start_time FROM monitor_target_install_history
WHERE target_id = sqlc.arg(target_id) ORDER BY id DESC LIMIT 1;

-- name: CancelInstallHistory :exec
UPDATE monitor_target_install_history
SET status='cancelled', summary_message='任务已取消', error_message_snapshot='任务已由用户取消',
    end_time=sqlc.arg(end_time), duration_seconds=sqlc.arg(duration_seconds), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: MarkTargetInstallCancelled :exec
UPDATE monitor_target
SET install_status='failed', install_message='安装/卸载任务已取消', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: GetTargetServiceContext :one
SELECT COALESCE(h.instance_name, ''), t.exporter_type
FROM monitor_target t JOIN assets_host h ON h.id = t.host_id WHERE t.id = sqlc.arg(id);

-- name: GetTargetHostAddress :one
SELECT h.instance_name, h.ip
FROM monitor_target t JOIN assets_host h ON h.id = t.host_id WHERE t.id = sqlc.arg(id);

-- ---- 安装/卸载下发 ----

-- name: GetTargetInstallContext :one
SELECT t.id, t.host_id, t.managed_enabled, t.exporter_type,
       COALESCE(h.instance_name, ''), COALESCE(h.ip, ''),
       COALESCE(s.os_id, ''), COALESCE(s.os_id_like, ''), COALESCE(s.os_version_id, ''),
       COALESCE(hw.architecture, '')
FROM monitor_target t
JOIN assets_host h ON h.id = t.host_id
LEFT JOIN assets_hostsystem s ON s.host_id = t.host_id
LEFT JOIN assets_hosthardware hw ON hw.host_id = t.host_id
WHERE t.id = sqlc.arg(id);

-- name: ResetTargetInstallRetry :exec
UPDATE monitor_target SET retry_count=0, install_message='人工触发重试', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: SetTargetInstallState :exec
UPDATE monitor_target SET install_status=sqlc.arg(install_status), install_message=sqlc.arg(install_message),
       update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: CreateMonitorTargetJob :execlastid
INSERT INTO automation_execution_job
  (create_time,update_time,remark,job_id,status,trigger_type,source,inventory_snapshot,extra_vars,result_summary,
   task_name_snapshot,template_name_snapshot,template_content_snapshot,`limit`,run_as_user_snapshot,
   run_as_group_snapshot,work_directory_snapshot,requested_user_id,requested_username)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(job_id),'pending','manual','monitor_target',
        sqlc.arg(inventory_snapshot),sqlc.arg(extra_vars),sqlc.arg(result_summary),sqlc.arg(task_name_snapshot),
        sqlc.arg(template_name_snapshot),sqlc.arg(template_content_snapshot),'',sqlc.arg(run_as_user_snapshot),
        sqlc.arg(run_as_group_snapshot),sqlc.arg(work_directory_snapshot),sqlc.narg(requested_user_id),
        sqlc.arg(requested_username));

-- name: CreateTargetInstallHistory :execlastid
INSERT INTO monitor_target_install_history
  (create_time,update_time,remark,action,trigger_type,status,host_id_snapshot,host_name_snapshot,host_ip_snapshot,
   exporter_type_snapshot,summary_message,stdout_snapshot,stderr_snapshot,error_message_snapshot,result_summary_snapshot,
   requested_user_id_snapshot,requested_username_snapshot,start_time,host_id,target_id,automation_job_id_snapshot)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(action),'manual','pending',
        sqlc.narg(host_id_snapshot),sqlc.arg(host_name_snapshot),sqlc.arg(host_ip_snapshot),
        sqlc.arg(exporter_type_snapshot),sqlc.arg(summary_message),'','','','{}',
        sqlc.narg(requested_user_id_snapshot),sqlc.arg(requested_username_snapshot),
        sqlc.narg(start_time),sqlc.narg(host_id),sqlc.arg(target_id),sqlc.narg(automation_job_id_snapshot));

-- name: MarkTargetInstallPending :exec
UPDATE monitor_target
SET install_status='pending', install_message=sqlc.arg(install_message), last_dispatch_manual=TRUE,
    update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- 作业收尾时读回结果摘要（原实现用 MySQL 的 JSON_UNQUOTE(JSON_EXTRACT(...,'$.message'))，
-- 改成取回 json 列在应用层解析）。
-- name: GetJobResultSummary :one
SELECT status, result_summary FROM automation_execution_job WHERE id = sqlc.arg(id);

-- name: FinishTargetInstallState :execrows
UPDATE monitor_target SET install_status=sqlc.arg(install_status), install_message=sqlc.arg(install_message),
       update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND install_status='pending';

-- name: FinishTargetInstallHistory :execrows
UPDATE monitor_target_install_history
SET status=sqlc.arg(status), summary_message=sqlc.arg(summary_message), end_time=sqlc.narg(end_time),
    duration_seconds=sqlc.narg(duration_seconds), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND status='pending';

-- name: GetTargetInstallHistoryForTimeout :one
SELECT id, status, create_time FROM monitor_target_install_history
WHERE target_id = sqlc.arg(target_id) ORDER BY id DESC LIMIT 1;

-- name: ExpireTargetInstallHistory :execrows
UPDATE monitor_target_install_history
SET status='failed', error_message_snapshot='任务执行超时（进程中断遗留），已自动过期', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND status IN ('pending','running');

-- 平台匹配用的软件包查询：卸载取最近一条（不论有无文件），安装只取已落文件的一批。
-- name: GetUninstallPackageForTarget :one
SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,
       COALESCE(file,''),COALESCE(sha256,''),service_file_content,service_run_as_user,service_run_as_group,
       work_directory,uninstall_playbook_template_id
FROM monitor_software_package
WHERE package_type='exporter' AND name=sqlc.arg(name) AND enabled=TRUE
ORDER BY create_time DESC LIMIT 1;

-- name: ListInstallPackagesForTarget :many
SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,file,sha256,
       service_file_content,service_run_as_user,service_run_as_group,work_directory,install_playbook_template_id
FROM monitor_software_package
WHERE package_type='exporter' AND name=sqlc.arg(name) AND enabled=TRUE AND file<>''
ORDER BY create_time DESC;

-- name: ListPackageChecksums :many
SELECT os,arch,sha256 FROM monitor_software_package
WHERE package_type='exporter' AND name=sqlc.arg(name) AND version=sqlc.arg(version) AND enabled=TRUE AND sha256<>'';

-- ---- 宿主总览（监控纳管情况）----

-- name: ListMonitorHostGroupTree :many
SELECT g.id, g.name, g.parent_id, COUNT(h.id) AS host_count,
       COUNT(CASE WHEN h.id IS NOT NULL AND (EXISTS(SELECT 1 FROM monitor_target t WHERE t.host_id = h.id)
             OR EXISTS(SELECT 1 FROM monitor_log_collection_target l WHERE l.host_id = h.id)) THEN 1 END) AS managed_count
FROM assets_hostgroup g
LEFT JOIN assets_host h ON h.group_id = g.id AND h.is_deleted_in_cloud = FALSE
GROUP BY g.id, g.name, g.parent_id
ORDER BY g.name, g.id;

-- name: CountMonitorHostTotals :one
SELECT COUNT(*),
       COUNT(CASE WHEN EXISTS(SELECT 1 FROM monitor_target t WHERE t.host_id = h.id)
             OR EXISTS(SELECT 1 FROM monitor_log_collection_target l WHERE l.host_id = h.id) THEN 1 END) AS managed_total,
       COUNT(CASE WHEN h.group_id IS NULL THEN 1 END) AS ungrouped
FROM assets_host h WHERE h.is_deleted_in_cloud = FALSE;

-- name: ListMonitorHostGroupParents :many
SELECT id, parent_id FROM assets_hostgroup;

-- 宿主列表的过滤：搜索 / 组（含子组，可变长 IN）/ exporter 纳管状态 / 日志采集纳管状态。
-- managed_filter 与 filebeat_filter 是 'true'/'false'/NULL 三态，用 CASE 表达"命中/未命中/不过滤"。
-- name: CountMonitorHosts :one
SELECT COUNT(*) FROM assets_host h
WHERE h.is_deleted_in_cloud = FALSE
  AND (sqlc.arg(search_pattern) = '' OR h.instance_name LIKE sqlc.arg(search_pattern)
       OR COALESCE(h.ip, '') LIKE sqlc.arg(search_pattern))
  AND (sqlc.arg(group_filter) = '' OR h.group_id IN (sqlc.slice(group_ids)))
  AND (CASE sqlc.narg(managed_filter)
         WHEN 'true' THEN (SELECT COUNT(*) FROM monitor_target mt WHERE mt.host_id = h.id
                AND (sqlc.arg(exporter_type) = '' OR mt.exporter_type = sqlc.arg(exporter_type))) > 0
         WHEN 'false' THEN (SELECT COUNT(*) FROM monitor_target mt WHERE mt.host_id = h.id
                AND (sqlc.arg(exporter_type) = '' OR mt.exporter_type = sqlc.arg(exporter_type))) = 0
         ELSE (sqlc.arg(exporter_type) = '' OR EXISTS (SELECT 1 FROM monitor_target mt
                WHERE mt.host_id = h.id AND mt.exporter_type = sqlc.arg(exporter_type)))
       END)
  AND (CASE sqlc.narg(filebeat_filter)
         WHEN 'true' THEN (SELECT COUNT(*) FROM monitor_log_collection_target lc WHERE lc.host_id = h.id AND lc.agent_installed = TRUE) > 0
         WHEN 'false' THEN (SELECT COUNT(*) FROM monitor_log_collection_target lc WHERE lc.host_id = h.id AND lc.agent_installed = TRUE) = 0
         ELSE TRUE
       END);

-- name: ListMonitorHosts :many
SELECT h.id, h.instance_name, h.ip, h.group_id, COALESCE(g.name, '') AS group_name,
       lc.id AS log_target_id, lc.agent_installed, lc.agent_version, lc.runtime_status,
       lc.install_status, lc.config_fingerprint, lc.last_applied_time, lc.last_error
FROM assets_host h
LEFT JOIN assets_hostgroup g ON g.id = h.group_id
LEFT JOIN monitor_log_collection_target lc ON lc.host_id = h.id
WHERE h.is_deleted_in_cloud = FALSE
  AND (sqlc.arg(search_pattern) = '' OR h.instance_name LIKE sqlc.arg(search_pattern)
       OR COALESCE(h.ip, '') LIKE sqlc.arg(search_pattern))
  AND (sqlc.arg(group_filter) = '' OR h.group_id IN (sqlc.slice(group_ids)))
  AND (CASE sqlc.narg(managed_filter)
         WHEN 'true' THEN (SELECT COUNT(*) FROM monitor_target mt WHERE mt.host_id = h.id
                AND (sqlc.arg(exporter_type) = '' OR mt.exporter_type = sqlc.arg(exporter_type))) > 0
         WHEN 'false' THEN (SELECT COUNT(*) FROM monitor_target mt WHERE mt.host_id = h.id
                AND (sqlc.arg(exporter_type) = '' OR mt.exporter_type = sqlc.arg(exporter_type))) = 0
         ELSE (sqlc.arg(exporter_type) = '' OR EXISTS (SELECT 1 FROM monitor_target mt
                WHERE mt.host_id = h.id AND mt.exporter_type = sqlc.arg(exporter_type)))
       END)
  AND (CASE sqlc.narg(filebeat_filter)
         WHEN 'true' THEN (SELECT COUNT(*) FROM monitor_log_collection_target lc WHERE lc.host_id = h.id AND lc.agent_installed = TRUE) > 0
         WHEN 'false' THEN (SELECT COUNT(*) FROM monitor_log_collection_target lc WHERE lc.host_id = h.id AND lc.agent_installed = TRUE) = 0
         ELSE TRUE
       END)
ORDER BY h.instance_name, h.id LIMIT ? OFFSET ?;

-- name: GetInstallHistoryForUpdate :one
SELECT status, target_id, log_collection_target_id, start_time FROM monitor_target_install_history
WHERE id = sqlc.arg(id) FOR UPDATE;

-- name: CancelMonitorTargetInstallState :exec
UPDATE monitor_target SET install_status='unknown', install_message='安装/卸载任务已取消', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: CancelLogTargetInstallState :exec
UPDATE monitor_log_collection_target SET install_status='unknown', install_message='安装/卸载任务已取消', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- ---- P2-3：告警媒介（monitor_alert_media）写路径与详情读取 ----
-- recipients 是 NOT NULL 的 json 列：MySQL 侧靠列默认值 '[]' 兜住，PG 的 schema 没有默认值，
-- 所以 INSERT 里显式写 '[]'。

-- name: GetAlertMediaConfig :one
SELECT config FROM monitor_alert_media WHERE id = sqlc.arg(id);

-- name: GetAlertMediaName :one
SELECT name FROM monitor_alert_media WHERE id = sqlc.arg(id);

-- name: CreateAlertMedia :execlastid
INSERT INTO monitor_alert_media(create_time,update_time,remark,name,media_type,config,enabled,recipients)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.arg(remark),sqlc.arg(name),sqlc.arg(media_type),
        sqlc.arg(config),sqlc.arg(enabled),'[]');

-- name: UpdateAlertMedia :execrows
UPDATE monitor_alert_media
SET update_time=sqlc.arg(update_time),remark=sqlc.arg(remark),name=sqlc.arg(name),
    media_type=sqlc.arg(media_type),config=sqlc.arg(config),enabled=sqlc.arg(enabled)
WHERE id=sqlc.arg(id);

-- name: DeleteAlertMedia :execresult
DELETE FROM monitor_alert_media WHERE id=sqlc.arg(id);

-- ---- 告警通知分发链路（event / delivery / 失联对账 / 服务树归属）----

-- 入队去重：deduplication_key 是唯一键，"已有事件"即影响行数为 0。
-- MySQL 用 INSERT IGNORE（影响行数 0），PG 侧派生为 ON CONFLICT DO NOTHING
-- （被跳过时 RETURNING 不返回行）；两侧判定见 alert_event_dialect_*.go。
-- name: CreateAlertNotificationEventIfAbsent :execresult
INSERT IGNORE INTO monitor_alert_notification_event
  (create_time,update_time,remark,event_type,deduplication_key,status,attempt_count,error_message,sent_at,alert_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(event_type),sqlc.arg(deduplication_key),
        'pending',0,'',NULL,sqlc.arg(alert_id));

-- 事件与告警关键字段一次取齐（原实现分两条语句读同一行，合并为一条）：
-- event_type 决定路由匹配开关，status 决定是否跳过已成功的事件，labels 用于 matchers 匹配。
-- name: GetAlertNotificationEventDispatch :one
SELECT e.event_type, e.attempt_count, e.status,
       a.id, a.alertname, a.severity, a.instance, a.state, a.labels
FROM monitor_alert_notification_event e
JOIN monitor_alert_history a ON a.id = e.alert_id
WHERE e.id = sqlc.arg(id);

-- name: MarkAlertNotificationEventFailed :exec
UPDATE monitor_alert_notification_event
SET status='failed', error_message=sqlc.arg(error_message), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: MarkAlertNotificationEventSending :exec
UPDATE monitor_alert_notification_event
SET status='sending', attempt_count=attempt_count+1, update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: MarkAlertNotificationEventSuccess :exec
UPDATE monitor_alert_notification_event
SET status='success', sent_at=sqlc.arg(sent_at), error_message='', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: MarkAlertNotificationEventPending :exec
UPDATE monitor_alert_notification_event
SET status='pending', error_message=sqlc.arg(error_message), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- 出口媒介的收件人绑定：不限组 / 仅限指定用户组成员两种（组限制为可变长 IN）。
-- name: ListAlertMediaBindingsByMedia :many
SELECT b.user_id, u.username, b.recipients
FROM monitor_user_alert_media_binding b JOIN sys_user u ON u.id = b.user_id
WHERE b.media_id = sqlc.arg(media_id) AND b.enabled = TRUE
ORDER BY b.id;

-- name: ListAlertMediaBindingsByMediaInUserGroups :many
SELECT b.user_id, u.username, b.recipients
FROM monitor_user_alert_media_binding b JOIN sys_user u ON u.id = b.user_id
WHERE b.media_id = sqlc.arg(media_id) AND b.enabled = TRUE
  AND b.user_id IN (SELECT user_id FROM sys_user_group_member WHERE group_id IN (sqlc.slice(group_ids)))
ORDER BY b.id;

-- 单地址投递的 get-or-create：唯一键 (event_id, media_id, user_id, address) 冲突时把既有行的
-- 主键作为 LastInsertId 返回（MySQL 惯用法 `id=LAST_INSERT_ID(id)`）。PG 没有 LAST_INSERT_ID，
-- 由 derive 的 perQueryOverride 换成 `id = monitor_alert_notification_delivery.id` 的等价空操作
-- —— 不能写成 VALUES(id)/EXCLUDED.id，那是序列的下一个值，不是既有行的 id。
-- conflict: event_id, media_id, user_id, address
-- name: CreateAlertNotificationDeliveryOrGetID :execlastid
-- conflict: event_id, media_id, user_id, address
INSERT INTO monitor_alert_notification_delivery
  (create_time,update_time,remark,address,status,attempt_count,error_message,sent_at,event_id,media_id,user_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(address),'pending',0,'',NULL,
        sqlc.arg(event_id),sqlc.arg(media_id),sqlc.arg(user_id))
ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id);

-- name: GetAlertNotificationDeliveryStatus :one
SELECT status FROM monitor_alert_notification_delivery WHERE id = sqlc.arg(id);

-- name: MarkAlertNotificationDeliverySending :exec
UPDATE monitor_alert_notification_delivery
SET status='sending', attempt_count=attempt_count+1, error_message='', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: MarkAlertNotificationDeliverySuccess :exec
UPDATE monitor_alert_notification_delivery
SET status='success', sent_at=sqlc.arg(sent_at), error_message='', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: MarkAlertNotificationDeliveryFailed :exec
UPDATE monitor_alert_notification_delivery
SET status='failed', error_message=sqlc.arg(error_message), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- 失联对账：阈值改为应用层算好的时间点（原实现用 UTC_TIMESTAMP(6) - INTERVAL ? MINUTE，
-- 既有方言函数又让阈值跟着库时钟走）。
-- name: ListStaleFiringAlerts :many
SELECT id, alertname, severity, instance, labels
FROM monitor_alert_history
WHERE state = 'firing' AND source = 'prometheus' AND last_seen_at < sqlc.arg(stale_before);

-- name: ResolveStaleAlert :execrows
UPDATE monitor_alert_history
SET state='resolved', resolved_at=sqlc.arg(resolved_at), resolved_by_reconciliation=TRUE,
    update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND state='firing';

-- 告警主机在服务树上的归属节点（供策略树的 tree matcher 用）。
-- 原实现写的是 bs.project —— assets_business_system 没有这一列（真库与 schema 都没有），
-- 语句恒报 1054，被调用点忽略后 tree matcher 永远匹配不上：改成 bs.project_id。
-- name: ListHostAlertScopeNodes :many
SELECT DISTINCT s.id AS service_id, s.business_system_id, s.environment_id, bs.project_id
FROM assets_application_deployment d
JOIN assets_application_service_deployment sd ON sd.deployment_id = d.id
JOIN assets_application_service s ON s.id = sd.service_id
JOIN assets_business_system bs ON bs.id = s.business_system_id
WHERE d.host_id = sqlc.arg(host_id) AND d.enabled = TRUE AND sd.enabled = TRUE;

-- ---- 告警链诊断（user-chain / chain/:historyId）----

-- name: GetUsernameByID :one
SELECT username FROM sys_user WHERE id = sqlc.arg(id);

-- 用户绑定（含媒介的 media_type/media_enabled）复用 user.sql 的 ListUserAlertMediaBindings。

-- name: CountUserGroupMemberships :one
SELECT COUNT(*) FROM sys_user_group_member
WHERE user_id = sqlc.arg(user_id) AND group_id IN (sqlc.slice(group_ids));

-- name: GetAlertHistoryForChain :one
SELECT alertname, severity, instance, labels, state, started_at
FROM monitor_alert_history WHERE id = sqlc.arg(id);

-- 策略出口媒介：只有启用中的才投递，且要 config（SMTP 参数）。
-- name: ListEnabledAlertMediaByIDs :many
SELECT id, name, media_type, config FROM monitor_alert_media
WHERE enabled = TRUE AND id IN (sqlc.slice(media_ids))
ORDER BY id;

-- name: ListAlertMediaBriefByIDs :many
SELECT id, name, enabled FROM monitor_alert_media
WHERE id IN (sqlc.slice(media_ids))
ORDER BY id;

-- name: ListAlertMediaBindingsWithUser :many
SELECT b.user_id, u.username, b.recipients, b.enabled
FROM monitor_user_alert_media_binding b JOIN sys_user u ON u.id = b.user_id
WHERE b.media_id = sqlc.arg(media_id)
ORDER BY b.id;

-- name: GetLatestAlertNotificationEventForAlert :one
SELECT id, event_type, status, attempt_count, error_message
FROM monitor_alert_notification_event
WHERE alert_id = sqlc.arg(alert_id) AND event_type = sqlc.arg(event_type)
ORDER BY id DESC LIMIT 1;

-- 链诊断里的投递记录：用户名保持可空（未登录用户的历史记录），与历史详情页的
-- ListAlertNotificationDeliveries（把空值渲染成 '-'）语义不同，故单独一条。
-- name: ListAlertNotificationDeliveriesForChain :many
SELECT d.user_id, d.address, d.status, d.error_message, u.username
FROM monitor_alert_notification_delivery d
LEFT JOIN sys_user u ON u.id = d.user_id
WHERE d.event_id = sqlc.arg(event_id)
ORDER BY d.id;

-- ---- 通知策略树（monitor_notification_policy）----

-- 树加载与管理列表共用一条：列集取并集（树的加载忽略 create_time/update_time）。
-- media_ids / user_group_ids 用左连接的 NULL 表达"继承父节点"，不能 COALESCE 成空串。
-- name: ListNotificationPolicyNodes :many
SELECT id, COALESCE(parent_id, 0) AS parent_id, name, position, COALESCE(remark, '') AS remark,
       matchers, media_ids, user_group_ids, notify_on_firing, notify_on_resolved,
       create_time, update_time
FROM monitor_notification_policy
ORDER BY position, id;

-- name: GetNotificationPolicyParent :one
SELECT COALESCE(parent_id, 0) FROM monitor_notification_policy WHERE id = sqlc.arg(id);

-- name: GetUserGroupName :one
SELECT name FROM sys_user_group WHERE id = sqlc.arg(id);

-- parent_id / media_ids / user_group_ids 的 NULL 是有意义的（根节点、继承），
-- 所以用 narg：nil 即写 NULL。
-- name: CreateNotificationPolicy :execlastid
INSERT INTO monitor_notification_policy
  (create_time,update_time,remark,parent_id,name,position,matchers,media_ids,user_group_ids,
   notify_on_firing,notify_on_resolved)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.arg(remark),sqlc.narg(parent_id),sqlc.arg(name),
        sqlc.arg(position),sqlc.arg(matchers),sqlc.narg(media_ids),sqlc.narg(user_group_ids),
        sqlc.arg(notify_on_firing),sqlc.arg(notify_on_resolved));

-- name: UpdateNotificationPolicy :exec
UPDATE monitor_notification_policy
SET update_time=sqlc.arg(update_time),remark=sqlc.arg(remark),parent_id=sqlc.narg(parent_id),
    name=sqlc.arg(name),position=sqlc.arg(position),matchers=sqlc.arg(matchers),
    media_ids=sqlc.narg(media_ids),user_group_ids=sqlc.narg(user_group_ids),
    notify_on_firing=sqlc.arg(notify_on_firing),notify_on_resolved=sqlc.arg(notify_on_resolved)
WHERE id=sqlc.arg(id);

-- name: DeleteNotificationPolicy :exec
DELETE FROM monitor_notification_policy WHERE id=sqlc.arg(id);

-- ---- 告警摄取 webhook（monitor_alert_history）----

-- 同 fingerprint 的未恢复告警行：加锁读，顺带取回 rule_group/rule_snapshot 供应用层合并
-- （原实现在 UPDATE 里用 IF(rule_group='',?,rule_group) 与
-- IF(IFNULL(JSON_LENGTH(rule_snapshot),0)=0,?,rule_snapshot) 表达"已有值优先"，是 MySQL 方言函数）。
-- name: GetOpenFiringAlertForUpdate :one
SELECT id, rule_group, rule_snapshot FROM monitor_alert_history
WHERE fingerprint = sqlc.arg(fingerprint) AND state = 'firing'
ORDER BY id DESC LIMIT 1 FOR UPDATE;

-- name: ResolveAlertHistoryFromWebhook :exec
UPDATE monitor_alert_history
SET state='resolved', resolved_at=sqlc.arg(resolved_at), last_seen_at=sqlc.arg(last_seen_at),
    annotations=sqlc.arg(annotations), resolved_by_reconciliation=FALSE, update_time=sqlc.arg(update_time),
    rule_group=sqlc.arg(rule_group), rule_snapshot=sqlc.arg(rule_snapshot)
WHERE id=sqlc.arg(id);

-- name: UpdateAlertHistoryHeartbeat :exec
UPDATE monitor_alert_history
SET last_seen_at=sqlc.arg(last_seen_at), labels=sqlc.arg(labels), annotations=sqlc.arg(annotations),
    update_time=sqlc.arg(update_time), rule_group=sqlc.arg(rule_group), rule_snapshot=sqlc.arg(rule_snapshot)
WHERE id=sqlc.arg(id);

-- name: CreateAlertHistory :execlastid
INSERT INTO monitor_alert_history
  (create_time,update_time,remark,source,fingerprint,alertname,rule_group,rule_snapshot,severity,instance,
   labels,annotations,generator_url,state,started_at,resolved_at,last_seen_at,resolved_by_reconciliation)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),'',sqlc.arg(source),sqlc.arg(fingerprint),
        sqlc.arg(alertname),sqlc.arg(rule_group),sqlc.arg(rule_snapshot),sqlc.arg(severity),sqlc.arg(instance),
        sqlc.arg(labels),sqlc.arg(annotations),sqlc.arg(generator_url),'firing',sqlc.arg(started_at),NULL,
        sqlc.arg(last_seen_at),FALSE);

-- ---- P2-3：日志采集目标（monitor_log_collection_target）的运维写路径 ----
-- 原实现有两处运行时拼 SQL：① Filebeat 软件包按"安装/卸载"拼 playbook 列名；
-- ② 时间差用 TIMESTAMPDIFF(MICROSECOND,…)/1000000。前者按角色分派成两条显式语句，
-- 后者改成应用层算（历史的 create_time 就是派发时刻，闭包里有同一个 now）。

-- name: GetLogTargetForAction :one
SELECT l.id, l.host_id, l.managed_enabled, l.install_status,
       COALESCE(h.instance_name, ''), COALESCE(h.ip, ''),
       COALESCE(s.os_type, ''), COALESCE(s.os_id_like, ''), COALESCE(s.os_version_id, '')
FROM monitor_log_collection_target l
JOIN assets_host h ON h.id = l.host_id
LEFT JOIN assets_hostsystem s ON s.host_id = l.host_id
WHERE l.id = sqlc.arg(id);

-- 取最近一条安装历史用于"是否有任务在执行中"（NULL 行由 ErrNoRows 表达）。
-- create_time 用于取消时算时长（该流程的历史行 start_time 为 NULL，只有 create_time
-- 是派发时刻）。
-- name: GetLatestLogTargetInstallHistory :one
SELECT id, status, create_time FROM monitor_target_install_history
WHERE log_collection_target_id = sqlc.arg(log_collection_target_id)
ORDER BY id DESC LIMIT 1;

-- Filebeat 只用官方便携 tar.gz，按 CPU 架构匹配，不区分发行版/主版本/包格式。
-- playbook 是否配置交给应用层判断，便于区分"没有包"与"包没配 playbook"。
-- name: ListInstallableFilebeatPackages :many
SELECT id,COALESCE(arch,''),COALESCE(file,''),COALESCE(sha256,''),COALESCE(service_file_content,''),install_playbook_template_id
FROM monitor_software_package
WHERE package_type='filebeat' AND package_format='tar.gz' AND enabled=TRUE
ORDER BY id;

-- name: ListUninstallableFilebeatPackages :many
SELECT id,COALESCE(arch,''),COALESCE(file,''),COALESCE(sha256,''),COALESCE(service_file_content,''),uninstall_playbook_template_id
FROM monitor_software_package
WHERE package_type='filebeat' AND package_format='tar.gz' AND enabled=TRUE
ORDER BY id;

-- name: CreateLogTargetInstallHistory :execlastid
INSERT INTO monitor_target_install_history
  (create_time,update_time,remark,action,trigger_type,status,host_id_snapshot,host_name_snapshot,host_ip_snapshot,
   exporter_type_snapshot,summary_message,stdout_snapshot,stderr_snapshot,error_message_snapshot,result_summary_snapshot,
   requested_user_id_snapshot,requested_username_snapshot,start_time,host_id,log_collection_target_id,automation_job_id_snapshot)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(action),'manual','pending',
        sqlc.narg(host_id_snapshot),sqlc.arg(host_name_snapshot),sqlc.arg(host_ip_snapshot),
        sqlc.arg(exporter_type_snapshot),sqlc.arg(summary_message),'','','','{}',
        sqlc.narg(requested_user_id_snapshot),sqlc.arg(requested_username_snapshot),
        NULL,sqlc.narg(host_id),sqlc.arg(log_collection_target_id),sqlc.narg(automation_job_id_snapshot));

-- name: MarkLogTargetInstallPending :exec
UPDATE monitor_log_collection_target
SET install_status='pending', install_message='', last_dispatch_manual=TRUE, update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- 收尾：只有仍处于 pending 的任务才落终态。
-- install_succeeded 用 0/1 传，不能把同一个 sqlc.arg 写两次（MySQL 引擎会拆成 FinalStatus/FinalStatus_2，
-- 而 PG 只合并成一个参数——同一个调用点在两侧就编译不过），用整数比较避开这个分歧。
-- agent_installed 是"Filebeat 二进制已装"的持久态，只有成功收尾才改写（install 成功 TRUE、
-- uninstall 成功 FALSE）；失败时传 NULL 保持原值，避免安装失败把已装状态抹掉。
-- name: FinishLogTargetInstallState :execrows
UPDATE monitor_log_collection_target
SET install_status=sqlc.arg(install_status), install_message=sqlc.arg(install_message),
    runtime_status=CASE WHEN sqlc.arg(install_succeeded) = 1 THEN 'running' ELSE runtime_status END,
    agent_installed=COALESCE(sqlc.narg(agent_installed), agent_installed),
    update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND install_status='pending';

-- name: FinishLogTargetInstallHistory :execrows
UPDATE monitor_target_install_history
SET status=sqlc.arg(status), summary_message=sqlc.arg(summary_message), end_time=sqlc.arg(end_time),
    duration_seconds=sqlc.arg(duration_seconds), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND status='pending';

-- name: GetLogTargetHostName :one
SELECT COALESCE(h.instance_name, '')
FROM monitor_log_collection_target l JOIN assets_host h ON h.id = l.host_id
WHERE l.id = sqlc.arg(id);

-- name: SetLogTargetRuntimeStatus :exec
UPDATE monitor_log_collection_target
SET runtime_status=sqlc.arg(runtime_status), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: GetDefaultEnabledElasticsearchCluster :one
SELECT hosts, username, password, COALESCE(index_prefix, 'logs'), verify_tls
FROM monitor_elasticsearch_cluster
WHERE enabled = TRUE
ORDER BY is_default DESC, id LIMIT 1;

-- 清理数据流用的完整默认集群连接信息（含 id/ca_cert/request_timeout，供 elasticsearchRequest 使用）。
-- name: GetDefaultEnabledElasticsearchClusterConnection :one
SELECT id,hosts,username,password,verify_tls,ca_cert,index_prefix,request_timeout,enabled
FROM monitor_elasticsearch_cluster
WHERE enabled = TRUE
ORDER BY is_default DESC, id LIMIT 1;

-- name: GetLogTargetConfigFingerprint :one
SELECT COALESCE(config_fingerprint, '') FROM monitor_log_collection_target WHERE id = sqlc.arg(id);

-- 渲染 Filebeat inputs 所需的「服务×实例」行，按主机批量取（一次查完一页/一批主机，
-- 避免按主机循环查库）。列与语义同 loadHostLogRenderInput 的实例查询。
-- name: ListHostLogRenderInstances :many
SELECT d.host_id, s.code AS service_code, d.instance_name,
       COALESCE(d.runtime_variables, '{}') AS runtime_variables,
       COALESCE(t.app_home, '') AS app_home, COALESCE(h.ip, '') AS host_ip
FROM assets_application_service_deployment sd
JOIN assets_application_deployment d ON d.id = sd.deployment_id
JOIN assets_application_service s ON s.id = sd.service_id
JOIN assets_application_deployment_template t ON t.id = s.deployment_template_id
JOIN assets_host h ON h.id = d.host_id
WHERE d.host_id IN (sqlc.slice(host_ids))
  AND sd.enabled = TRUE AND d.enabled = TRUE AND s.enabled = TRUE AND s.log_collection_enabled = TRUE;

-- 渲染 Filebeat inputs 所需的「服务×日志定义」行，按主机批量取。
-- 原先按主机用 `s.id IN (子查询)` 表达"该主机上部署了哪些服务"，批量取必须把主机维度带出来，
-- 因此把子查询改成对 deployment 的 JOIN 并 SELECT d.host_id；DISTINCT 保证同一主机上
-- 一个服务部署多实例时只出一行（与逐主机查询的行为一致）。
--
-- 采集开关只有两处且都在服务侧：服务总开关 `s.log_collection_enabled`，
-- 以及 `(服务×日志定义)` 的覆盖行 `ls.collection_enabled`（NULL 或无覆盖行 = 默认采，
-- 显式 FALSE = 不采）。模板日志定义不再带开关（迁移 000034 删列），
-- 因此这里**不能**再出现 `ld.collection_enabled`。
--
-- 解析规则（pipeline 名 + 多行参数）**只来自模板日志定义**：删掉了服务级覆盖那一支
-- （迁移 000035 删掉 `ls.processing_rule_id`），所以 `rule_definition` 是唯一来源，
-- 不能再出现 `rule_setting` 这个别名。
-- name: ListHostLogRenderEntries :many
SELECT DISTINCT d.host_id, p.code AS project_code, e.code AS environment_code,
    bs.code AS business_system_code, s.code AS service_code,
    COALESCE(app.code, '') AS application_code,
    COALESCE(tier.code, '') AS tier_code,
    COALESCE(rule_definition.name, '') AS pipeline_name,
    ld.name AS log_name, ld.path_pattern,
    COALESCE(s.macro_values, '{}') AS macro_values,
    COALESCE(t.macro_definitions, '[]') AS macro_definitions,
    COALESCE(rule_definition.multiline_enabled, FALSE) AS multiline_enabled,
    COALESCE(rule_definition.start_pattern, '') AS start_pattern,
    COALESCE(rule_definition.flush_timeout, 2000) AS flush_timeout
FROM assets_application_service s
JOIN assets_business_system bs ON bs.id = s.business_system_id
JOIN assets_project p ON p.id = bs.project_id
JOIN assets_business_environment e ON e.id = s.environment_id
JOIN assets_application app ON app.id = s.application_id
JOIN assets_application_deployment_template t ON t.id = s.deployment_template_id
JOIN assets_application_log_definition ld ON ld.deployment_template_id = s.deployment_template_id
JOIN assets_application_service_deployment sd ON sd.service_id = s.id AND sd.enabled = TRUE
JOIN assets_application_deployment d ON d.id = sd.deployment_id AND d.enabled = TRUE
LEFT JOIN assets_application_service_log_setting ls ON ls.service_id = s.id AND ls.log_definition_id = ld.id
LEFT JOIN monitor_log_retention_tier tier ON tier.id = COALESCE(ls.retention_tier_id, s.log_retention_tier_id)
LEFT JOIN monitor_log_processing_rule rule_definition ON rule_definition.id = ld.processing_rule_id
WHERE s.enabled = TRUE AND s.log_collection_enabled = TRUE
  AND COALESCE(ls.collection_enabled, TRUE) = TRUE
  AND d.host_id IN (sqlc.slice(host_ids));

-- name: SetLogTargetLastError :exec
UPDATE monitor_log_collection_target
SET last_error=sqlc.arg(last_error), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- 指纹未变时的"只刷新下发时间"路径。
-- name: MarkLogTargetConfigApplied :exec
UPDATE monitor_log_collection_target
SET last_applied_time=sqlc.arg(last_applied_time), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- 指纹变化并下发成功后：记下发时间与指纹。
-- name: MarkLogTargetConfigSynced :exec
UPDATE monitor_log_collection_target
SET last_applied_time=sqlc.arg(last_applied_time), runtime_status='running', last_error='',
    config_fingerprint=sqlc.arg(config_fingerprint), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: MarkLogTargetInstallCancelled :exec
UPDATE monitor_log_collection_target
SET install_status='failed', install_message='安装/卸载任务已取消', update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- 删除目标前解除安装历史的外键引用（历史本身保留，供追溯）。
-- name: DetachInstallHistoryFromLogTarget :exec
UPDATE monitor_target_install_history SET log_collection_target_id=NULL
WHERE log_collection_target_id = sqlc.arg(log_collection_target_id);

-- name: DeleteLogCollectionTarget :execresult
DELETE FROM monitor_log_collection_target WHERE id = sqlc.arg(id);

-- 批量纳管：host_id 唯一键冲突即"已纳管"（MySQL 的 INSERT IGNORE / PG 的 ON CONFLICT DO NOTHING）。
-- name: CreateLogCollectionTargetIfAbsent :execresult
INSERT IGNORE INTO monitor_log_collection_target
  (create_time,update_time,remark,host_id,agent_installed,agent_version,runtime_status,config_fingerprint,
   last_error,install_status,install_message,last_dispatch_manual,managed_enabled,retry_count)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(host_id),FALSE,'','unknown','',
        '','unknown','',FALSE,TRUE,0);

-- ---- 日志存储（Elasticsearch 集群）与保留档位 ----

-- 保留档位列表（下拉/管理路径用：只列启用的）。
-- 识别流名要的是**全部**档位码，用 ListRetentionTierCodes：停用一个档位不该让既有流变成"未识别"。
-- name: ListEnabledRetentionTiers :many
SELECT code,retention_days,daily_size_gb,rollover_min_index_age
FROM monitor_log_retention_tier WHERE enabled=TRUE ORDER BY retention_days,id;

-- 流名匹配用的档位码全集（不过滤 enabled，理由见 ListServiceStreamDims 的说明）。
-- name: ListRetentionTierCodes :many
SELECT code FROM monitor_log_retention_tier WHERE code <> '' ORDER BY id;

-- name: GetElasticsearchClusterConnection :one
SELECT id,hosts,username,password,verify_tls,ca_cert,index_prefix,request_timeout,enabled
FROM monitor_elasticsearch_cluster WHERE id = sqlc.arg(id);

-- openSearchClusterResponse 的 LastCheckTime/LastCheckSuccess/StorageSyncTime 是可空的，
-- 这里只更新探测结果，其余列不动。
-- name: MarkElasticsearchClusterCheckFailed :exec
UPDATE monitor_elasticsearch_cluster
SET last_check_time=sqlc.arg(last_check_time), last_check_success=FALSE,
    last_check_message=sqlc.arg(last_check_message), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: MarkElasticsearchClusterCheckSuccess :exec
UPDATE monitor_elasticsearch_cluster
SET last_check_time=sqlc.arg(last_check_time), last_check_success=TRUE,
    last_check_message=sqlc.arg(last_check_message), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- 存储同步（index template + ILM 策略）的三个状态落库入口，对应 Django sync_log_storage 任务。
-- name: MarkClusterStorageSyncPending :exec
UPDATE monitor_elasticsearch_cluster
SET storage_sync_status='pending', storage_sync_error='', storage_sync_time=NULL, update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: MarkClusterStorageSyncFailed :exec
UPDATE monitor_elasticsearch_cluster
SET storage_sync_status='failed', storage_sync_error=sqlc.arg(storage_sync_error),
    storage_sync_time=sqlc.arg(storage_sync_time), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: MarkClusterStorageSyncSuccess :exec
UPDATE monitor_elasticsearch_cluster
SET storage_sync_status='success', storage_sync_error='',
    storage_sync_time=sqlc.arg(storage_sync_time), update_time=sqlc.arg(update_time)
WHERE id = sqlc.arg(id);

-- name: ListEnabledElasticsearchClusterIDs :many
SELECT id FROM monitor_elasticsearch_cluster WHERE enabled = TRUE;

-- name: CountAllElasticsearchClusters :one
SELECT COUNT(*) FROM monitor_elasticsearch_cluster;

-- name: ClearDefaultElasticsearchCluster :exec
UPDATE monitor_elasticsearch_cluster SET is_default=FALSE, update_time=sqlc.arg(update_time)
WHERE is_default=TRUE AND id<>sqlc.arg(id);

-- 探测与同步状态三列是 NOT NULL 且库级没有默认值：原实现在建集群时根本不写这三列，
-- 于是在严格模式（真库 sql_mode 含 STRICT_TRANS_TABLES）下报 1364 "Field 'last_check_message'
-- doesn't have a default value" —— 建集群接口一直不可用（"只支持一个集群"所以没人碰到）。
-- 这里按"尚未探测/尚未同步"的语义显式写空串。
-- name: CreateElasticsearchCluster :execlastid
INSERT INTO monitor_elasticsearch_cluster
  (create_time,update_time,name,hosts,username,password,verify_tls,ca_cert,index_prefix,request_timeout,
   enabled,is_default,remark,last_check_time,last_check_success,last_check_message,
   storage_sync_error,storage_sync_status,storage_sync_time)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.arg(name),sqlc.arg(hosts),sqlc.arg(username),
        sqlc.arg(password),sqlc.arg(verify_tls),sqlc.arg(ca_cert),sqlc.arg(index_prefix),
        sqlc.arg(request_timeout),sqlc.arg(enabled),sqlc.arg(is_default),sqlc.arg(remark),
        NULL,NULL,'','','',NULL);

-- 整行写（PATCH 语义由应用层"读回现值 + 合并提交的字段"承担，见 elasticsearch_config.go）。
-- name: UpdateElasticsearchCluster :execrows
UPDATE monitor_elasticsearch_cluster
SET update_time=sqlc.arg(update_time),name=sqlc.arg(name),hosts=sqlc.arg(hosts),username=sqlc.arg(username),
    password=sqlc.arg(password),verify_tls=sqlc.arg(verify_tls),ca_cert=sqlc.arg(ca_cert),
    index_prefix=sqlc.arg(index_prefix),request_timeout=sqlc.arg(request_timeout),enabled=sqlc.arg(enabled),
    is_default=sqlc.arg(is_default),remark=sqlc.arg(remark)
WHERE id=sqlc.arg(id);

-- name: DeleteElasticsearchCluster :execresult
DELETE FROM monitor_elasticsearch_cluster WHERE id = sqlc.arg(id);

-- ---- 日志解析规则 / 采集过滤规则的引用计数与应用名称（读路径的补充列）----

-- name: CountLogDefinitionReferences :one
SELECT COUNT(*) FROM assets_application_log_definition WHERE processing_rule_id = sqlc.arg(processing_rule_id);

-- 档位占用检查拆成两条：一条语句里把同一个参数写两次会被 MySQL 引擎拆成两个参数、
-- 而 PG 引擎合并成一个（同一个调用点在两侧就编译不过），拆开写才是可移植的形状。
-- name: CountRetentionTierServices :one
SELECT COUNT(*) FROM assets_application_service WHERE log_retention_tier_id = sqlc.arg(retention_tier_id);

-- name: CountRetentionTierLogSettings :one
SELECT COUNT(*) FROM assets_application_service_log_setting WHERE retention_tier_id = sqlc.arg(retention_tier_id);

-- name: GetApplicationNameCode :one
SELECT COALESCE(name, ''), COALESCE(code, '') FROM assets_application WHERE id = sqlc.arg(id);

-- name: GetApplicationServiceCode :one
SELECT code FROM assets_application_service WHERE id = sqlc.arg(id);

-- ---- P2-3：通用配置资源的写路径（原实现运行时拼表名与列名）----
-- 表名按资源分派成显式语句；"只写提交了的列"这一 PATCH 语义改由应用层承担：
-- 更新前读回整行 → 合并提交的字段 → 整行写（与 inspection 组 PATCH 同一手法）。
-- 这样做的前提是这三张表的可写列都在 Get* 查询的列集里（已确认）。

-- name: CreateLogRetentionTier :execlastid
INSERT INTO monitor_log_retention_tier
  (create_time,update_time,code,name,daily_size_gb,retention_days,rollover_min_index_age,enabled,is_default,remark)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.arg(code),sqlc.arg(name),sqlc.arg(daily_size_gb),
        sqlc.arg(retention_days),sqlc.arg(rollover_min_index_age),sqlc.arg(enabled),sqlc.arg(is_default),
        sqlc.arg(remark));

-- name: UpdateLogRetentionTier :execrows
UPDATE monitor_log_retention_tier
SET update_time=sqlc.arg(update_time),code=sqlc.arg(code),name=sqlc.arg(name),daily_size_gb=sqlc.arg(daily_size_gb),
    retention_days=sqlc.arg(retention_days),rollover_min_index_age=sqlc.arg(rollover_min_index_age),
    enabled=sqlc.arg(enabled),is_default=sqlc.arg(is_default),remark=sqlc.arg(remark)
WHERE id=sqlc.arg(id);

-- name: DeleteLogRetentionTier :execresult
DELETE FROM monitor_log_retention_tier WHERE id = sqlc.arg(id);

-- name: ClearDefaultLogRetentionTier :exec
UPDATE monitor_log_retention_tier SET is_default=FALSE WHERE id <> sqlc.arg(id);

-- name: CreateLogProcessingRule :execlastid
INSERT INTO monitor_log_processing_rule
  (create_time,update_time,remark,name,description,input_format,multiline_enabled,start_pattern,
   continuation_pattern,sample_log,flush_timeout,pipeline_body,cluster_id,application_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(description),
        sqlc.arg(input_format),sqlc.arg(multiline_enabled),sqlc.arg(start_pattern),
        sqlc.arg(continuation_pattern),sqlc.arg(sample_log),sqlc.arg(flush_timeout),sqlc.arg(pipeline_body),
        sqlc.arg(cluster_id),sqlc.narg(application_id));

-- name: UpdateLogProcessingRule :execrows
UPDATE monitor_log_processing_rule
SET update_time=sqlc.arg(update_time),remark=sqlc.narg(remark),name=sqlc.arg(name),description=sqlc.arg(description),
    input_format=sqlc.arg(input_format),multiline_enabled=sqlc.arg(multiline_enabled),
    start_pattern=sqlc.arg(start_pattern),continuation_pattern=sqlc.arg(continuation_pattern),
    sample_log=sqlc.arg(sample_log),
    flush_timeout=sqlc.arg(flush_timeout),pipeline_body=sqlc.arg(pipeline_body),
    cluster_id=sqlc.arg(cluster_id),application_id=sqlc.narg(application_id)
WHERE id=sqlc.arg(id);

-- name: DeleteLogProcessingRule :execresult
DELETE FROM monitor_log_processing_rule WHERE id = sqlc.arg(id);

-- name: CreateLogCollectionFilterRule :execlastid
INSERT INTO monitor_log_collection_filter_rule
  (create_time,update_time,remark,name,description,pattern,enabled,application_id)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.narg(remark),sqlc.arg(name),sqlc.arg(description),
        sqlc.arg(pattern),sqlc.arg(enabled),sqlc.narg(application_id));

-- name: UpdateLogCollectionFilterRule :execrows
UPDATE monitor_log_collection_filter_rule
SET update_time=sqlc.arg(update_time),remark=sqlc.narg(remark),name=sqlc.arg(name),description=sqlc.arg(description),
    pattern=sqlc.arg(pattern),enabled=sqlc.arg(enabled),application_id=sqlc.narg(application_id)
WHERE id=sqlc.arg(id);

-- name: DeleteLogCollectionFilterRule :execresult
DELETE FROM monitor_log_collection_filter_rule WHERE id = sqlc.arg(id);

-- ---- 日志链路对账与数据流水位（只读）----

-- name: ListProcessingRulesByCluster :many
SELECT name, pipeline_body, application_id FROM monitor_log_processing_rule
WHERE cluster_id = sqlc.arg(cluster_id) ORDER BY name;

-- 逻辑服务**当前生效**的档位集合（服务 × 档位对，可能多条）。
--
-- 用途：判定一条已有的 data stream 是不是"改档位后留下的历史流"——流的档位不在该服务的生效集合里
-- 就说明它已停止写入（数据按原档位保留到期）。判定放后端做，因为：
--   - 全局视图（不按服务收窄）没有"本服务生效档位"这份数据，前端推不出来；
--   - 生效档位是"覆盖档位 → 服务默认档位 → is_default → 'std'"的 COALESCE 链，必须与
--     ListServiceTemplateLogs 的 tier_code 逐字一致，两处各写一份必然漂移。
-- 没有任何日志定义的服务不会出现在结果里 → 它的所有档位都不生效 → 存量流全部判为历史流
-- （与"这个服务现在什么都不采"一致）。
-- name: ListServiceActiveStreamTiers :many
SELECT DISTINCT s.code AS service_code,
       COALESCE(tier.code, (SELECT code FROM monitor_log_retention_tier WHERE is_default = TRUE ORDER BY id LIMIT 1), 'std') AS tier_code
FROM assets_application_service s
JOIN assets_application_log_definition ld ON ld.deployment_template_id = s.deployment_template_id
LEFT JOIN assets_application_service_log_setting ls ON ls.service_id = s.id AND ls.log_definition_id = ld.id
LEFT JOIN monitor_log_retention_tier tier ON tier.id = COALESCE(ls.retention_tier_id, s.log_retention_tier_id)
WHERE s.code <> '';

-- 服务级下发的解析：该服务**承载在哪些主机上**，以及每台主机对应的采集目标。
--
-- 三个刻意的取舍：
--  1. **不过滤启用态**（sd.enabled / d.enabled / s.enabled / s.log_collection_enabled 都不滤）：
--     停用服务或关掉采集之后，恰恰需要下发一次来**移除**主机上的片段——渲染层会把停用的服务排除，
--     agent 侧"没交付的 .yml 一律删除"，于是旧片段被清掉。按启用态过滤主机，停用的服务就永远清不干净。
--     变化后的主机是否需要真下发由渲染指纹决定（一致则跳过，见 applyLogTargetConfigRow）。
--  2. 只在 LEFT JOIN 里要求 `managed_enabled = TRUE`：没纳管（或已停用纳管）的主机没有采集目标行，
--     下发不了，必须能被识别出来告诉用户（target_id 为 NULL），而不是静默少下发几台。
--  3. SELECT DISTINCT：入参是服务，它可能在同一台主机上有多个部署实例，而去重后每一列都是主机级事实。
-- name: ListServiceLogApplyTargets :many
SELECT DISTINCT d.host_id, COALESCE(h.ip, '') AS host_ip,
       COALESCE(h.instance_name, '') AS host_instance_name,
       l.id AS target_id, COALESCE(l.config_fingerprint, '') AS config_fingerprint
FROM assets_application_service_deployment sd
JOIN assets_application_deployment d ON d.id = sd.deployment_id
JOIN assets_host h ON h.id = d.host_id
LEFT JOIN monitor_log_collection_target l ON l.host_id = d.host_id AND l.managed_enabled = TRUE
WHERE sd.service_id = sqlc.arg(service_id)
ORDER BY d.host_id;

-- name: ListManagedLogTargetConfigs :many
SELECT l.id, l.host_id, COALESCE(h.ip, ''), l.agent_installed, COALESCE(l.config_fingerprint, '')
FROM monitor_log_collection_target l JOIN assets_host h ON h.id = l.host_id
WHERE l.managed_enabled = TRUE ORDER BY l.id;

-- name: ListInstalledLogTargetRuntime :many
SELECT l.id, COALESCE(h.ip, ''), COALESCE(l.runtime_status, ''), COALESCE(l.last_error, '')
FROM monitor_log_collection_target l JOIN assets_host h ON h.id = l.host_id
WHERE l.managed_enabled = TRUE AND l.agent_installed = TRUE ORDER BY l.id;

-- 流名匹配候选：**全部**逻辑服务的维度码（新命名 = 项目-业务系统-环境-逻辑服务-档位；
-- 旧命名 = 项目-环境-业务系统-档位，业务系统/环境段序为调整前的旧段序）。
--
-- 刻意**不**过滤 `s.enabled`：这条查询回答的是"这条已有的流属于哪个已知服务"，
-- 而不是"这个服务现在是否在采集"。停用是可逆状态、服务行还在、存量流要么还在写
-- （还没重新下发配置）要么停写并保留到 ILM 到期（见计划 §0「停止采集 ≠ 删除数据」）。
-- 早先带 `WHERE s.enabled = TRUE` 会让停用服务的流解析不出来 → 在存储水位页被判成
-- 「未识别」孤儿，等于把暂停采集的存量数据标成待清理对象（2026-09-18 修复）。
-- 真正被判成孤儿的应该是"服务行已删/改名"——那种情况下这里本来就查不到维度码。
--
-- 下发路径（ListHostLogRenderEntries）仍照旧过滤 `s.enabled = TRUE`：停用的服务不该再往主机推片段。
-- 附带服务级的采集开关（enabled / log_collection_enabled）：存储水位页要按**逻辑服务**标注
-- "已停用 / 未开启采集"——这是配置事实（来自库），不是对数据流的断言，所以不需要查 ES。
-- name: ListServiceStreamDims :many
SELECT DISTINCT p.code AS project_code, e.code AS environment_code, bs.code AS business_system_code,
       s.code AS service_code, COALESCE(t.code, '') AS tier_code,
       s.enabled AS service_enabled, s.log_collection_enabled
FROM assets_application_service s
JOIN assets_business_system bs ON bs.id = s.business_system_id
JOIN assets_project p ON p.id = bs.project_id
JOIN assets_business_environment e ON e.id = s.environment_id
LEFT JOIN monitor_log_retention_tier t ON t.id = s.log_retention_tier_id;

-- 单个逻辑服务的流名维度码（清理数据流用：按服务解析 <project>-<business>-<env>-<service>-* 模式）。
-- name: GetApplicationServiceStreamDims :one
SELECT p.code AS project_code, e.code AS environment_code, bs.code AS business_system_code, s.code AS service_code
FROM assets_application_service s
JOIN assets_business_system bs ON bs.id = s.business_system_id
JOIN assets_project p ON p.id = bs.project_id
JOIN assets_business_environment e ON e.id = s.environment_id
WHERE s.id = sqlc.arg(id);

-- name: ListEnabledProjects :many
SELECT id, code, name FROM assets_project WHERE enabled = TRUE ORDER BY name;

-- name: ListEnabledBusinessSystems :many
SELECT id, code, name, project_id FROM assets_business_system WHERE enabled = TRUE ORDER BY name;

-- name: ListEnabledBusinessEnvironments :many
SELECT id, code, name FROM assets_business_environment WHERE enabled = TRUE ORDER BY `order`, name;

-- 服务维度元数据（编码维度，便于与流名对齐）：存储水位页要"把每一层的成员都列出来（含没有任何
-- 日志的）"，光靠流里解析出的服务码做不到——那样没日志的服务就不出现在统计里。
-- 环境是**服务上的属性**（assets_business_environment 不挂在业务系统下），所以"某业务系统下的环境"
-- 只能由它名下服务反推，这也需要这份数据。
-- name: ListEnabledServiceStreamDims :many
SELECT s.id, s.code, s.name, bs.code AS business_system, COALESCE(e.code, '') AS environment
FROM assets_application_service s
JOIN assets_business_system bs ON bs.id = s.business_system_id
LEFT JOIN assets_business_environment e ON e.id = s.environment_id
WHERE s.enabled = TRUE
ORDER BY s.name;

-- ---- 模块总览 / Prometheus 服务发现 / 机器令牌校验 ----

-- Prometheus 基地址：只要 value 一列（放本域而不是 sys_config.sql，因为调用方在 monitor；
-- 也不复用 GetConfigByKey —— 那条是 SELECT *，为读一个配置值拖回整行没必要）。
-- 参数名不能叫 key（P5 陷阱 23：命名参数与保留字相撞会让 MySQL 引擎语法错误）。
-- name: GetConfigValueByKey :one
SELECT value FROM sys_config WHERE `key` = sqlc.arg(config_key) ORDER BY id LIMIT 1;

-- name: CountMonitorTargetSummary :one
-- 计数用 COUNT(CASE WHEN …) 而不是 SUM(布尔)：PG 里布尔不能求和（同 inspection/automation 的处理）。
SELECT COUNT(*) AS total,
       COUNT(CASE WHEN managed_enabled THEN 1 END) AS managed_enabled,
       COUNT(CASE WHEN install_status='success' THEN 1 END) AS install_success,
       COUNT(CASE WHEN last_scrape_status='up' THEN 1 END) AS scrape_up
FROM monitor_target;

-- name: ListPrometheusServiceDiscoveryTargets :many
SELECT t.exporter_type, t.scrape_port, h.id, h.instance_name, h.ip
FROM monitor_target t JOIN assets_host h ON h.id = t.host_id
WHERE t.managed_enabled = TRUE AND t.install_status = 'success' AND h.ip IS NOT NULL
ORDER BY t.id DESC;

-- 机器令牌校验：过期判定改成应用层传时间（原实现用 UTC_TIMESTAMP(6)）。
-- name: ListActiveAgentTokens :many
SELECT id, token_hash FROM sys_agent_token
WHERE is_active = TRUE AND (expires_at IS NULL OR expires_at > sqlc.arg(now));

-- name: MarkAgentTokenUsed :exec
UPDATE sys_agent_token SET last_used_at = sqlc.arg(last_used_at) WHERE id = sqlc.arg(id);

-- ---- 日志采集批量动作的作业与进度（monitor_log_batch_job / _item）----
-- 见 migration 000033 与 docs/plans/LOG_COLLECTION_LIFECYCLE.md §8 Phase 2。
-- 状态机：作业 pending → running → success|partial|failed；item pending → running → success|failed。
-- 计数不应用层自增，而是从 item 表重算（RefreshLogBatchJobProgress），避免并发下计数漂移。

-- name: CreateLogBatchJob :execlastid
INSERT INTO monitor_log_batch_job
  (create_time,update_time,remark,action,status,total_count,success_count,failed_count,concurrency,message,
   requested_user_id,requested_username,started_at,finished_at)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),NULL,sqlc.arg(action),'pending',sqlc.arg(total_count),0,0,
        sqlc.arg(concurrency),'',sqlc.narg(requested_user_id),sqlc.arg(requested_username),NULL,NULL);

-- 首次认领（pending → running）：拿到 0 行说明作业已被别的分片认领或已结束，直接 ack。
--
-- 只认 pending、且**不用 "status IN (pending,running)"**：MySQL 的 UPDATE 默认返回"实际改变的行数"
-- （客户端未开 CLIENT_FOUND_ROWS），把已经在 running 的行再写一次同样值会返回 0 行，
-- 于是分片续跑会被误判成"抢不到执行权"而永远停在 running（真库验出，2026-07-XX 版本曾如此）。
-- 分片续跑不走这条语句：执行器先读作业状态，读到 running 就继续（进程内按作业 id 串行，
-- 见 log_batch_job.go 的说明）。
-- name: ClaimLogBatchJob :execrows
UPDATE monitor_log_batch_job
SET status='running', started_at=COALESCE(started_at, sqlc.arg(started_at)), message=sqlc.arg(message),
    update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND status='pending';

-- name: GetLogBatchJob :one
SELECT id, action, status, total_count, success_count, failed_count, concurrency, message,
       COALESCE(requested_username, ''), started_at, finished_at, create_time, update_time
FROM monitor_log_batch_job WHERE id=sqlc.arg(id);

-- 页面上挂着的"进行中的批量作业"（刷新页面后仍能接着看进度）。
-- name: GetActiveLogBatchJobByAction :one
SELECT id, action, status, total_count, success_count, failed_count, concurrency, message,
       COALESCE(requested_username, ''), started_at, finished_at, create_time, update_time
FROM monitor_log_batch_job
WHERE action=sqlc.arg(action) AND status IN ('pending','running')
ORDER BY id DESC LIMIT 1;

-- 批量作业明细的建 item 前一步：一次取回全部目标（主机名/IP 作为快照写入 item）。
-- name: ListLogBatchTargets :many
SELECT l.id, l.host_id, COALESCE(h.instance_name, ''), COALESCE(h.ip, '')
FROM monitor_log_collection_target l JOIN assets_host h ON h.id = l.host_id
WHERE l.id IN (sqlc.slice(target_ids));

-- name: CreateLogBatchJobItem :exec
INSERT INTO monitor_log_batch_job_item
  (create_time,update_time,batch_job_id,target_id,host_id,host_name,host_ip,status,message,started_at,finished_at)
VALUES (sqlc.arg(create_time),sqlc.arg(update_time),sqlc.arg(batch_job_id),sqlc.arg(target_id),sqlc.arg(host_id),
        sqlc.arg(host_name),sqlc.arg(host_ip),'pending','',NULL,NULL);

-- name: ListLogBatchJobItems :many
SELECT id, target_id, host_id, host_name, host_ip, status, message, started_at, finished_at
FROM monitor_log_batch_job_item WHERE batch_job_id=sqlc.arg(batch_job_id) ORDER BY id;

-- 续跑/分片执行时取下一批待处理项。只取 pending：已成功的不重跑，running 的由
-- ResetStaleLogBatchJobItems 在续跑前回落为 pending。
-- name: ListPendingLogBatchJobItems :many
SELECT id, target_id, host_id, host_name, host_ip, status, message, started_at, finished_at
FROM monitor_log_batch_job_item
WHERE batch_job_id=sqlc.arg(batch_job_id) AND status='pending'
ORDER BY id LIMIT ?;

-- name: MarkLogBatchJobItemRunning :exec
UPDATE monitor_log_batch_job_item
SET status='running', started_at=COALESCE(started_at, sqlc.arg(started_at)), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- name: FinishLogBatchJobItem :exec
UPDATE monitor_log_batch_job_item
SET status=sqlc.arg(status), message=sqlc.arg(message), finished_at=sqlc.arg(finished_at), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id);

-- 续跑前把"上一轮留下的 running"落回 pending：这些项的执行者已经不在了。
-- name: ResetStaleLogBatchJobItems :execrows
UPDATE monitor_log_batch_job_item
SET status='pending', update_time=sqlc.arg(update_time)
WHERE batch_job_id=sqlc.arg(batch_job_id) AND status='running';

-- 计数从 item 表重算，不依赖应用层累加（并发下应用层累加必然漂移）。
-- name: RefreshLogBatchJobProgress :exec
UPDATE monitor_log_batch_job j
SET success_count=(SELECT COUNT(*) FROM monitor_log_batch_job_item i WHERE i.batch_job_id=j.id AND i.status='success'),
    failed_count=(SELECT COUNT(*) FROM monitor_log_batch_job_item i WHERE i.batch_job_id=j.id AND i.status='failed'),
    update_time=sqlc.arg(update_time)
WHERE j.id=sqlc.arg(id);

-- name: FinishLogBatchJob :execrows
UPDATE monitor_log_batch_job
SET status=sqlc.arg(status), message=sqlc.arg(message), finished_at=sqlc.arg(finished_at), update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND status='running';

-- name: UpdateLogBatchJobHeartbeat :exec
UPDATE monitor_log_batch_job SET message=sqlc.arg(message), update_time=sqlc.arg(update_time) WHERE id=sqlc.arg(id);

-- 失联作业对账：执行进程消失后作业会永久停在 running，由 reaper 回落为 pending 后重新入队。
-- 判据是 update_time：健康作业每完成一个 item 都会刷新它。
-- name: ListStaleLogBatchJobs :many
SELECT id, action FROM monitor_log_batch_job
WHERE status='running' AND update_time < sqlc.arg(stale_before) ORDER BY id;

-- 失联对账把停摆的作业放回 pending，重新投递后再由 ClaimLogBatchJob 认领。
-- name: RequeueLogBatchJob :execrows
UPDATE monitor_log_batch_job SET status='pending', update_time=sqlc.arg(update_time)
WHERE id=sqlc.arg(id) AND status='running';
