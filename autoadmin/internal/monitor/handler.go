package monitor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/api/response"
	"autoadmin/internal/assets"
	"autoadmin/internal/identity"

	"github.com/gin-gonic/gin"
)

const defaultPrometheusBaseURL = "http://10.25.66.150:9999"

type Handler struct {
	db          *sql.DB
	client      *http.Client
	gateway     *agent.Gateway
	secrets     *assets.SecretEncryptor
	packageRoot string
}

func NewHandler(db *sql.DB, gateway *agent.Gateway, encryptionKey, djangoSecret string) (*Handler, error) {
	secrets, err := assets.NewSecretEncryptor(encryptionKey, djangoSecret)
	if err != nil {
		return nil, err
	}
	// Both backends use Django's MEDIA_ROOT while the migration is in progress.
	packageRoot, err := filepath.Abs(filepath.Join("..", "backend", "djadmin", "media"))
	if err != nil {
		return nil, err
	}
	return &Handler{db: db, client: &http.Client{Timeout: 8 * time.Second}, gateway: gateway, secrets: secrets, packageRoot: packageRoot}, nil
}

func (handler *Handler) Summary(context *gin.Context) {
	var total, managedEnabled, installSuccess, scrapeUp int64
	err := handler.db.QueryRowContext(context, `SELECT COUNT(*),COALESCE(SUM(managed_enabled=1),0),COALESCE(SUM(install_status='success'),0),COALESCE(SUM(last_scrape_status='up'),0) FROM monitor_target`).Scan(&total, &managedEnabled, &installSuccess, &scrapeUp)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{
		"module": "monitor", "name": "智能监控", "status": "ready",
		"message": "智能监控模块已就绪，可在此扩展告警、巡检与AI分析能力。",
		"targets": gin.H{"total": total, "managed_enabled": managedEnabled, "install_success": installSuccess, "scrape_up": scrapeUp},
	})
}

func (handler *Handler) PrometheusOverview(context *gin.Context) {
	baseURL, payload, err := handler.prometheusGet(context, "/api/v1/targets", url.Values{"state": {"active"}})
	if err != nil {
		response.Success(context, gin.H{"status": "error", "prometheus_base_url": baseURL, "error": err.Error()})
		return
	}
	activeTargets, _ := payload.Data["activeTargets"].([]any)
	up := 0
	for _, rawTarget := range activeTargets {
		target, _ := rawTarget.(map[string]any)
		if strings.EqualFold(fmt.Sprint(target["health"]), "up") {
			up++
		}
	}
	response.Success(context, gin.H{"status": "success", "prometheus_base_url": baseURL, "targets": gin.H{"total": len(activeTargets), "up": up, "down": len(activeTargets) - up}, "warnings": payload.Warnings})
}

func (handler *Handler) PrometheusTargets(context *gin.Context) {
	baseURL, payload, err := handler.prometheusGet(context, "/api/v1/targets", url.Values{"state": {"active"}})
	if err != nil {
		response.Success(context, gin.H{"status": "error", "prometheus_base_url": baseURL, "error": err.Error(), "results": []any{}})
		return
	}
	activeTargets, _ := payload.Data["activeTargets"].([]any)
	results := make([]gin.H, 0, len(activeTargets))
	for _, rawTarget := range activeTargets {
		target, _ := rawTarget.(map[string]any)
		labels, _ := target["labels"].(map[string]any)
		results = append(results, gin.H{
			"scrape_pool": stringValue(target["scrapePool"]), "health": defaultString(target["health"], "unknown"),
			"job": stringValue(labels["job"]), "instance": stringValue(labels["instance"]),
			"last_error": stringValue(target["lastError"]), "last_scrape": stringValue(target["lastScrape"]), "scrape_url": stringValue(target["scrapeUrl"]),
		})
	}
	response.Success(context, gin.H{"status": "success", "prometheus_base_url": baseURL, "count": len(results), "results": results, "warnings": payload.Warnings})
}

