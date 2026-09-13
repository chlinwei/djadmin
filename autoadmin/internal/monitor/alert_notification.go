package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// 告警通知分发链路：对齐 Django alert_history.enqueue_notification + tasks.resolve_alert_media
// + tasks.send_alert_notification。链路为 webhook/对账提交事务 -> enqueue（有媒介预判 + 去重建
// monitor_alert_notification_event）-> 事务提交后的 goroutine 异步逐地址投递（SMTP 能力复用
// media_send.go 的 sendSMTPMedia）并落 monitor_alert_notification_delivery。

const (
	notificationEventPending = "pending"
	notificationEventSending = "sending"
	notificationEventSuccess = "success"
	notificationEventFailed  = "failed"

	// 与 Django send_alert_notification 的 max_retries=5 一致：首次尝试 + 最多 5 次重试。
	notificationMaxAttempts = 6
	// 失联对账阈值：last_seen_at 距今超过该时长视为告警已恢复（Prometheus resolution 语义的简化版）。
	reconcileStaleAfter = 10 * time.Minute
)

// alertNotificationMedia 是一次命中匹配后可投递的媒介（对齐 Django AlertMedia）。
type alertNotificationMedia struct {
	id        int64
	name      string
	mediaType string
	config    []byte
}

// alertNotificationTarget 描述一条待通知的告警：入库行关键字段 + 原始 labels。
type alertNotificationTarget struct {
	id        int64
	alertname string
	severity  string
	instance  string
	state     string
	labels    map[string]any
}

// smtpSender 抽出 SMTP 发信能力（默认 handler.sendSMTPMedia），便于单测注入假实现。
type smtpSender func(config map[string]any, subject, body string, recipients []string) (bool, string)

// mergeAlertLabels 对齐 Django tasks.resolve_alert_media 的 labels 预处理：
// labels 全部转字符串后合并 alertname/severity/instance 三个便捷键（覆盖同名 label）。
func mergeAlertLabels(labels map[string]any, alertname, severity, instance string) map[string]string {
	merged := map[string]string{}
	for key, value := range labels {
		merged[key] = stringValue(value)
	}
	merged["alertname"] = alertname
	merged["severity"] = severity
	merged["instance"] = instance
	return merged
}

// matchersMatch 对齐 Django 的全量等值匹配：matchers 中每个 key=value 都必须命中 labels。
func matchersMatch(matchers map[string]any, labels map[string]string) bool {
	for key, rawValue := range matchers {
		if labels[key] != stringValue(rawValue) {
			return false
		}
	}
	return true
}

