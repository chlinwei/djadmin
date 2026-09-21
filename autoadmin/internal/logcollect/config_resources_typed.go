package logcollect

import (
	"math"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 这个文件是 config_resources.go 通用 map[string]any 引擎的类型化替代——只替换"读"这一侧
// （List/Get），因为 sqlc 生成的是编译期固定的 SQL，没法处理"写"那侧运行时可变的动态字段列表。
// "写"（saveResource/deleteResource）继续走原来的动态 CRUD，JSON 请求体解码出来的 bool 本来就是
// 正确类型，问题只出在"读"用 Scan 进 interface{} 上。

func optionalBoolParam(context *gin.Context, name string) sql.NullBool {
	value := strings.TrimSpace(context.Query(name))
	parsed, err := strconv.ParseBool(value)
	if value == "" || err != nil {
		return sql.NullBool{}
	}
	return sql.NullBool{Bool: parsed, Valid: true}
}

func optionalInt64Param(context *gin.Context, name string) sql.NullInt64 {
	value := strings.TrimSpace(context.Query(name))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if value == "" || err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: parsed, Valid: true}
}

func optionalStringParam(context *gin.Context, name string) sql.NullString {
	value := strings.TrimSpace(context.Query(name))
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

func searchPatternParam(context *gin.Context) sql.NullString {
	search := strings.TrimSpace(context.Query("search"))
	if search == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: "%" + search + "%", Valid: true}
}

// ---- monitor_log_retention_tier ----

type retentionTierResponse struct {
	ID                          int64     `json:"id"`
	CreateTime                  time.Time `json:"create_time"`
	UpdateTime                  time.Time `json:"update_time"`
	Code                        string    `json:"code"`
	Name                        string    `json:"name"`
	DailySizeGB float64 `json:"daily_size_gb"`
	// RetentionValue + RetentionUnit 是完整保留期（unit ∈ {d, h}）。单位与值分开存：
	// 同一列存 "12h" 这样的字符串就没法按大小排序，也没法算容量。
	RetentionValue      int64  `json:"retention_value"`
	RetentionUnit       string `json:"retention_unit"`
	RolloverMinIndexAge string `json:"rollover_min_index_age"`
	Enabled                     bool      `json:"enabled"`
	IsDefault                   bool      `json:"is_default"`
	Remark                      string    `json:"remark"`
	EstimatedTotalGB            float64   `json:"estimated_total_gb"`
	RolloverMinPrimaryShardSize string    `json:"rollover_min_primary_shard_size"`
	ServiceCount                int64     `json:"service_count"`
}

// estimatedTierTotalGB 按"每天写入量 × 保留期"反推该档位的稳态占用（容量规划用，见 §4.6）。
// 保留期以小时为单位时要折算成天（12h = 0.5 天），否则小时档位的预估容量会虚高 24 倍。
func estimatedTierTotalGB(dailySizeGB float64, value int64, unit string) float64 {
	days := float64(value)
	if strings.ToLower(strings.TrimSpace(unit)) == "h" {
		days = float64(value) / 24
	}
	return math.Round(dailySizeGB*days*100) / 100
}

func (handler *Handler) retentionTierResponse(context *gin.Context, row db.MonitorLogRetentionTier) retentionTierResponse {
	threshold := int64(row.DailySizeGb + 0.5)
	if threshold < 1 {
		threshold = 1
	}
	// 引用计数用于前端提示"仍被逻辑服务引用，不能删除"。
	serviceCount, _ := db.New(handler.db).CountRetentionTierServices(context, sql.NullInt64{Int64: row.ID, Valid: true})
	return retentionTierResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		Code: row.Code, Name: row.Name, DailySizeGB: row.DailySizeGb,
		RetentionValue: int64(row.RetentionValue), RetentionUnit: row.RetentionUnit,
		RolloverMinIndexAge: row.RolloverMinIndexAge,
		Enabled: row.Enabled, IsDefault: row.IsDefault, Remark: row.Remark,
		EstimatedTotalGB:            estimatedTierTotalGB(row.DailySizeGb, int64(row.RetentionValue), row.RetentionUnit),
		RolloverMinPrimaryShardSize: fmt.Sprintf("%dgb", threshold),
		ServiceCount:                serviceCount,
	}
}