func (handler *Handler) PrometheusAlerts(context *gin.Context) {
	baseURL, payload, err := handler.prometheusGet(context, "/api/v1/alerts", nil)
	if err != nil {
		response.Success(context, gin.H{"status": "error", "prometheus_base_url": baseURL, "error": err.Error(), "results": []any{}})
		return
	}
	alerts, _ := payload.Data["alerts"].([]any)
	results := make([]gin.H, 0, len(alerts))
	firingCount, resolvedCount := 0, 0
	for _, rawAlert := range alerts {
		alert, _ := rawAlert.(map[string]any)
		labels, _ := alert["labels"].(map[string]any)
		annotations, _ := alert["annotations"].(map[string]any)
		state := strings.ToLower(stringValue(alert["state"]))
		if state == "firing" {
			firingCount++
		} else if state == "resolved" {
			resolvedCount++
		}
		results = append(results, gin.H{"name": stringValue(labels["alertname"]), "severity": stringValue(labels["severity"]), "state": defaultString(state, "unknown"), "instance": stringValue(labels["instance"]), "labels": labels, "summary": defaultString(annotations["summary"], stringValue(annotations["description"])), "active_at": stringValue(alert["activeAt"]), "value": stringValue(alert["value"]), "history_id": nil, "notification_count": 0, "notification_delivery_count": 0, "notification_status": "none"})
	}
	response.Success(context, gin.H{"status": "success", "prometheus_base_url": baseURL, "count": len(results), "firing_count": firingCount, "resolved_count": resolvedCount, "results": results, "warnings": payload.Warnings})
}

func (handler *Handler) PrometheusRules(context *gin.Context) {
	baseURL, payload, err := handler.prometheusGet(context, "/api/v1/rules", nil)
	if err != nil {
		response.Success(context, gin.H{"status": "error", "prometheus_base_url": baseURL, "error": err.Error(), "group_count": 0, "rule_count": 0, "groups": []any{}})
		return
	}
	rawGroups, _ := payload.Data["groups"].([]any)
	groups := make([]gin.H, 0, len(rawGroups))
	ruleCount := 0
	for _, rawGroup := range rawGroups {
		group, _ := rawGroup.(map[string]any)
		rawRules, _ := group["rules"].([]any)
		rules := make([]gin.H, 0, len(rawRules))
		for _, rawRule := range rawRules {
			rule, _ := rawRule.(map[string]any)
			labels, _ := rule["labels"].(map[string]any)
			annotations, _ := rule["annotations"].(map[string]any)
			ruleType := stringValue(rule["type"])
			activeAlerts, _ := rule["alerts"].([]any)
			rules = append(rules, gin.H{"type": ruleType, "name": stringValue(rule["name"]), "query": stringValue(rule["query"]), "duration": rule["duration"], "keep_firing_for": rule["keepFiringFor"], "labels": labels, "annotations": annotations, "health": stringValue(rule["health"]), "state": stringValue(rule["state"]), "last_error": stringValue(rule["lastError"]), "evaluation_time": rule["evaluationTime"], "last_evaluation": stringValue(rule["lastEvaluation"]), "active_alerts_count": len(activeAlerts)})
			ruleCount++
		}
		groups = append(groups, gin.H{"name": stringValue(group["name"]), "file": stringValue(group["file"]), "interval": group["interval"], "rules": rules})
	}
	response.Success(context, gin.H{"status": "success", "prometheus_base_url": baseURL, "group_count": len(groups), "rule_count": ruleCount, "groups": groups, "warnings": payload.Warnings})
}

func (handler *Handler) PrometheusQuery(context *gin.Context) {
	handler.prometheusQuery(context, "/api/v1/query", []string{"query", "time", "timeout"}, []string{"query"})
}

func (handler *Handler) PrometheusQueryRange(context *gin.Context) {
	handler.prometheusQuery(context, "/api/v1/query_range", []string{"query", "start", "end", "step", "timeout"}, []string{"query", "start", "end", "step"})
}