// matchedAlertMedias 对齐 tasks.resolve_alert_media：仅取 enabled 路由，按事件类型检查
// notify_on_firing/notify_on_resolved，matchers 对合并后的 labels 做全量等值匹配，
// 命中路由收集其所有 enabled 媒介（monitor_alert_route_media 多对多）。
func (handler *Handler) matchedAlertMedias(target alertNotificationTarget, eventType string) ([]alertNotificationMedia, error) {
	rows, err := handler.db.Query(`SELECT r.matchers,m.id,m.name,m.media_type,m.config
FROM monitor_alert_route r
JOIN monitor_alert_route_media rm ON rm.alertroute_id=r.id
JOIN monitor_alert_media m ON m.id=rm.alertmedia_id
WHERE r.enabled=TRUE AND m.enabled=TRUE AND (CASE WHEN ?='firing' THEN r.notify_on_firing ELSE r.notify_on_resolved END)=TRUE
ORDER BY m.id`, eventType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	merged := mergeAlertLabels(target.labels, target.alertname, target.severity, target.instance)
	seen := map[int64]bool{}
	medias := make([]alertNotificationMedia, 0)
	for rows.Next() {
		var matchersJSON, config []byte
		media := alertNotificationMedia{}
		if err = rows.Scan(&matchersJSON, &media.id, &media.name, &media.mediaType, &config); err != nil {
			return nil, err
		}
		if seen[media.id] {
			continue
		}
		matchers := map[string]any{}
		_ = json.Unmarshal(matchersJSON, &matchers)
		if !matchersMatch(matchers, merged) {
			continue
		}
		seen[media.id] = true
		media.config = config
		medias = append(medias, media)
	}
	return medias, rows.Err()
}

// enqueueAlertNotification 对齐 enqueue_notification：先做“有无可投递媒介”预判（无命中不建
// 事件，避免注定失败的垃圾记录），再按 deduplication_key={alert_id}:{event_type} 幂等建事件。
// 返回是否新建了事件。
func (handler *Handler) enqueueAlertNotification(target alertNotificationTarget, eventType string) (bool, error) {
	medias, err := handler.matchedAlertMedias(target, eventType)
	if err != nil {
		return false, err
	}
	if len(medias) == 0 {
		return false, nil
	}
	now := time.Now().UTC()
	result, err := handler.db.Exec(`INSERT IGNORE INTO monitor_alert_notification_event(create_time,update_time,remark,event_type,deduplication_key,status,attempt_count,error_message,sent_at,alert_id) VALUES(?,?,NULL,?,?,'pending',0,'',NULL,?)`, now, now, eventType, fmt.Sprintf("%d:%s", target.id, eventType), target.id)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected == 0 {
		return false, err
	}
	return true, nil
}

// enqueueWebhookNotifications 供 webhook 在摄取事务提交后调用：逐条预判 + 去重建事件，
// 返回实际新建的事件数（预判无媒介的不算）与待派发的事件 id。
func (handler *Handler) enqueueWebhookNotifications(targets []alertNotificationTarget) (int, []int64) {
	enqueued := 0
	eventIDs := make([]int64, 0)
	for _, target := range targets {
		created, err := handler.enqueueAlertNotification(target, target.state)
		if err != nil {
			continue
		}
		if !created {
			continue
		}
		enqueued++
		var eventID int64
		if err = handler.db.QueryRow(`SELECT id FROM monitor_alert_notification_event WHERE deduplication_key=?`, fmt.Sprintf("%d:%s", target.id, target.state)).Scan(&eventID); err == nil {
			eventIDs = append(eventIDs, eventID)
		}
	}
	return enqueued, eventIDs
}

// dispatchAlertNotificationEvent 在事务提交后的 goroutine 中执行（webhook 响应不等待）。
// 对齐 tasks.send_alert_notification：单次尝试 + 指数退避重试（最多 5 次重试）。
func (handler *Handler) dispatchAlertNotificationEvent(eventID int64) {
	for attempt := 1; ; attempt++ {
		retryable, err := handler.sendAlertNotificationEvent(eventID)
		if err == nil && !retryable {
			return
		}
		if err != nil || !retryable || attempt >= notificationMaxAttempts {
			// 次数用尽或不可重试：状态已在 sendAlertNotificationEvent 内落为 failed。
			return
		}
		// 指数退避：10s、20s、40s、80s、160s，上限 300s（对齐 Django retry countdown）。
		backoff := time.Duration(10<<uint(attempt-1)) * time.Second
		if backoff > 300*time.Second {
			backoff = 300 * time.Second
		}
		time.Sleep(backoff)
	}
}

// sendAlertNotificationEvent 执行一次投递尝试，返回 (是否存在可重试错误, 查询/写库错误)。
func (handler *Handler) sendAlertNotificationEvent(eventID int64) (bool, error) {
	// 事件 + 告警关键字段一次取齐；event_type 决定路由匹配开关，labels 用于 matchers 匹配。
	var eventType string
	var attemptCount int
	var labelsJSON []byte
	target := alertNotificationTarget{}
	err := handler.db.QueryRow(`SELECT e.event_type,e.attempt_count,a.id,a.alertname,a.severity,a.instance,a.state,a.labels
FROM monitor_alert_notification_event e JOIN monitor_alert_history a ON a.id=e.alert_id WHERE e.id=?`, eventID).
		Scan(&eventType, &attemptCount, &target.id, &target.alertname, &target.severity, &target.instance, &target.state, &labelsJSON)
	if err != nil {
		return false, err
	}
	target.labels = unmarshalAlertLabels(labelsJSON)

	var status string
	if err = handler.db.QueryRow(`SELECT status FROM monitor_alert_notification_event WHERE id=?`, eventID).Scan(&status); err != nil {
		return false, err
	}
	if status == notificationEventSuccess {
		return false, nil
	}

	medias, err := handler.matchedAlertMedias(target, eventType)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	if len(medias) == 0 {
		// 入队时已筛过一次，走到这里说明媒介在入队后被停用或路由被改；不可重试。
		_, _ = handler.db.Exec(`UPDATE monitor_alert_notification_event SET status='failed',error_message='媒介在入队后被停用或路由已变更',update_time=? WHERE id=?`, now, eventID)
		return false, nil
	}
	if _, err = handler.db.Exec(`UPDATE monitor_alert_notification_event SET status='sending',attempt_count=attempt_count+1,update_time=? WHERE id=?`, now, eventID); err != nil {
		return false, err
	}

	errors := make([]string, 0)
	retryable := false
	deliveryCount := 0
	for _, media := range medias {
		if media.mediaType != "email" {
			// 当前仅实现 Email，其余媒介类型后续补充。
			continue
		}
		// 每个绑定 = 一个用户在该媒介上的收件人列表。
		type bindingRow struct {
			userID     int64
			username   string
			recipients []byte
			scope      []byte
		}
		rows, err := handler.db.Query(`SELECT b.user_id,u.username,b.recipients,b.scope FROM monitor_user_alert_media_binding b JOIN sys_user u ON u.id=b.user_id WHERE b.media_id=? AND b.enabled=TRUE ORDER BY b.id`, media.id)
		if err != nil {
			return false, err
		}
		bindings := make([]bindingRow, 0)
		for rows.Next() {
			var binding bindingRow
			if err = rows.Scan(&binding.userID, &binding.username, &binding.recipients, &binding.scope); err != nil {
				rows.Close()
				return false, err
			}
			bindings = append(bindings, binding)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return false, err
		}
		// 订阅范围过滤（路线三）：scope 为 NULL = 全局；否则需与告警主机的服务树归属节点有交集。
		scopeNodes := handler.alertScopeNodes(target)
		filtered := make([]bindingRow, 0, len(bindings))
		for _, binding := range bindings {
			if bindingMatchesScope(binding.scope, scopeNodes) {
				filtered = append(filtered, binding)
			}
		}
		bindings = filtered
		if len(bindings) == 0 {
			errors = append(errors, fmt.Sprintf("媒介 %q 没有任何用户绑定", media.name))
			continue
		}

		var config map[string]any
		_ = json.Unmarshal(media.config, &config)
		for _, binding := range bindings {
			var recipientsRaw []any
			_ = json.Unmarshal(binding.recipients, &recipientsRaw)
			// dict.fromkeys 语义：去重且保持顺序。
			seen := map[string]bool{}
			for _, rawRecipient := range recipientsRaw {
				recipient := strings.TrimSpace(stringValue(rawRecipient))
				if recipient == "" || seen[recipient] {
					continue
				}
				seen[recipient] = true
				deliveryCount++
				handler.deliverAlertNotification(eventID, media, binding.userID, binding.username, recipient, config, target, &errors, &retryable)
			}
		}
	}

	if deliveryCount == 0 && len(errors) == 0 {
		errors = append(errors, "匹配的告警媒介没有可投递的用户地址")
	}

	now = time.Now().UTC()
	if len(errors) == 0 {
		_, err = handler.db.Exec(`UPDATE monitor_alert_notification_event SET status='success',sent_at=?,error_message='',update_time=? WHERE id=?`, now, now, eventID)
		return false, err
	}
	errorMessage := strings.Join(errors, "; ")
	if retryable && attemptCount+1 < notificationMaxAttempts {
		// 还有重试机会：事件回到 pending（退避由 dispatchAlertNotificationEvent 控制）。
		_, err = handler.db.Exec(`UPDATE monitor_alert_notification_event SET status='pending',error_message=?,update_time=? WHERE id=?`, errorMessage, now, eventID)
		return true, err
	}
	_, err = handler.db.Exec(`UPDATE monitor_alert_notification_event SET status='failed',error_message=?,update_time=? WHERE id=?`, errorMessage, now, eventID)
	return false, err
}

// deliverAlertNotification 投递单个地址：get-or-create delivery（唯一键 event,media,user,address），
// 已 success 的跳过，否则置 sending、attempt_count+1，逐地址发信并按结果落 success/failed。
func (handler *Handler) deliverAlertNotification(eventID int64, media alertNotificationMedia, userID int64, username, recipient string, config map[string]any, target alertNotificationTarget, errors *[]string, retryable *bool) {
	now := time.Now().UTC()
	// get-or-create：撞唯一键时 LAST_INSERT_ID(id) 让 RowsAffected/LastInsertId 返回既有行 id。
	result, err := handler.db.Exec(`INSERT INTO monitor_alert_notification_delivery(create_time,update_time,remark,address,status,attempt_count,error_message,sent_at,event_id,media_id,user_id) VALUES(?,?,NULL,?,'pending',0,'',NULL,?,?,?)
ON DUPLICATE KEY UPDATE id=LAST_INSERT_ID(id)`, now, now, recipient, eventID, media.id, userID)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %v", username, media.name, recipient, err))
		*retryable = true
		return
	}
	deliveryID, err := result.LastInsertId()
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %v", username, media.name, recipient, err))
		*retryable = true
		return
	}

	var deliveryStatus string
	if err = handler.db.QueryRow(`SELECT status FROM monitor_alert_notification_delivery WHERE id=?`, deliveryID).Scan(&deliveryStatus); err != nil {
		*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %v", username, media.name, recipient, err))
		*retryable = true
		return
	}
	if deliveryStatus == notificationEventSuccess {
		return
	}
	if _, err = handler.db.Exec(`UPDATE monitor_alert_notification_delivery SET status='sending',attempt_count=attempt_count+1,error_message='',update_time=? WHERE id=?`, now, deliveryID); err != nil {
		*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %v", username, media.name, recipient, err))
		*retryable = true
		return
	}

	success, errorMessage := handler.smtpSend(config, buildAlertEmailSubject(target), buildAlertEmailBody(target), []string{recipient})
	now = time.Now().UTC()
	if success {
		_, _ = handler.db.Exec(`UPDATE monitor_alert_notification_delivery SET status='success',sent_at=?,error_message='',update_time=? WHERE id=?`, now, now, deliveryID)
		return
	}
	_, _ = handler.db.Exec(`UPDATE monitor_alert_notification_delivery SET status='failed',error_message=?,update_time=? WHERE id=?`, errorMessage, now, deliveryID)
	*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %s", username, media.name, recipient, errorMessage))
	*retryable = true
}

