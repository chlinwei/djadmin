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
	"autoadmin/internal/automation"
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

const defaultPrometheusBaseURL = "http://10.25.66.150:9999"

type Handler struct {
	db          *sql.DB
	client      *http.Client
	gateway     *agent.Gateway
	secrets     *assets.SecretEncryptor
	packageRoot string
	jobs        *automation.Handler
	// 目标安装缺少主机平台/架构信息时主动补采一次资产信息（由 assets 域注入，避免 monitor 反向依赖采集实现）。
	refreshHostInfo func(ctx context.Context, hostID int64) error
	// 告警通知的 SMTP 发信能力，缺省 sendSMTPMedia；单测可注入假实现。
	smtpSend smtpSender
}

// SetHostInfoRefresher 注入主机资产补采能力（assets.Handler.RefreshHostInfoByID）。
func (handler *Handler) SetHostInfoRefresher(refresher func(ctx context.Context, hostID int64) error) {
	handler.refreshHostInfo = refresher
}

func NewHandler(db *sql.DB, gateway *agent.Gateway, jobs *automation.Handler, encryptionKey, djangoSecret string) (*Handler, error) {
	secrets, err := assets.NewSecretEncryptor(encryptionKey, djangoSecret)
	if err != nil {
		return nil, err
	}
	// 软件包根目录：autoadmin 自身的 media 目录（monitor_packages/ 与 agent_packages/）。
	// backend/ 已废弃，包存储随之从 Django MEDIA_ROOT 迁出。
	packageRoot, err := filepath.Abs("media")
	if err != nil {
		return nil, err
	}
	handler := &Handler{db: db, client: &http.Client{Timeout: 8 * time.Second}, gateway: gateway, secrets: secrets, packageRoot: packageRoot, jobs: jobs}
	handler.smtpSend = handler.handlerSendSMTP
	// 给历史 Filebeat 软件包补默认安装/卸载 Playbook（没配的补齐到软件包配置里）。
	handler.backfillFilebeatPackagePlaybooks()
	// 失联对账 ticker：单实例部署，进程内唯一 goroutine，随进程退出终止（无需优雅停止）。
	go handler.reconcileStaleAlertsLoop()
	return handler, nil
}

// handlerSendSMTP 适配 media_send.go 的 sendSMTPMedia 到通知链路使用的 smtpSender 签名。
func (handler *Handler) handlerSendSMTP(config map[string]any, subject, body string, recipients []string) (bool, string) {
	return handler.sendSMTPMedia(config, subject, body, recipients)
}

func (handler *Handler) Summary(context *gin.Context) {
	summary, err := db.New(handler.db).CountMonitorTargetSummary(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	total, managedEnabled, installSuccess, scrapeUp := summary.Total, summary.ManagedEnabled, summary.InstallSuccess, summary.ScrapeUp
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
	activeTargets, _ := payload.dataMap()["activeTargets"].([]any)
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
	activeTargets, _ := payload.dataMap()["activeTargets"].([]any)
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
	alerts, _ := payload.dataMap()["alerts"].([]any)
	// `/api/v1/alerts` 不含规则表达式（PromQL 只在 `/api/v1/rules`），这里按 alertname
	// 关联规则索引，补上 rule_group / rule_details（query、labels、annotations）。
	// 否则前端"当前告警"的规则组与展开行的 PromQL 恒为空（历史告警读的是落库快照，所以正常）。
	ruleIndexes := handler.prometheusAlertRuleIndexes(context.Request.Context())
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
		ruleDetails, hasRuleDetails := ruleIndexes.byAlertname[strings.TrimSpace(stringValue(labels["alertname"]))]
		ruleGroup, ruleDetailsValue := "", any(nil)
		if hasRuleDetails {
			ruleGroup = ruleDetails.GroupName
			ruleDetailsValue = ruleDetails
		}
		results = append(results, gin.H{"name": stringValue(labels["alertname"]), "severity": stringValue(labels["severity"]), "state": defaultString(state, "unknown"), "instance": stringValue(labels["instance"]), "labels": labels, "summary": defaultString(annotations["summary"], stringValue(annotations["description"])), "active_at": stringValue(alert["activeAt"]), "value": stringValue(alert["value"]), "rule_group": ruleGroup, "rule_details": ruleDetailsValue, "history_id": nil, "notification_count": 0, "notification_delivery_count": 0, "notification_status": "none"})
	}
	response.Success(context, gin.H{"status": "success", "prometheus_base_url": baseURL, "count": len(results), "firing_count": firingCount, "resolved_count": resolvedCount, "results": results, "warnings": payload.Warnings})
}