func (handler *Handler) ListRetentionTiers(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	enabled, isDefault, pattern := optionalBoolParam(context, "enabled"), optionalBoolParam(context, "is_default"), searchPatternParam(context)
	count, err := queries.CountLogRetentionTiers(context, db.CountLogRetentionTiersParams{Enabled: enabled, IsDefault: isDefault, Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListLogRetentionTiers(context, db.ListLogRetentionTiersParams{
		Enabled: enabled, IsDefault: isDefault, Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]retentionTierResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, handler.retentionTierResponse(context, row))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) respondRetentionTier(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetLogRetentionTier(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "resource not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, handler.retentionTierResponse(context, row))
}
func (handler *Handler) GetRetentionTier(context *gin.Context) {
	handler.respondRetentionTier(context, parseID(context.Param("id")))
}

// ---- monitor_elasticsearch_cluster ----

type elasticsearchClusterResponse struct {
	ID                 int64      `json:"id"`
	CreateTime         time.Time  `json:"create_time"`
	UpdateTime         time.Time  `json:"update_time"`
	Name               string     `json:"name"`
	Hosts              string     `json:"hosts"`
	Username           string     `json:"username"`
	PasswordConfigured bool       `json:"password_configured"`
	VerifyTLS          bool       `json:"verify_tls"`
	CACert             string     `json:"ca_cert"`
	IndexPrefix        string     `json:"index_prefix"`
	RequestTimeout     int64      `json:"request_timeout"`
	Enabled            bool       `json:"enabled"`
	IsDefault          bool       `json:"is_default"`
	LastCheckTime      *time.Time `json:"last_check_time"`
	LastCheckSuccess   *bool      `json:"last_check_success"`
	LastCheckMessage   string     `json:"last_check_message"`
	Remark             string     `json:"remark"`
	StorageSyncError   string     `json:"storage_sync_error"`
	StorageSyncStatus  string     `json:"storage_sync_status"`
	StorageSyncTime    *time.Time `json:"storage_sync_time"`
}

func elasticsearchClusterResponseFrom(row db.MonitorElasticsearchCluster) elasticsearchClusterResponse {
	var lastCheckTime, storageSyncTime *time.Time
	if row.LastCheckTime.Valid {
		lastCheckTime = &row.LastCheckTime.Time
	}
	if row.StorageSyncTime.Valid {
		storageSyncTime = &row.StorageSyncTime.Time
	}
	var lastCheckSuccess *bool
	if row.LastCheckSuccess.Valid {
		lastCheckSuccess = &row.LastCheckSuccess.Bool
	}
	return elasticsearchClusterResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		Name: row.Name, Hosts: row.Hosts, Username: row.Username, PasswordConfigured: row.Password != "",
		VerifyTLS: row.VerifyTls, CACert: row.CaCert, IndexPrefix: row.IndexPrefix,
		RequestTimeout: int64(row.RequestTimeout), Enabled: row.Enabled, IsDefault: row.IsDefault,
		LastCheckTime: lastCheckTime, LastCheckSuccess: lastCheckSuccess, LastCheckMessage: row.LastCheckMessage,
		Remark: row.Remark, StorageSyncError: row.StorageSyncError, StorageSyncStatus: row.StorageSyncStatus,
		StorageSyncTime: storageSyncTime,
	}
}

func (handler *Handler) ListElasticsearchClusters(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	enabled, isDefault, pattern := optionalBoolParam(context, "enabled"), optionalBoolParam(context, "is_default"), searchPatternParam(context)
	count, err := queries.CountElasticsearchClusters(context, db.CountElasticsearchClustersParams{Enabled: enabled, IsDefault: isDefault, Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListElasticsearchClustersTyped(context, db.ListElasticsearchClustersTypedParams{
		Enabled: enabled, IsDefault: isDefault, Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]elasticsearchClusterResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, elasticsearchClusterResponseFrom(row))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) respondElasticsearchCluster(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetElasticsearchClusterTyped(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "Elasticsearch cluster not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, elasticsearchClusterResponseFrom(row))
}
func (handler *Handler) GetElasticsearchCluster(context *gin.Context) {
	handler.respondElasticsearchCluster(context, parseID(context.Param("id")))
}

// ---- monitor_log_processing_rule ----

type processingRuleResponse struct {
	ID                  int64           `json:"id"`
	CreateTime          time.Time       `json:"create_time"`
	UpdateTime          time.Time       `json:"update_time"`
	Remark              string          `json:"remark"`
	Name                string          `json:"name"`
	Description         string          `json:"description"`
	InputFormat         string          `json:"input_format"`
	MultilineEnabled    bool            `json:"multiline_enabled"`
	StartPattern        string          `json:"start_pattern"`
	ContinuationPattern string          `json:"continuation_pattern"`
	SampleLog           string          `json:"sample_log"`
	FlushTimeout        int64           `json:"flush_timeout"`
	PipelineBody        json.RawMessage `json:"pipeline_body"`
	Cluster             int64           `json:"cluster"`
	Application         *int64          `json:"application"`
	ApplicationName     string          `json:"application_name"`
	ApplicationCode     string          `json:"application_code"`
	// 巡检用：这条规则静态看着缺哪些必备字段（log_level / log_message / error_fingerprint）。
	// 非空意味着它采上来的日志会被 <prefix>-mapping-guard 判违规（drop 模式下直接丢弃），
	// 所以它是切换 LOG_MAPPING_GUARD_MODE=drop 之前必须先看的清单。
	MissingRequiredFields []string `json:"missing_required_fields,omitempty"`
}

func (handler *Handler) processingRuleResponse(context *gin.Context, row db.MonitorLogProcessingRule) processingRuleResponse {
	var application *int64
	var applicationName, applicationCode string
	if row.ApplicationID.Valid {
		application = &row.ApplicationID.Int64
		if applicationRow, err := db.New(handler.db).GetApplicationNameCode(context, row.ApplicationID.Int64); err == nil {
			applicationName, applicationCode = applicationRow.Name, applicationRow.Code
		}
	}
	return processingRuleResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark.String,
		Name: row.Name, Description: row.Description, InputFormat: row.InputFormat,
		MultilineEnabled: row.MultilineEnabled, StartPattern: row.StartPattern, ContinuationPattern: row.ContinuationPattern,
		SampleLog: row.SampleLog, FlushTimeout: int64(row.FlushTimeout), PipelineBody: row.PipelineBody, Cluster: row.ClusterID,
		Application: application, ApplicationName: applicationName, ApplicationCode: applicationCode,
		MissingRequiredFields: missingPipelineOutputs(row.PipelineBody),
	}
}

func (handler *Handler) ListProcessingRules(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	clusterID, applicationID := optionalInt64Param(context, "cluster"), optionalInt64Param(context, "application")
	inputFormat, multilineEnabled, pattern := optionalStringParam(context, "input_format"), optionalBoolParam(context, "multiline_enabled"), searchPatternParam(context)
	count, err := queries.CountLogProcessingRules(context, db.CountLogProcessingRulesParams{
		ClusterID: clusterID, ApplicationID: applicationID, InputFormat: inputFormat, MultilineEnabled: multilineEnabled, Pattern: pattern,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListLogProcessingRules(context, db.ListLogProcessingRulesParams{
		ClusterID: clusterID, ApplicationID: applicationID, InputFormat: inputFormat, MultilineEnabled: multilineEnabled, Pattern: pattern,
		Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]processingRuleResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, handler.processingRuleResponse(context, row))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) respondProcessingRule(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetLogProcessingRule(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "resource not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, handler.processingRuleResponse(context, row))
}
func (handler *Handler) GetProcessingRule(context *gin.Context) {
	handler.respondProcessingRule(context, parseID(context.Param("id")))
}

// ListProcessingRuleUsages 解析规则的引用关系（只读，一次全量）：
// GET /monitor/log-processing-rules/usage/
//
// 关联链：规则 ← 模板日志定义（assets_application_log_definition.processing_rule_id，
// 迁移 000035 后它是唯一来源）← 部署模板 ← 逻辑服务（service_count 量化爆炸半径）。
// 行数随引用数线性（规则量级小），一次读回由前端按左侧应用筛选，不值得再做分页参数。
// 未被引用的规则不在结果里——前端在「解析规则」表里能看到它们，这里只回答"被谁引用"。
type processingRuleUsageResponse struct {
	RuleID          int64  `json:"rule_id"`
	RuleName        string `json:"rule_name"`
	Application     *int64 `json:"application"`
	LogDefinitionID int64  `json:"log_definition_id"`
	LogName         string `json:"log_name"`
	PathPattern     string `json:"path_pattern"`
	TemplateID      int64  `json:"template_id"`
	TemplateName    string `json:"template_name"`
	ServiceCount    int64  `json:"service_count"`
}

func (handler *Handler) ListProcessingRuleUsages(context *gin.Context) {
	rows, err := db.New(handler.db).ListProcessingRuleUsages(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]processingRuleUsageResponse, 0, len(rows))
	for _, row := range rows {
		item := processingRuleUsageResponse{
			RuleID: row.RuleID, RuleName: row.RuleName,
			LogDefinitionID: row.LogDefinitionID, LogName: row.LogName, PathPattern: row.PathPattern,
			TemplateID: row.TemplateID, TemplateName: row.TemplateName, ServiceCount: row.ServiceCount,
		}
		if row.ApplicationID.Valid {
			application := row.ApplicationID.Int64
			item.Application = &application
		}
		items = append(items, item)
	}
	response.Success(context, gin.H{"count": len(items), "results": items})
}

// ---- monitor_log_collection_filter_rule ----

type filterRuleResponse struct {
	ID              int64     `json:"id"`
	CreateTime      time.Time `json:"create_time"`
	UpdateTime      time.Time `json:"update_time"`
	Remark          string    `json:"remark"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	Pattern         string    `json:"pattern"`
	Enabled         bool      `json:"enabled"`
	Application     *int64    `json:"application"`
	ApplicationName string    `json:"application_name"`
}

func (handler *Handler) filterRuleResponse(context *gin.Context, row db.MonitorLogCollectionFilterRule) filterRuleResponse {
	var application *int64
	var applicationName string
	if row.ApplicationID.Valid {
		application = &row.ApplicationID.Int64
		if applicationRow, err := db.New(handler.db).GetApplicationNameCode(context, row.ApplicationID.Int64); err == nil {
			applicationName = applicationRow.Name
		}
	}
	return filterRuleResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark.String,
		Name: row.Name, Description: row.Description, Pattern: row.Pattern, Enabled: row.Enabled,
		Application: application, ApplicationName: applicationName,
	}
}

func (handler *Handler) ListFilterRules(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	applicationID, enabled, pattern := optionalInt64Param(context, "application"), optionalBoolParam(context, "enabled"), searchPatternParam(context)
	count, err := queries.CountLogCollectionFilterRules(context, db.CountLogCollectionFilterRulesParams{
		ApplicationID: applicationID, Enabled: enabled, SearchPattern: pattern,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListLogCollectionFilterRules(context, db.ListLogCollectionFilterRulesParams{
		ApplicationID: applicationID, Enabled: enabled, SearchPattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]filterRuleResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, handler.filterRuleResponse(context, row))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) respondFilterRule(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetLogCollectionFilterRule(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "resource not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, handler.filterRuleResponse(context, row))
}
func (handler *Handler) GetFilterRule(context *gin.Context) {
	handler.respondFilterRule(context, parseID(context.Param("id")))
}