// buildAlertEmailSubject / buildAlertEmailBody 对齐 Django _send_email_alert 的邮件模板。
func buildAlertEmailSubject(target alertNotificationTarget) string {
	stateLabel := "Firing"
	if target.state == "resolved" {
		stateLabel = "Resolved"
	}
	severity := target.severity
	if severity == "" {
		severity = "unknown"
	}
	alertname := target.alertname
	if alertname == "" {
		alertname = "Unknown Alert"
	}
	return fmt.Sprintf("[%s] %s - %s", stateLabel, alertname, severity)
}

func buildAlertEmailBody(target alertNotificationTarget) string {
	stateLabel := "Firing"
	if target.state == "resolved" {
		stateLabel = "Resolved"
	}
	instance := target.instance
	if instance == "" {
		instance = "N/A"
	}
	labelsJSON, _ := json.Marshal(nonNilMap(target.labels))
	return fmt.Sprintf(`Alert: %s
Severity: %s
State: %s
Instance: %s

Labels: %s
`, target.alertname, target.severity, stateLabel, instance, string(labelsJSON))
}

// unmarshalAlertLabels 把 json 列的 labels 解为 map，坏数据退化为空 map（匹配等于无便捷键外命中）。
func unmarshalAlertLabels(raw []byte) map[string]any {
	labels := map[string]any{}
	_ = json.Unmarshal(raw, &labels)
	return labels
}

