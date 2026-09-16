package monitor

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

type webhookAlert struct {
	Labels       map[string]any `json:"labels"`
	Annotations  map[string]any `json:"annotations"`
	StartsAt     string         `json:"startsAt"`
	EndsAt       string         `json:"endsAt"`
	GeneratorURL string         `json:"generatorURL"`
}

func (handler *Handler) AlertWebhook(context *gin.Context) {
	var alerts []webhookAlert
	if err := context.ShouldBindJSON(&alerts); err != nil {
		context.JSON(200, gin.H{"status": "success", "created": 0, "resolved": 0, "heartbeats": 0, "notifications": 0})
		return
	}
	// 与 Django 版 ingest_alert_webhook_alerts 对齐：webhook 载荷不含规则定义，
	// 查一次 /api/v1/rules 建 fingerprint/alertname 索引，解析 rule_group 与含 query 的规则快照。
	ruleIndexes := handler.prometheusAlertRuleIndexes(context.Request.Context())
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		context.JSON(500, gin.H{"error": "persist alerts failed"})
		return
	}
	created, resolved, heartbeats := 0, 0, 0
	now := time.Now().UTC()
	// 事务内的语句走同一组 sqlc 查询（db.New 接受 *sql.Tx）。
	queries := db.New(tx)
	// 通知事件在事务提交后入队（对齐 Django transaction.on_commit），避免读连接看到未提交数据。
	notificationTargets := make([]alertNotificationTarget, 0)
	for _, alert := range alerts {
		labelsJSON, _ := json.Marshal(nonNilMap(alert.Labels))
		annotationsJSON, _ := json.Marshal(nonNilMap(alert.Annotations))
		fingerprint := alertFingerprint(alert.Labels)
		alertname := mapString(alert.Labels, "alertname")
		ruleGroup := ruleIndexes.byFingerprint[fingerprint]
		ruleDetails, hasRuleDetails := ruleIndexes.byAlertname[strings.TrimSpace(alertname)]
		if ruleGroup == "" {
			ruleGroup = ruleDetails.GroupName
		}
		ruleSnapshotJSON := []byte(`{}`)
		if hasRuleDetails {
			if encoded, marshalErr := json.Marshal(ruleDetails); marshalErr == nil {
				ruleSnapshotJSON = encoded
			}
		}
		resolvedAt, isResolved := resolvedTime(alert.EndsAt, now)

		// 同 fingerprint 的未恢复告警行：加锁读，顺带取回 rule_group/rule_snapshot。
		// 原实现把"已有值优先"写在 UPDATE 里（IF(rule_group='',?,rule_group) 与
		// IF(IFNULL(JSON_LENGTH(rule_snapshot),0)=0,?,rule_snapshot)），是 MySQL 方言函数；
		// 现在改成读回后在应用层合并（见 keepExistingRuleSnapshot）。
		open, selectErr := queries.GetOpenFiringAlertForUpdate(context, fingerprint)
		if selectErr != nil && selectErr != sql.ErrNoRows {
			tx.Rollback()
			context.JSON(500, gin.H{"error": "persist alerts failed"})
			return
		}
		if selectErr == nil {
			ruleSnapshotJSON = keepExistingRuleSnapshot(open.RuleSnapshot, ruleSnapshotJSON)
			if ruleGroup == "" {
				ruleGroup = open.RuleGroup
			}
		}
		if isResolved {
			if selectErr == nil {
				err = queries.ResolveAlertHistoryFromWebhook(context, db.ResolveAlertHistoryFromWebhookParams{
					ResolvedAt: sql.NullTime{Time: resolvedAt, Valid: true}, LastSeenAt: now,
					Annotations: annotationsJSON, UpdateTime: now, RuleGroup: ruleGroup,
					RuleSnapshot: ruleSnapshotJSON, ID: open.ID,
				})
				if err != nil {
					tx.Rollback()
					context.JSON(500, gin.H{"error": "persist alerts failed"})
					return
				}
				resolved++
				notificationTargets = append(notificationTargets, alertNotificationTarget{id: open.ID, alertname: alertname, severity: mapString(alert.Labels, "severity"), instance: mapString(alert.Labels, "instance"), state: "resolved", labels: nonNilMap(alert.Labels)})
			}
			continue
		}
		if selectErr == nil {
			err = queries.UpdateAlertHistoryHeartbeat(context, db.UpdateAlertHistoryHeartbeatParams{
				LastSeenAt: now, Labels: labelsJSON, Annotations: annotationsJSON,
				UpdateTime: now, RuleGroup: ruleGroup, RuleSnapshot: ruleSnapshotJSON, ID: open.ID,
			})
			if err != nil {
				tx.Rollback()
				context.JSON(500, gin.H{"error": "persist alerts failed"})
				return
			}
			heartbeats++
			continue
		}
		// 新告警行：主键由插入语句取回（MySQL 的 LastInsertId / PG 的 RETURNING，见 derive），
		// 供通知事件 deduplication_key={alert_id}:{event_type} 使用。
		newAlertID, err := queries.CreateAlertHistory(context, db.CreateAlertHistoryParams{
			CreateTime: now, UpdateTime: now, Source: "prometheus", Fingerprint: fingerprint,
			Alertname: mapString(alert.Labels, "alertname"), RuleGroup: ruleGroup,
			RuleSnapshot: ruleSnapshotJSON, Severity: mapString(alert.Labels, "severity"),
			Instance: mapString(alert.Labels, "instance"), Labels: labelsJSON, Annotations: annotationsJSON,
			GeneratorUrl: alert.GeneratorURL, StartedAt: parseAlertTime(alert.StartsAt, now), LastSeenAt: now,
		})
		if err != nil {
			tx.Rollback()
			context.JSON(500, gin.H{"error": "persist alerts failed"})
			return
		}
		created++
		notificationTargets = append(notificationTargets, alertNotificationTarget{id: newAlertID, alertname: alertname, severity: mapString(alert.Labels, "severity"), instance: mapString(alert.Labels, "instance"), state: "firing", labels: nonNilMap(alert.Labels)})
	}
	if err = tx.Commit(); err != nil {
		context.JSON(500, gin.H{"error": "persist alerts failed"})
		return
	}
	notifications, eventIDs := handler.enqueueWebhookNotifications(notificationTargets)
	for _, eventID := range eventIDs {
		go handler.dispatchAlertNotificationEvent(eventID)
	}
	context.JSON(200, gin.H{"status": "success", "created": created, "resolved": resolved, "heartbeats": heartbeats, "notifications": notifications})
}