func (handler *Handler) prometheusQuery(context *gin.Context, path string, allowed, required []string) {
	query := url.Values{}
	for _, name := range allowed {
		if value := strings.TrimSpace(context.Query(name)); value != "" {
			query.Set(name, value)
		}
	}
	for _, name := range required {
		if query.Get(name) == "" {
			response.BusinessError(context, 400, name+" parameter is required", nil)
			return
		}
	}
	_, payload, err := handler.prometheusGet(context, path, query)
	if err != nil {
		response.Success(context, gin.H{"status": "error", "error": err.Error(), "error_type": payload.Error, "result_type": "", "result": []any{}})
		return
	}
	response.Success(context, gin.H{"status": "success", "result_type": stringValue(payload.Data["resultType"]), "result": payload.Data["result"], "warnings": payload.Warnings})
}

func (handler *Handler) PrometheusProxy(context *gin.Context) {
	path := "/" + strings.TrimLeft(context.Param("apiPath"), "/")
	if !strings.HasPrefix(path, "/api/v1/") {
		context.JSON(http.StatusBadRequest, gin.H{"status": "error", "errorType": "bad_data", "error": "only /api/v1/* is allowed"})
		return
	}
	_, payload, err := handler.prometheusGet(context, path, context.Request.URL.Query())
	if err != nil {
		context.JSON(http.StatusOK, gin.H{"status": "error", "data": gin.H{}, "errorType": payload.Error, "error": err.Error(), "warnings": payload.Warnings})
		return
	}
	context.JSON(http.StatusOK, gin.H{"status": "success", "data": payload.Data, "errorType": "", "error": "", "warnings": payload.Warnings})
}

func (handler *Handler) MachineAuthenticate() gin.HandlerFunc {
	return func(context *gin.Context) {
		token := strings.TrimSpace(context.Query("token"))
		if token == "" {
			authorization := strings.TrimSpace(context.GetHeader("Authorization"))
			if len(authorization) >= len("Bearer ") && strings.EqualFold(authorization[:len("Bearer ")], "Bearer ") {
				token = strings.TrimSpace(authorization[len("Bearer "):])
			} else {
				token = authorization
			}
		}
		rows, err := handler.db.QueryContext(context, `SELECT id,token_hash FROM sys_agent_token WHERE is_active=TRUE AND (expires_at IS NULL OR expires_at>UTC_TIMESTAMP(6))`)
		if err == nil {
			defer rows.Close()
		}
		valid := false
		var matchedID int64
		for err == nil && rows.Next() {
			var id int64
			var encoded string
			if rows.Scan(&id, &encoded) == nil && identity.VerifyPassword(encoded, token) {
				valid = true
				matchedID = id
				break
			}
		}
		if err == nil {
			err = rows.Err()
		}
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "token validation failed"})
			return
		}
		if !valid {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		_, _ = handler.db.ExecContext(context, `UPDATE sys_agent_token SET last_used_at=? WHERE id=?`, time.Now().UTC(), matchedID)
		context.Next()
	}
}

func (handler *Handler) PrometheusHTTPServiceDiscovery(context *gin.Context) {
	rows, err := handler.db.QueryContext(context, `SELECT t.exporter_type,t.scrape_port,h.id,h.instance_name,h.ip FROM monitor_target t JOIN assets_host h ON h.id=t.host_id WHERE t.managed_enabled=TRUE AND t.install_status='success' AND h.ip IS NOT NULL ORDER BY t.id DESC`)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "query targets failed"})
		return
	}
	defer rows.Close()
	results := make([]gin.H, 0)
	for rows.Next() {
		var exporterType string
		var scrapePort, hostID int64
		var instanceName, ip sql.NullString
		if rows.Scan(&exporterType, &scrapePort, &hostID, &instanceName, &ip) != nil || !ip.Valid || strings.TrimSpace(ip.String) == "" {
			continue
		}
		job := strings.TrimSpace(exporterType)
		if job == "" {
			job = "exporter"
		}
		results = append(results, gin.H{"targets": []string{fmt.Sprintf("%s:%d", strings.TrimSpace(ip.String), scrapePort)}, "labels": gin.H{"job": job, "__meta_dj_exporter_type": exporterType, "__meta_dj_host_id": strconv.FormatInt(hostID, 10), "__meta_dj_instance_name": instanceName.String}})
	}
	if err = rows.Err(); err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "query targets failed"})
		return
	}
	context.JSON(http.StatusOK, results)
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprint(value)
}
func defaultString(value any, fallback string) string {
	result := stringValue(value)
	if result == "" {
		return fallback
	}
	return result
}