// reconcileStaleAlertsLoop 失联对账 ticker：单实例部署，无需分布式锁——进程内唯一 goroutine，
// 进程退出即随进程终止，不做优雅停止。
func (handler *Handler) reconcileStaleAlertsLoop() {
	// 首次 tick 延迟 1 分钟，避免与进程启动抢资源。
	time.Sleep(time.Minute)
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		handler.reconcileStaleAlerts()
	}
}

// reconcileStaleAlerts 把 firing 且 last_seen_at 超过阈值（简化为 10 分钟，取 Prometheus
// resolution/for 语义）的告警置 resolved，并对开启 notify_on_resolved 的命中路由走同一
// enqueue -> 发送链路。每次 tick 收集到的事件在事务提交后统一异步派发。
func (handler *Handler) reconcileStaleAlerts() {
	rows, err := handler.db.Query(`SELECT id,alertname,severity,instance,labels FROM monitor_alert_history WHERE state='firing' AND source='prometheus' AND last_seen_at < UTC_TIMESTAMP(6) - INTERVAL ? MINUTE`, int(reconcileStaleAfter.Minutes()))
	if err != nil {
		return
	}
	stale := make([]alertNotificationTarget, 0)
	for rows.Next() {
		target := alertNotificationTarget{}
		var labelsJSON []byte
		if err = rows.Scan(&target.id, &target.alertname, &target.severity, &target.instance, &labelsJSON); err != nil {
			rows.Close()
			return
		}
		target.labels = unmarshalAlertLabels(labelsJSON)
		stale = append(stale, target)
	}
	rows.Close()
	if err = rows.Err(); err != nil || len(stale) == 0 {
		return
	}

	eventIDs := make([]int64, 0)
	for _, target := range stale {
		now := time.Now().UTC()
		if _, err = handler.db.Exec(`UPDATE monitor_alert_history SET state='resolved',resolved_at=?,resolved_by_reconciliation=TRUE,update_time=? WHERE id=? AND state='firing'`, now, now, target.id); err != nil {
			continue
		}
		target.state = "resolved"
		if enqueued, enqueueErr := handler.enqueueAlertNotification(target, "resolved"); enqueueErr == nil && enqueued {
			var eventID int64
			if scanErr := handler.db.QueryRow(`SELECT id FROM monitor_alert_notification_event WHERE deduplication_key=?`, fmt.Sprintf("%d:resolved", target.id)).Scan(&eventID); scanErr == nil {
				eventIDs = append(eventIDs, eventID)
			}
		}
	}
	for _, eventID := range eventIDs {
		go handler.dispatchAlertNotificationEvent(eventID)
	}
}