func (handler *Handler) PrometheusRules(context *gin.Context) {
	baseURL, payload, err := handler.prometheusGet(context, "/api/v1/rules", nil)
	if err != nil {
		response.Success(context, gin.H{"status": "error", "prometheus_base_url": baseURL, "error": err.Error(), "group_count": 0, "rule_count": 0, "groups": []any{}})
		return
	}
	rawGroups, _ := payload.dataMap()["groups"].([]any)
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
	data := payload.dataMap()
	response.Success(context, gin.H{"status": "success", "result_type": stringValue(data["resultType"]), "result": data["result"], "warnings": payload.Warnings})
}

func (handler *Handler) PrometheusProxy(context *gin.Context) {
	path := "/" + strings.TrimLeft(context.Param("apiPath"), "/")
	if !strings.HasPrefix(path, "/api/v1/") {
		context.JSON(http.StatusBadRequest, gin.H{"status": "error", "errorType": "bad_data", "error": "only /api/v1/* is allowed"})
		return
	}
	query := context.Request.URL.Query()
	// codemirror-promql 会以 POST + form 提交 /labels、/series 等请求，统一转成 query 参数发给上游
	if context.Request.Method == http.MethodPost {
		if err := context.Request.ParseForm(); err == nil {
			for key, values := range context.Request.PostForm {
				for _, value := range values {
					query.Set(key, value)
				}
			}
		}
	}
	_, payload, err := handler.prometheusGet(context, path, query)
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
		// 过期判定用应用层传的时间（原实现是 SQL 里的 UTC_TIMESTAMP(6)，是方言函数）。
		tokens, err := db.New(handler.db).ListActiveAgentTokens(context, sql.NullTime{Time: time.Now().UTC(), Valid: true})
		if err != nil {
			context.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "token validation failed"})
			return
		}
		valid := false
		var matchedID int32
		for _, row := range tokens {
			if identity.VerifyPassword(row.TokenHash, token) {
				valid = true
				matchedID = row.ID
				break
			}
		}
		if !valid {
			context.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		_ = db.New(handler.db).MarkAgentTokenUsed(context, db.MarkAgentTokenUsedParams{
			LastUsedAt: sql.NullTime{Time: time.Now().UTC(), Valid: true}, ID: matchedID,
		})
		context.Next()
	}
}

func (handler *Handler) PrometheusHTTPServiceDiscovery(context *gin.Context) {
	rows, err := db.New(handler.db).ListPrometheusServiceDiscoveryTargets(context)
	if err != nil {
		context.JSON(http.StatusInternalServerError, gin.H{"error": "query targets failed"})
		return
	}
	results := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		if !row.Ip.Valid || strings.TrimSpace(row.Ip.String) == "" {
			continue
		}
		job := strings.TrimSpace(row.ExporterType)
		if job == "" {
			job = "exporter"
		}
		results = append(results, gin.H{"targets": []string{fmt.Sprintf("%s:%d", strings.TrimSpace(row.Ip.String), row.ScrapePort)}, "labels": gin.H{"job": job, "__meta_dj_exporter_type": row.ExporterType, "__meta_dj_host_id": strconv.FormatInt(row.ID, 10), "__meta_dj_instance_name": row.InstanceName.String}})
	}
	context.JSON(http.StatusOK, results)
}

// boolValue 把 PATCH 提交的 JSON 值转成 bool（兼容 true/1/"true" 三种写法）。
func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed != 0
	case string:
		return typed == "true" || typed == "1"
	default:
		return false
	}
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
	result := payload.dataMap()
	data["result"] = result
	data["warnings"] = payload.Warnings
	if includeYAML {
		data["config_yaml"] = fmt.Sprint(result["yaml"])
	}
	response.Success(context, data)
}

type prometheusPayload struct {
	Status   string          `json:"status"`
	Data     json.RawMessage `json:"data"`
	Warnings []any           `json:"warnings"`
	Error    string          `json:"error"`
}

// dataMap 将 data 解为对象；label values、series 等端点的 data 是数组，会得到 nil。
func (payload prometheusPayload) dataMap() map[string]any {
	data := map[string]any{}
	_ = json.Unmarshal(payload.Data, &data)
	return data
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
		payload.Data = json.RawMessage(`{}`)
	}
	return baseURL, payload, nil
}

func (handler *Handler) prometheusBaseURL(context context.Context) string {
	value, err := db.New(handler.db).GetConfigValueByKey(context, "monitor.prometheus.base_url")
	if err == nil && strings.TrimSpace(value) != "" {
		return strings.TrimRight(strings.TrimSpace(value), "/")
	}
	return defaultPrometheusBaseURL
}
