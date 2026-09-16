package monitor

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

type resourceSpec struct {
	table        string
	fields       map[string]bool
	jsonFields   map[string]bool
	filterFields map[string]string
	searchFields []string
	order        string
	// required 是"落库必需"的列（NOT NULL 且库级没有默认值）。新建时缺了就 400——
	// 与改动前把语句交给 MySQL 严格模式报错的语义一致，只是文案更明确。
	required []string
	// 表名与列名过去是运行时拼进 SQL 的；sqlc 的语句编译期固定，所以写路径按表分派到
	// 下面这三个显式实现（PATCH 的"只写提交了的列"改由"读回整行 + 合并 + 整行写"承担）。
	create func(context.Context, db.DBTX, map[string]any) (int64, error)
	update func(context.Context, db.DBTX, int64, map[string]any) error
	delete func(context.Context, db.DBTX, int64) error
}

var retentionSpec = resourceSpec{
	table:        "monitor_log_retention_tier",
	fields:       fieldSet("code", "name", "daily_size_gb", "retention_days", "rollover_min_index_age", "enabled", "is_default", "remark"),
	filterFields: map[string]string{"enabled": "enabled", "is_default": "is_default"},
	searchFields: []string{"code", "name", "remark"}, order: "retention_days,id",
	required: []string{"code", "name", "daily_size_gb", "retention_days", "rollover_min_index_age", "enabled", "is_default", "remark"},
	create:   createLogRetentionTier, update: updateLogRetentionTier, delete: deleteLogRetentionTier,
}

var processingSpec = resourceSpec{
	table:        "monitor_log_processing_rule",
	fields:       fieldSet("cluster", "application", "name", "description", "input_format", "multiline_enabled", "start_pattern", "continuation_pattern", "flush_timeout", "pipeline_body", "remark"),
	jsonFields:   fieldSet("pipeline_body"),
	filterFields: map[string]string{"cluster": "cluster_id", "application": "application_id", "input_format": "input_format", "multiline_enabled": "multiline_enabled"},
	searchFields: []string{"name", "description"}, order: "name,id",
	required: []string{"cluster", "name", "description", "input_format", "multiline_enabled", "start_pattern", "continuation_pattern", "flush_timeout", "pipeline_body"},
	create:   createLogProcessingRule, update: updateLogProcessingRule, delete: deleteLogProcessingRule,
}

var filterRuleSpec = resourceSpec{
	table:        "monitor_log_collection_filter_rule",
	fields:       fieldSet("application", "name", "description", "pattern", "enabled", "remark"),
	filterFields: map[string]string{"application": "application_id", "enabled": "enabled"},
	searchFields: []string{"name", "description", "pattern"}, order: "name,id",
	required: []string{"name", "description", "pattern", "enabled"},
	create:   createLogCollectionFilterRule, update: updateLogCollectionFilterRule, delete: deleteLogCollectionFilterRule,
}

// resourceColumn 把请求体里的键映射到列名（cluster/application 是 *_id）。
func resourceColumn(key string) string {
	if key == "cluster" || key == "application" {
		return key + "_id"
	}
	return key
}

