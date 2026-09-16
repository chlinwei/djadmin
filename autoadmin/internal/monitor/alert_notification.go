package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// 告警通知分发链路（策略树版）：webhook/对账提交事务 -> enqueue（策略树预判 + 去重建
// monitor_alert_notification_event）-> 事务提交后的 goroutine 异步逐地址投递（SMTP 能力复用
// media_send.go 的 sendSMTPMedia）并落 monitor_alert_notification_delivery。
// 范围路由由管理员维护的通知策略树（notification_policy.go）决定；用户绑定只是收件配置。

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

// enqueueAlertNotification 对齐 enqueue_notification：先做“有无可投递媒介”预判（策略树未命中
// 或出口为空不建事件，避免注定失败的垃圾记录），再按 deduplication_key={alert_id}:{event_type}
// 幂等建事件。返回 (事件 id, 是否新建了事件)——原实现建完事件再按 deduplication_key 查一次 id，
// 现在直接由插入语句取回（MySQL 的 LastInsertId / PG 的 RETURNING，见 alert_event_dialect_*.go）。
func (handler *Handler) enqueueAlertNotification(target alertNotificationTarget, eventType string) (int64, bool, error) {
	medias, _, err := handler.matchedPolicyMedias(context.Background(), target, eventType)
	if err != nil {
		return 0, false, err
	}
	if len(medias) == 0 {
		return 0, false, nil
	}
	now := time.Now().UTC()
	return createAlertNotificationEventIfAbsent(context.Background(), db.New(handler.db), db.CreateAlertNotificationEventIfAbsentParams{
		CreateTime: now, UpdateTime: now, EventType: eventType,
		DeduplicationKey: dedupeKey(target.id, eventType), AlertID: target.id,
	})
}

// dedupeKey 是通知事件的去重键（对齐 Django 的 {alert_id}:{event_type}）。
func dedupeKey(alertID int64, eventType string) string {
	return fmt.Sprintf("%d:%s", alertID, eventType)
}

