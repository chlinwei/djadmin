package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

type resourceSpec struct {
	table        string
	fields       map[string]bool
	jsonFields   map[string]bool
	filterFields map[string]string
	searchFields []string
	order        string
}

var retentionSpec = resourceSpec{
	table:        "monitor_log_retention_tier",
	fields:       fieldSet("code", "name", "daily_size_gb", "retention_days", "rollover_min_index_age", "enabled", "is_default", "remark"),
	filterFields: map[string]string{"enabled": "enabled", "is_default": "is_default"},
	searchFields: []string{"code", "name", "remark"}, order: "retention_days,id",
}

var processingSpec = resourceSpec{
	table:        "monitor_log_processing_rule",
	fields:       fieldSet("cluster", "application", "name", "description", "input_format", "multiline_enabled", "start_pattern", "continuation_pattern", "flush_timeout", "pipeline_body", "remark"),
	jsonFields:   fieldSet("pipeline_body"),
	filterFields: map[string]string{"cluster": "cluster_id", "application": "application_id", "input_format": "input_format", "multiline_enabled": "multiline_enabled"},
	searchFields: []string{"name", "description"}, order: "name,id",
}

var filterRuleSpec = resourceSpec{
	table:        "monitor_log_collection_filter_rule",
	fields:       fieldSet("application", "name", "description", "pattern", "enabled", "remark"),
	filterFields: map[string]string{"application": "application_id", "enabled": "enabled"},
	searchFields: []string{"name", "description", "pattern"}, order: "name,id",
}

func fieldSet(names ...string) map[string]bool {
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result[name] = true
	}
	return result
}

func (handler *Handler) CreateRetentionTier(context *gin.Context) {
	handler.saveResource(context, retentionSpec, 0, handler.respondRetentionTierThenSync)
}
func (handler *Handler) UpdateRetentionTier(context *gin.Context) {
	handler.saveResource(context, retentionSpec, parseID(context.Param("id")), handler.respondRetentionTierThenSync)
}
func (handler *Handler) DeleteRetentionTier(context *gin.Context) {
	id := parseID(context.Param("id"))
	var serviceCount, settingCount int
	if err := handler.db.QueryRowContext(context, `SELECT (SELECT COUNT(*) FROM assets_application_service WHERE log_retention_tier_id=?), (SELECT COUNT(*) FROM assets_application_service_log_setting WHERE retention_tier_id=?)`, id, id).Scan(&serviceCount, &settingCount); err != nil {
		response.Error(context, err)
		return
	}
	if serviceCount+settingCount > 0 {
		response.BusinessError(context, 400, "该档位仍被逻辑服务引用，不能删除", nil)
		return
	}
	handler.deleteResource(context, retentionSpec)
	// 档位删除后已启用集群的 ISM 策略需要重新对账。
	go handler.syncAllClusterLogStorage()
}

// respondRetentionTierThenSync 档位保存成功后异步刷新所有启用集群的模板与 ISM 策略。
// Django 走 celery 异步任务（_apply_policies），Go 用 goroutine 等价实现，不阻塞保存响应。
func (handler *Handler) respondRetentionTierThenSync(context *gin.Context, id int64) {
	handler.respondRetentionTier(context, id)
	go handler.syncAllClusterLogStorage()
}

