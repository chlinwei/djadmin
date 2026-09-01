package monitor

import (
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
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		context.JSON(500, gin.H{"error": "persist alerts failed"})
		return
	}
	created, resolved, heartbeats := 0, 0, 0
	now := time.Now().UTC()
	for _, alert := range alerts {
		labelsJSON, _ := json.Marshal(nonNilMap(alert.Labels))
		annotationsJSON, _ := json.Marshal(nonNilMap(alert.Annotations))
		fingerprint := alertFingerprint(alert.Labels)
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
				_, err = tx.ExecContext(context, `UPDATE monitor_alert_history SET state='resolved',resolved_at=?,last_seen_at=?,annotations=?,resolved_by_reconciliation=FALSE,update_time=? WHERE id=?`, resolvedAt, now, annotationsJSON, now, openID)
				if err != nil {
					tx.Rollback()
					context.JSON(500, gin.H{"error": "persist alerts failed"})
					return
				}
				resolved++
			}
			continue
		}
		if selectErr == nil {
			_, err = tx.ExecContext(context, `UPDATE monitor_alert_history SET last_seen_at=?,labels=?,annotations=?,update_time=? WHERE id=?`, now, labelsJSON, annotationsJSON, now, openID)
			if err != nil {
				tx.Rollback()
				context.JSON(500, gin.H{"error": "persist alerts failed"})
				return
			}
			heartbeats++
			continue
		}
		startedAt := parseAlertTime(alert.StartsAt, now)
		_, err = tx.ExecContext(context, `INSERT INTO monitor_alert_history(create_time,update_time,remark,source,fingerprint,alertname,rule_group,rule_snapshot,severity,instance,labels,annotations,generator_url,state,started_at,resolved_at,last_seen_at,resolved_by_reconciliation) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'firing',?,NULL,?,FALSE)`, now, now, "", "prometheus", fingerprint, mapString(alert.Labels, "alertname"), "", []byte("{}"), mapString(alert.Labels, "severity"), mapString(alert.Labels, "instance"), labelsJSON, annotationsJSON, alert.GeneratorURL, startedAt, now)
		if err != nil {
			tx.Rollback()
			context.JSON(500, gin.H{"error": "persist alerts failed"})
			return
		}
		created++
	}
	if err = tx.Commit(); err != nil {
		context.JSON(500, gin.H{"error": "persist alerts failed"})
		return
	}
	context.JSON(200, gin.H{"status": "success", "created": created, "resolved": resolved, "heartbeats": heartbeats, "notifications": 0})
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