func (handler *Handler) PrometheusTSDBStatus(context *gin.Context) {
	handler.prometheusStatus(context, "/api/v1/status/tsdb", "query prometheus tsdb status failed", false)
}

func (handler *Handler) PrometheusConfig(context *gin.Context) {
	handler.prometheusStatus(context, "/api/v1/status/config", "query prometheus config failed", true)
}

func (handler *Handler) PrometheusFlags(context *gin.Context) {
	handler.prometheusStatus(context, "/api/v1/status/flags", "query prometheus flags failed", false)
}

func (handler *Handler) prometheusStatus(context *gin.Context, path, fallbackError string, includeYAML bool) {
	baseURL, payload, err := handler.prometheusGet(context, path, nil)
	data := gin.H{"status": "success", "prometheus_base_url": baseURL, "result": gin.H{}, "warnings": []any{}}
	if includeYAML {
		data["config_yaml"] = ""
	}
	if err != nil {
		data["status"] = "error"
		data["error"] = fallbackError + ": " + err.Error()
		response.Success(context, data)
		return
	}
	data["result"] = payload.Data
	data["warnings"] = payload.Warnings
	if includeYAML {
		data["config_yaml"] = fmt.Sprint(payload.Data["yaml"])
	}
	response.Success(context, data)
}

type prometheusPayload struct {
	Status   string         `json:"status"`
	Data     map[string]any `json:"data"`
	Warnings []any          `json:"warnings"`
	Error    string         `json:"error"`
}

func (handler *Handler) prometheusGet(requestContext context.Context, path string, query url.Values) (string, prometheusPayload, error) {
	baseURL := handler.prometheusBaseURL(requestContext)
	endpoint := baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
	if err != nil {
		return baseURL, prometheusPayload{}, err
	}
	request.Header.Set("Accept", "application/json")
	upstream, err := handler.client.Do(request)
	if err != nil {
		return baseURL, prometheusPayload{}, fmt.Errorf("prometheus request failed: %w", err)
	}
	defer upstream.Body.Close()
	var payload prometheusPayload
	if err = json.NewDecoder(upstream.Body).Decode(&payload); err != nil {
		return baseURL, prometheusPayload{}, fmt.Errorf("prometheus invalid json response: %w", err)
	}
	if upstream.StatusCode < 200 || upstream.StatusCode >= 300 || !strings.EqualFold(payload.Status, "success") {
		if payload.Error == "" {
			payload.Error = upstream.Status
		}
		return baseURL, payload, fmt.Errorf("%s", payload.Error)
	}
	if payload.Data == nil {
		payload.Data = map[string]any{}
	}
	return baseURL, payload, nil
}

func (handler *Handler) prometheusBaseURL(context context.Context) string {
	var value sql.NullString
	err := handler.db.QueryRowContext(context, "SELECT value FROM sys_config WHERE `key`=? ORDER BY id LIMIT 1", "monitor.prometheus.base_url").Scan(&value)
	if err == nil && value.Valid && strings.TrimSpace(value.String) != "" {
		return strings.TrimRight(strings.TrimSpace(value.String), "/")
	}
	return defaultPrometheusBaseURL
}