// enqueueWebhookNotifications 供 webhook 在摄取事务提交后调用：逐条预判 + 去重建事件，
// 返回实际新建的事件数（预判无媒介的不算）与待派发的事件 id。
func (handler *Handler) enqueueWebhookNotifications(targets []alertNotificationTarget) (int, []int64) {
	enqueued := 0
	eventIDs := make([]int64, 0)
	for _, target := range targets {
		eventID, created, err := handler.enqueueAlertNotification(target, target.state)
		if err != nil || !created {
			continue
		}
		enqueued++
		eventIDs = append(eventIDs, eventID)
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
	queries := db.New(handler.db)
	ctx := context.Background()
	// 事件 + 告警关键字段一次取齐（含事件状态与尝试次数，原实现分两条语句读同一行）：
	// event_type 决定路由匹配开关，labels 用于 matchers 匹配。
	dispatch, err := queries.GetAlertNotificationEventDispatch(ctx, eventID)
	if err != nil {
		return false, err
	}
	if dispatch.Status == notificationEventSuccess {
		return false, nil
	}
	eventType := dispatch.EventType
	attemptCount := int(dispatch.AttemptCount)
	target := alertNotificationTarget{
		id: dispatch.ID, alertname: dispatch.Alertname, severity: dispatch.Severity,
		instance: dispatch.Instance, state: dispatch.State,
		labels: unmarshalAlertLabels(dispatch.Labels),
	}

	medias, userGroupIDs, err := handler.matchedPolicyMedias(ctx, target, eventType)
	if err != nil {
		return false, err
	}
	now := time.Now().UTC()
	if len(medias) == 0 {
		// 入队时已筛过一次，走到这里说明媒介在入队后被停用或策略树已变更；不可重试。
		_ = queries.MarkAlertNotificationEventFailed(ctx, db.MarkAlertNotificationEventFailedParams{
			ErrorMessage: "媒介在入队后被停用或策略树已变更", UpdateTime: now, ID: eventID,
		})
		return false, nil
	}
	if err = queries.MarkAlertNotificationEventSending(ctx, db.MarkAlertNotificationEventSendingParams{
		UpdateTime: now, ID: eventID,
	}); err != nil {
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
		// 每个绑定 = 一个用户在该媒介上的收件人列表（范围路由由策略树统一决定，绑定不再过滤）。
		type bindingRow struct {
			userID     int32
			username   string
			recipients []byte
		}
		// 出口用户组限制：nil=不限组（媒介全部绑定）；否则仅组成员的绑定可收。
		bindings := make([]bindingRow, 0)
		if userGroupIDs != nil {
			rows, queryErr := queries.ListAlertMediaBindingsByMediaInUserGroups(ctx, db.ListAlertMediaBindingsByMediaInUserGroupsParams{
				MediaID: media.id, GroupIds: userGroupIDs,
			})
			if queryErr != nil {
				return false, queryErr
			}
			for _, row := range rows {
				bindings = append(bindings, bindingRow{userID: row.UserID, username: row.Username, recipients: row.Recipients})
			}
		} else {
			rows, queryErr := queries.ListAlertMediaBindingsByMedia(ctx, media.id)
			if queryErr != nil {
				return false, queryErr
			}
			for _, row := range rows {
				bindings = append(bindings, bindingRow{userID: row.UserID, username: row.Username, recipients: row.Recipients})
			}
		}
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
				handler.deliverAlertNotification(eventID, media, int64(binding.userID), binding.username, recipient, config, target, &errors, &retryable)
			}
		}
	}

	if deliveryCount == 0 && len(errors) == 0 {
		errors = append(errors, "匹配的告警媒介没有可投递的用户地址")
	}

	now = time.Now().UTC()
	if len(errors) == 0 {
		err = queries.MarkAlertNotificationEventSuccess(ctx, db.MarkAlertNotificationEventSuccessParams{
			SentAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: eventID,
		})
		return false, err
	}
	errorMessage := strings.Join(errors, "; ")
	if retryable && attemptCount+1 < notificationMaxAttempts {
		// 还有重试机会：事件回到 pending（退避由 dispatchAlertNotificationEvent 控制）。
		err = queries.MarkAlertNotificationEventPending(ctx, db.MarkAlertNotificationEventPendingParams{
			ErrorMessage: errorMessage, UpdateTime: now, ID: eventID,
		})
		return true, err
	}
	err = queries.MarkAlertNotificationEventFailed(ctx, db.MarkAlertNotificationEventFailedParams{
		ErrorMessage: errorMessage, UpdateTime: now, ID: eventID,
	})
	return false, err
}