// keepExistingRuleSnapshot 复刻原 SQL 里 `IF(IFNULL(JSON_LENGTH(rule_snapshot),0)=0, <新值>, rule_snapshot)`
// 的语义：既有值"有内容"时保留既有值，否则写新值。
// JSON_LENGTH 的等价判定：NULL / 无效 JSON / 空对象 / 空数组都算"没内容"（长度 0），
// 标量与非空容器算"有内容"。
func keepExistingRuleSnapshot(existing, incoming []byte) []byte {
	if !jsonValueHasContent(existing) {
		return incoming
	}
	return existing
}

func jsonValueHasContent(raw []byte) bool {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return false
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return false
	}
	switch typed := decoded.(type) {
	case map[string]any:
		return len(typed) > 0
	case []any:
		return len(typed) > 0
	default:
		return true
	}
}

func alertFingerprint(labels map[string]any) string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+mapString(labels, key))
	}
	digest := sha1.Sum([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(digest[:])
}

func resolvedTime(value string, now time.Time) (time.Time, bool) {
	if value == "" || value == "0001-01-01T00:00:00Z" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.After(now) {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}

func parseAlertTime(value string, fallback time.Time) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return fallback
	}
	return parsed.UTC()
}

func mapString(values map[string]any, key string) string {
	value, ok := values[key].(string)
	if !ok {
		return ""
	}
	return value
}

func nonNilMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	return value
}

// prometheusAlertRule 转存 Prometheus 规则定义里告警展示需要的字段（含 PromQL query）。
type prometheusAlertRule struct {
	GroupName   string         `json:"group_name"`
	Name        string         `json:"name"`
	Query       string         `json:"query"`
	Duration    any            `json:"duration"`
	Labels      map[string]any `json:"labels"`
	Annotations map[string]any `json:"annotations"`
}

type prometheusAlertRuleIndexes struct {
	byFingerprint map[string]string
	byAlertname   map[string]prometheusAlertRule
}

// prometheusAlertRuleIndexes 构建规则索引：fingerprint 来自各规则当前 active alerts 的
// labels（恢复后的历史告警不在 active 列表里，靠 alertname 兜底）。rules 接口失败时返回
// 空索引，不阻塞告警入库。
func (handler *Handler) prometheusAlertRuleIndexes(requestContext context.Context) prometheusAlertRuleIndexes {
	indexes := prometheusAlertRuleIndexes{
		byFingerprint: map[string]string{},
		byAlertname:   map[string]prometheusAlertRule{},
	}
	_, payload, err := handler.prometheusGet(requestContext, "/api/v1/rules", nil)
	if err != nil {
		return indexes
	}
	groups, _ := payload.dataMap()["groups"].([]any)
	for _, rawGroup := range groups {
		group, _ := rawGroup.(map[string]any)
		groupName := strings.TrimSpace(stringValue(group["name"]))
		rules, _ := group["rules"].([]any)
		for _, rawRule := range rules {
			rule, _ := rawRule.(map[string]any)
			name := strings.TrimSpace(stringValue(rule["name"]))
			if name != "" {
				details := prometheusAlertRule{
					GroupName:   groupName,
					Name:        name,
					Query:       stringValue(rule["query"]),
					Duration:    rule["duration"],
					Labels:      nonNilMap(rule["labels"].(map[string]any)),
					Annotations: nonNilMap(rule["annotations"].(map[string]any)),
				}
				if _, exists := indexes.byAlertname[name]; !exists {
					indexes.byAlertname[name] = details
				}
			}
			activeAlerts, _ := rule["alerts"].([]any)
			for _, rawAlert := range activeAlerts {
				alert, _ := rawAlert.(map[string]any)
				labels, _ := alert["labels"].(map[string]any)
				if len(labels) > 0 {
					indexes.byFingerprint[alertFingerprint(labels)] = groupName
				}
			}
		}
	}
	return indexes
}
