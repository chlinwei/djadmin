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

		var openID int64
		selectErr := tx.QueryRowContext(context, `SELECT id FROM monitor_alert_history WHERE fingerprint=? AND state='firing' ORDER BY id DESC LIMIT 1 FOR UPDATE`, fingerprint).Scan(&openID)
		if selectErr != nil && selectErr != sql.ErrNoRows {
			tx.Rollback()
			context.JSON(500, gin.H{"error": "persist alerts failed"})
			return
		}
		if isResolved {
			if selectErr == nil {
				_, err = tx.ExecContext(context, `UPDATE monitor_alert_history SET state='resolved',resolved_at=?,last_seen_at=?,annotations=?,resolved_by_reconciliation=FALSE,update_time=?,rule_group=IF(rule_group='',?,rule_group),rule_snapshot=IF(IFNULL(JSON_LENGTH(rule_snapshot),0)=0,?,rule_snapshot) WHERE id=?`, resolvedAt, now, annotationsJSON, now, ruleGroup, ruleSnapshotJSON, openID)
				if err != nil {
					tx.Rollback()
					context.JSON(500, gin.H{"error": "persist alerts failed"})
					return
				}
				resolved++
				notificationTargets = append(notificationTargets, alertNotificationTarget{id: openID, alertname: alertname, severity: mapString(alert.Labels, "severity"), instance: mapString(alert.Labels, "instance"), state: "resolved", labels: nonNilMap(alert.Labels)})
			}
			continue
		}
		if selectErr == nil {
			_, err = tx.ExecContext(context, `UPDATE monitor_alert_history SET last_seen_at=?,labels=?,annotations=?,update_time=?,rule_group=IF(rule_group='',?,rule_group),rule_snapshot=IF(IFNULL(JSON_LENGTH(rule_snapshot),0)=0,?,rule_snapshot) WHERE id=?`, now, labelsJSON, annotationsJSON, now, ruleGroup, ruleSnapshotJSON, openID)
			if err != nil {
				tx.Rollback()
				context.JSON(500, gin.H{"error": "persist alerts failed"})
				return
			}
			heartbeats++
			continue
		}
		startedAt := parseAlertTime(alert.StartsAt, now)
		result, err := tx.ExecContext(context, `INSERT INTO monitor_alert_history(create_time,update_time,remark,source,fingerprint,alertname,rule_group,rule_snapshot,severity,instance,labels,annotations,generator_url,state,started_at,resolved_at,last_seen_at,resolved_by_reconciliation) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'firing',?,NULL,?,FALSE)`, now, now, "", "prometheus", fingerprint, mapString(alert.Labels, "alertname"), ruleGroup, ruleSnapshotJSON, mapString(alert.Labels, "severity"), mapString(alert.Labels, "instance"), labelsJSON, annotationsJSON, alert.GeneratorURL, startedAt, now)
		if err != nil {
			tx.Rollback()
			context.JSON(500, gin.H{"error": "persist alerts failed"})
			return
		}
		// 拿到新告警行 id，供通知事件 deduplication_key={alert_id}:{event_type} 使用。
		var newAlertID int64
		if lastInsertID, lastErr := result.LastInsertId(); lastErr == nil {
			newAlertID = lastInsertID
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