// deliverAlertNotification 投递单个地址：get-or-create delivery（唯一键 event,media,user,address），
// 已 success 的跳过，否则置 sending、attempt_count+1，逐地址发信并按结果落 success/failed。
func (handler *Handler) deliverAlertNotification(eventID int64, media alertNotificationMedia, userID int64, username, recipient string, config map[string]any, target alertNotificationTarget, errors *[]string, retryable *bool) {
	queries := db.New(handler.db)
	ctx := context.Background()
	now := time.Now().UTC()
	// get-or-create：撞唯一键时把既有行 id 返回（MySQL 的 LAST_INSERT_ID(id) / PG 的
	// `DO UPDATE SET id=<表>.id RETURNING id`，见 SQL 源与派生 override）。
	deliveryID, err := queries.CreateAlertNotificationDeliveryOrGetID(ctx, db.CreateAlertNotificationDeliveryOrGetIDParams{
		CreateTime: now, UpdateTime: now, Address: recipient, EventID: eventID,
		MediaID: sql.NullInt64{Int64: media.id, Valid: true},
		UserID:  sql.NullInt32{Int32: int32(userID), Valid: true},
	})
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %v", username, media.name, recipient, err))
		*retryable = true
		return
	}

	deliveryStatus, err := queries.GetAlertNotificationDeliveryStatus(ctx, deliveryID)
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %v", username, media.name, recipient, err))
		*retryable = true
		return
	}
	if deliveryStatus == notificationEventSuccess {
		return
	}
	if err = queries.MarkAlertNotificationDeliverySending(ctx, db.MarkAlertNotificationDeliverySendingParams{
		UpdateTime: now, ID: deliveryID,
	}); err != nil {
		*errors = append(*errors, fmt.Sprintf("%s / %s / %s: %v", username, media.name, recipient, err))
		*retryable = true
		return
	}

	success, errorMessage := handler.smtpSend(config, buildAlertEmailSubject(target), buildAlertEmailBody(target), []string{recipient})
	now = time.Now().UTC()
	if success {
		_ = queries.MarkAlertNotificationDeliverySuccess(ctx, db.MarkAlertNotificationDeliverySuccessParams{
			SentAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: deliveryID,
		})
		return
	}
	_ = queries.MarkAlertNotificationDeliveryFailed(ctx, db.MarkAlertNotificationDeliveryFailedParams{
		ErrorMessage: errorMessage, UpdateTime: now, ID: deliveryID,
	})
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
//
// 阈值改成应用层算好的时间点（原实现是 `UTC_TIMESTAMP(6) - INTERVAL ? MINUTE`：既是方言函数，
// 又让判定跟着库时钟走）；只有真的把 state 从 firing 翻成 resolved（影响行数 1）才入队通知，
// 已被别处恢复过的行跳过（原来无论如何都入队，靠 deduplication_key 兜住重复）。
func (handler *Handler) reconcileStaleAlerts() {
	queries := db.New(handler.db)
	ctx := context.Background()
	stale, err := queries.ListStaleFiringAlerts(ctx, time.Now().UTC().Add(-reconcileStaleAfter))
	if err != nil || len(stale) == 0 {
		return
	}

	eventIDs := make([]int64, 0)
	for _, row := range stale {
		now := time.Now().UTC()
		affected, err := queries.ResolveStaleAlert(ctx, db.ResolveStaleAlertParams{
			ResolvedAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: row.ID,
		})
		if err != nil || affected == 0 {
			continue
		}
		target := alertNotificationTarget{
			id: row.ID, alertname: row.Alertname, severity: row.Severity,
			instance: row.Instance, state: "resolved", labels: unmarshalAlertLabels(row.Labels),
		}
		if eventID, created, enqueueErr := handler.enqueueAlertNotification(target, "resolved"); enqueueErr == nil && created {
			eventIDs = append(eventIDs, eventID)
		}
	}
	for _, eventID := range eventIDs {
		go handler.dispatchAlertNotificationEvent(eventID)
	}
}

// alertScopeNodes 解析告警主机的服务树归属节点（key 形如 "project:3"），供策略树的
// tree matcher 使用。labels 无 host_id 或主机未挂任何服务时返回空集——只有不带 tree
// matcher 的策略（如默认策略）能接住这类告警。
// 解析结果不缓存：每个事件每次派发各查一次（告警频率低，表很小）。
//
// 注意（2026-09-16 修）：原实现的 SQL 里写的是 `bs.project`，而 assets_business_system
// 只有 project_id —— 语句在真库上恒报 1054（Unknown column），错误被这里的 `return nodes`
// 吞掉，于是 tree matcher 永远匹配不上（默认策略仍能收，按服务/环境/业务/项目路由的策略全静默失效）。
func (handler *Handler) alertScopeNodes(target alertNotificationTarget) map[string]bool {
	nodes := map[string]bool{}
	hostID := int64(0)
	if raw, ok := target.labels["host_id"]; ok {
		_, _ = fmt.Sscan(strings.TrimSpace(stringValue(raw)), &hostID)
	}
	if hostID <= 0 {
		return nodes
	}
	rows, err := db.New(handler.db).ListHostAlertScopeNodes(context.Background(), hostID)
	if err != nil {
		return nodes
	}
	for _, row := range rows {
		nodes[fmt.Sprintf("service:%d", row.ServiceID)] = true
		nodes[fmt.Sprintf("business:%d", row.BusinessSystemID)] = true
		if row.EnvironmentID.Valid {
			nodes[fmt.Sprintf("environment:%d", row.EnvironmentID.Int64)] = true
		}
		if row.ProjectID.Valid {
			nodes[fmt.Sprintf("project:%d", row.ProjectID.Int64)] = true
		}
	}
	return nodes
}