// requiredResourceColumns 返回缺失的必填键（保持 required 的顺序，便于稳定报错文案）。
func requiredResourceColumns(spec resourceSpec, input map[string]any) []string {
	missing := make([]string, 0, len(spec.required))
	for _, key := range spec.required {
		if _, ok := input[key]; !ok {
			missing = append(missing, key)
		}
	}
	return missing
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

// deleteRetentionTierByID 复用原单删逻辑：仍被逻辑服务/日志设置引用的档位拒绝删除。
func (handler *Handler) deleteRetentionTierByID(context *gin.Context, id int64) error {
	queries := db.New(handler.db)
	serviceCount, err := queries.CountRetentionTierServices(context, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return err
	}
	settingCount, err := queries.CountRetentionTierLogSettings(context, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return err
	}
	if serviceCount+settingCount > 0 {
		return errors.New("该档位仍被逻辑服务引用，不能删除")
	}
	return retentionSpec.delete(context, handler.db, id)
}

func (handler *Handler) BatchDeleteRetentionTiers(context *gin.Context) {
	ids, ok := logTargetIDs(context)
	if !ok {
		return
	}
	results := make([]gin.H, 0, len(ids))
	okCount := 0
	for _, id := range ids {
		if err := handler.deleteRetentionTierByID(context, id); err != nil {
			results = append(results, gin.H{"id": id, "ok": false, "message": batchErrMessage(err)})
			continue
		}
		okCount++
		results = append(results, gin.H{"id": id, "ok": true, "message": ""})
	}
	if okCount > 0 {
		// 档位删除后已启用集群的 ISM 策略需要重新对账。
		go handler.syncAllClusterLogStorage()
	}
	response.Success(context, gin.H{"count": okCount, "results": results})
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

// deleteProcessingRuleByID 复用原单删逻辑：被日志定义引用的规则拒绝删除；先删集群上的
// pipeline 再删记录，集群侧失败则中止（与 Django destroy 行为一致）。
func (handler *Handler) deleteProcessingRuleByID(context *gin.Context, id int64) error {
	queries := db.New(handler.db)
	rule, err := queries.GetLogProcessingRule(context, id)
	if err != nil {
		return err
	}
	referenceCount, err := queries.CountLogDefinitionReferences(context, sql.NullInt64{Int64: id, Valid: true})
	if err != nil {
		return err
	}
	if referenceCount > 0 {
		return errors.New("规则仍被日志定义引用，不能删除")
	}
	if err := handler.deleteProcessingPipeline(context, rule.ClusterID, rule.Name); err != nil {
		return fmt.Errorf("删除 Pipeline 失败: %w", err)
	}
	return processingSpec.delete(context, handler.db, id)
}

func (handler *Handler) BatchDeleteProcessingRules(context *gin.Context) {
	handler.batchDeleteResources(context, processingSpec, handler.deleteProcessingRuleByID)
}

// publishProcessingRuleBeforeSave 对齐 Django LogProcessingRuleViewSet：先发布 pipeline
// 到 OpenSearch，发布失败则整条请求以 400 中止、不落库；更新时未提供的字段沿用旧值。
func (handler *Handler) publishProcessingRuleBeforeSave(context *gin.Context, input map[string]any, id int64) string {
	name := stringValue(input["name"])
	clusterValue, _ := input["cluster"].(float64)
	clusterID := int64(clusterValue)
	pipelineBody := input["pipeline_body"]
	if id != 0 {
		// 复用整行读：更新时未提交的字段沿用旧值。
		if existing, err := db.New(handler.db).GetLogProcessingRule(context, id); err == nil {
			if name == "" {
				name = existing.Name
			}
			if clusterID == 0 {
				clusterID = existing.ClusterID
			}
			if pipelineBody == nil {
				pipelineBody = existing.PipelineBody
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
func (handler *Handler) BatchDeleteFilterRules(context *gin.Context) {
	handler.batchDeleteResources(context, filterRuleSpec, func(context *gin.Context, id int64) error {
		return filterRuleSpec.delete(context, handler.db, id)
	})
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
	if len(writableResourceKeys(spec, input)) == 0 {
		response.BusinessError(context, 400, "no writable fields", nil)
		return
	}
	if id == 0 {
		if missing := requiredResourceColumns(spec, input); len(missing) > 0 {
			response.BusinessError(context, 400, "缺少必填字段: "+strings.Join(missing, ", "), nil)
			return
		}
		newID, err := spec.create(context, handler.db, input)
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		id = newID
	} else {
		if err := spec.update(context, handler.db, id, input); err != nil {
			if err == sql.ErrNoRows {
				response.BusinessError(context, 404, "resource not found", nil)
				return
			}
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
	}
	if input["is_default"] == true {
		if err := clearDefaultLogRetentionTier(context, handler.db, id); err != nil {
			response.Error(context, err)
			return
		}
	}
	respond(context, id)
}

// writableResourceKeys 返回请求体里可写的键（与建表列一致的子集），供"没有可写字段"判定。
func writableResourceKeys(spec resourceSpec, input map[string]any) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		if spec.fields[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
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

// batchErrMessage 把 sql.ErrNoRows 统一转成 "resource not found"，其余透传原始错误文案。
func batchErrMessage(err error) string {
	if err == sql.ErrNoRows {
		return "resource not found"
	}
	return err.Error()
}

// batchDeleteResources 通用批删入口：解析 {"ids":[...]}，逐 id 执行 deleteOne 并汇总
// count/results；单条失败不中断其余 id（与 BatchDeleteLogTargets 范式一致）。
func (handler *Handler) batchDeleteResources(context *gin.Context, spec resourceSpec, deleteOne func(*gin.Context, int64) error) {
	ids, ok := logTargetIDs(context)
	if !ok {
		return
	}
	results := make([]gin.H, 0, len(ids))
	okCount := 0
	for _, id := range ids {
		if err := deleteOne(context, id); err != nil {
			results = append(results, gin.H{"id": id, "ok": false, "message": batchErrMessage(err)})
			continue
		}
		okCount++
		results = append(results, gin.H{"id": id, "ok": true, "message": ""})
	}
	response.Success(context, gin.H{"count": okCount, "results": results})
}

// deleteRowsAffected 把"没删到行"统一成 sql.ErrNoRows，供 batchDeleteMonitorRows 的各调用方复用
// （sqlc 生成的 DELETE 都是 :execresult，两侧都返回 sql.Result）。
func deleteRowsAffected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// batchDeleteMonitorRows 无前置校验的简单表通用批删（alert media、OpenSearch 集群）。
// 表名不再作为字符串传入：sqlc 的语句是编译期固定的，按表分派由调用方给出的 deleteOne 承担。
func batchDeleteMonitorRows(context *gin.Context, handler *Handler, deleteOne func(*gin.Context, int64) error) {
	ids, ok := logTargetIDs(context)
	if !ok {
		return
	}
	results := make([]gin.H, 0, len(ids))
	okCount := 0
	for _, id := range ids {
		err := deleteOne(context, id)
		if err != nil {
			results = append(results, gin.H{"id": id, "ok": false, "message": batchErrMessage(err)})
			continue
		}
		okCount++
		results = append(results, gin.H{"id": id, "ok": true, "message": ""})
	}
	response.Success(context, gin.H{"count": okCount, "results": results})
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
