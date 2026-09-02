package monitor

import (
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
	handler.saveResource(context, retentionSpec, 0, handler.respondRetentionTier)
}
func (handler *Handler) UpdateRetentionTier(context *gin.Context) {
	handler.saveResource(context, retentionSpec, parseID(context.Param("id")), handler.respondRetentionTier)
}
func (handler *Handler) DeleteRetentionTier(context *gin.Context) {
	handler.deleteResource(context, retentionSpec)
}

func (handler *Handler) CreateProcessingRule(context *gin.Context) {
	handler.saveResource(context, processingSpec, 0, handler.respondProcessingRule)
}
func (handler *Handler) UpdateProcessingRule(context *gin.Context) {
	handler.saveResource(context, processingSpec, parseID(context.Param("id")), handler.respondProcessingRule)
}
func (handler *Handler) DeleteProcessingRule(context *gin.Context) {
	handler.deleteResource(context, processingSpec)
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

func (handler *Handler) saveResource(context *gin.Context, spec resourceSpec, id int64, respond func(*gin.Context, int64)) {
	var input map[string]any
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	if message := validateResource(spec, input, id); message != "" {
		response.BusinessError(context, 400, message, nil)
		return
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