// alertScopeNodes 解析告警主机的服务树归属节点（key 形如 "project:3"）。
// labels 无 host_id 或主机未挂任何服务时返回空集——只有全局订阅绑定的用户能收到。
// 解析结果不缓存：每个事件每次派发各查一次（告警频率低，表很小）。
func (handler *Handler) alertScopeNodes(target alertNotificationTarget) map[string]bool {
	nodes := map[string]bool{}
	hostID := int64(0)
	if raw, ok := target.labels["host_id"]; ok {
		_, _ = fmt.Sscan(strings.TrimSpace(stringValue(raw)), &hostID)
	}
	if hostID <= 0 {
		return nodes
	}
	rows, err := handler.db.Query(`SELECT DISTINCT s.id, s.business_system_id, s.environment_id, bs.project
		FROM assets_application_deployment d
		JOIN assets_application_service_deployment sd ON sd.deployment_id=d.id
		JOIN assets_application_service s ON s.id=sd.service_id
		JOIN assets_business_system bs ON bs.id=s.business_system_id
		WHERE d.host_id=? AND d.enabled=TRUE AND sd.enabled=TRUE`, hostID)
	if err != nil {
		return nodes
	}
	defer rows.Close()
	for rows.Next() {
		var serviceID, businessID int64
		var environmentID, projectID sql.NullInt64
		if err = rows.Scan(&serviceID, &businessID, &environmentID, &projectID); err != nil {
			return nodes
		}
		nodes[fmt.Sprintf("service:%d", serviceID)] = true
		nodes[fmt.Sprintf("business:%d", businessID)] = true
		if environmentID.Valid {
			nodes[fmt.Sprintf("environment:%d", environmentID.Int64)] = true
		}
		if projectID.Valid {
			nodes[fmt.Sprintf("project:%d", projectID.Int64)] = true
		}
	}
	return nodes
}

// bindingMatchesScope：scope 为 NULL/空 = 全局订阅恒命中；
// 否则解析 scope 条目，任一 type:id 命中告警归属节点即命中。坏 JSON 按全局处理（不因数据问题静默吞掉通知）。
func bindingMatchesScope(scope []byte, nodes map[string]bool) bool {
	if len(scope) == 0 || string(scope) == "null" {
		return true
	}
	var entries []struct {
		Type string `json:"type"`
		ID   int64  `json:"id"`
	}
	if err := json.Unmarshal(scope, &entries); err != nil {
		return true
	}
	for _, entry := range entries {
		if entry.ID > 0 && nodes[fmt.Sprintf("%s:%d", entry.Type, entry.ID)] {
			return true
		}
	}
	return false
}