func (handler *Handler) CreateProcessingRule(context *gin.Context) {
	handler.saveResource(context, processingSpec, 0, handler.respondProcessingRule, handler.publishProcessingRuleBeforeSave)
}
func (handler *Handler) UpdateProcessingRule(context *gin.Context) {
	handler.saveResource(context, processingSpec, parseID(context.Param("id")), handler.respondProcessingRule, handler.publishProcessingRuleBeforeSave)
}
func (handler *Handler) DeleteProcessingRule(context *gin.Context) {
	id := parseID(context.Param("id"))
	var name string
	var clusterID int64
	err := handler.db.QueryRowContext(context, `SELECT name,cluster_id FROM monitor_log_processing_rule WHERE id=?`, id).Scan(&name, &clusterID)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "resource not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	var referenceCount int
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM assets_application_log_definition WHERE processing_rule_id=?`, id).Scan(&referenceCount); err != nil {
		response.Error(context, err)
		return
	}
	if referenceCount > 0 {
		response.BusinessError(context, 400, "规则仍被日志定义引用，不能删除", nil)
		return
	}
	// 先删集群上的 pipeline 再删记录；集群侧失败则中止（与 Django destroy 行为一致）。
	if err := handler.deleteProcessingPipeline(context, clusterID, name); err != nil {
		response.BusinessError(context, 400, fmt.Sprintf("删除 Pipeline 失败: %v", err), nil)
		return
	}
	handler.deleteResource(context, processingSpec)
}

// publishProcessingRuleBeforeSave 对齐 Django LogProcessingRuleViewSet：先发布 pipeline
// 到 OpenSearch，发布失败则整条请求以 400 中止、不落库；更新时未提供的字段沿用旧值。
func (handler *Handler) publishProcessingRuleBeforeSave(context *gin.Context, input map[string]any, id int64) string {
	name := stringValue(input["name"])
	clusterValue, _ := input["cluster"].(float64)
	clusterID := int64(clusterValue)
	pipelineBody := input["pipeline_body"]
	if id != 0 {
		var existingName string
		var existingCluster int64
		var existingBody json.RawMessage
		err := handler.db.QueryRowContext(context, `SELECT name,cluster_id,pipeline_body FROM monitor_log_processing_rule WHERE id=?`, id).Scan(&existingName, &existingCluster, &existingBody)
		if err == nil {
			if name == "" {
				name = existingName
			}
			if clusterID == 0 {
				clusterID = existingCluster
			}
			if pipelineBody == nil {
				pipelineBody = existingBody
			}
		}
	}
	if name == "" || clusterID == 0 || pipelineBody == nil {
		return "name、cluster 与 pipeline_body 均为必填"
	}
	if err := handler.publishProcessingPipeline(context, clusterID, name, pipelineBody); err != nil {
		return fmt.Sprintf("发布 Pipeline 失败: %v", err)
	}
	return ""
}

func (handler *Handler) CreateFilterRule(context *gin.Context) {
	handler.saveResource(context, filterRuleSpec, 0, handler.respondFilterRule)
}
func (handler *Handler) UpdateFilterRule(context *gin.Context) {
	handler.saveResource(context, filterRuleSpec, parseID(context.Param("id")), handler.respondFilterRule)
}
func (handler *Handler) DeleteFilterRule(context *gin.Context) {
	handler.deleteResource(context, filterRuleSpec)
}

// beforeSave 在校验通过后、落库前执行（返回非空文案则以 400 中止），
// 供解析规则等需要“先发布到 OpenSearch 再写库”的资源挂接外部副作用。
func (handler *Handler) saveResource(context *gin.Context, spec resourceSpec, id int64, respond func(*gin.Context, int64), beforeSave ...func(*gin.Context, map[string]any, int64) string) {
	var input map[string]any
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	if message := validateResource(spec, input, id); message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	for _, hook := range beforeSave {
		if message := hook(context, input, id); message != "" {
			response.BusinessError(context, 400, message, nil)
			return
		}
	}
	keys := make([]string, 0)
	for key := range input {
		if spec.fields[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		response.BusinessError(context, 400, "no writable fields", nil)
		return
	}
	columns, values, arguments := make([]string, 0, len(keys)), make([]string, 0, len(keys)), make([]any, 0, len(keys)+2)
	for _, key := range keys {
		column := key
		if key == "cluster" || key == "application" {
			column = key + "_id"
		}
		value := input[key]
		if spec.jsonFields[key] {
			encoded, err := json.Marshal(value)
			if err != nil {
				response.BusinessError(context, 400, key+" must be valid JSON", nil)
				return
			}
			value = string(encoded)
		}
		columns = append(columns, column)
		values = append(values, "?")
		arguments = append(arguments, value)
	}
	now := time.Now().UTC()
	if id == 0 {
		columns = append(columns, "create_time", "update_time")
		values = append(values, "?", "?")
		arguments = append(arguments, now, now)
		result, err := handler.db.ExecContext(context, "INSERT INTO "+spec.table+" ("+strings.Join(columns, ",")+") VALUES ("+strings.Join(values, ",")+")", arguments...)
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		id, _ = result.LastInsertId()
	} else {
		sets := make([]string, len(columns))
		for index, column := range columns {
			sets[index] = column + "=?"
		}
		sets = append(sets, "update_time=?")
		arguments = append(arguments, now, id)
		result, err := handler.db.ExecContext(context, "UPDATE "+spec.table+" SET "+strings.Join(sets, ",")+" WHERE id=?", arguments...)
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			response.BusinessError(context, 404, "resource not found", nil)
			return
		}
	}
	if input["is_default"] == true {
		_, _ = handler.db.ExecContext(context, "UPDATE "+spec.table+" SET is_default=FALSE WHERE id<>?", id)
	}
	respond(context, id)
}

func validateResource(spec resourceSpec, input map[string]any, id int64) string {
	if spec.table == retentionSpec.table {
		if code, ok := input["code"].(string); ok && !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(strings.TrimSpace(code)) {
			return "code must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens"
		}
		if value, ok := input["daily_size_gb"].(float64); ok && value <= 0 {
			return "daily_size_gb must be greater than zero"
		}
		if value, ok := input["retention_days"].(float64); ok && (value < 1 || value > 3650) {
			return "retention_days must be between 1 and 3650"
		}
		if value, ok := input["rollover_min_index_age"].(string); ok && !regexp.MustCompile(`^\d+[mhd]$`).MatchString(value) {
			return "rollover_min_index_age must look like 30m, 12h, or 1d"
		}
	}
	if spec.table == processingSpec.table {
		if name, ok := input["name"].(string); ok && !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`).MatchString(name) {
			return "name contains unsupported characters"
		}
		if body, ok := input["pipeline_body"].(map[string]any); ok {
			if _, ok = body["processors"].([]any); !ok {
				return "pipeline_body.processors must be an array"
			}
		}
		if value, ok := input["flush_timeout"].(float64); ok && (value < 100 || value > 60000) {
			return "flush_timeout must be between 100 and 60000"
		}
		if enabled, _ := input["multiline_enabled"].(bool); enabled {
			if strings.TrimSpace(stringValue(input["start_pattern"])) == "" || strings.TrimSpace(stringValue(input["continuation_pattern"])) == "" {
				return "multiline patterns are required"
			}
		}
	}
	if spec.table == filterRuleSpec.table {
		if name, ok := input["name"].(string); ok && !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`).MatchString(name) {
			return "name contains unsupported characters"
		}
		if pattern, ok := input["pattern"].(string); ok {
			if strings.ContainsAny(pattern, "\r\n") {
				return "pattern cannot contain newlines"
			}
			if _, err := regexp.Compile(pattern); err != nil {
				return "invalid pattern: " + err.Error()
			}
		}
	}
	return ""
}

func (handler *Handler) deleteResource(context *gin.Context, spec resourceSpec) {
	id := parseID(context.Param("id"))
	result, err := handler.db.ExecContext(context, "DELETE FROM "+spec.table+" WHERE id=?", id)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		response.BusinessError(context, 404, "resource not found", nil)
		return
	}
	response.Success(context, gin.H{"deleted": true})
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int64:
		return float64(typed)
	default:
		var result float64
		fmt.Sscan(stringValue(value), &result)
		return result
	}
}
