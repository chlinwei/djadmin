package monitor

import (
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
	DailySizeGB                 float64   `json:"daily_size_gb"`
	RetentionDays               int64     `json:"retention_days"`
	RolloverMinIndexAge         string    `json:"rollover_min_index_age"`
	Enabled                     bool      `json:"enabled"`
	IsDefault                   bool      `json:"is_default"`
	Remark                      string    `json:"remark"`
	EstimatedTotalGB            float64   `json:"estimated_total_gb"`
	RolloverMinPrimaryShardSize string    `json:"rollover_min_primary_shard_size"`
	ServiceCount                int64     `json:"service_count"`
}

func (handler *Handler) retentionTierResponse(context *gin.Context, row db.MonitorLogRetentionTier) retentionTierResponse {
	threshold := int64(row.DailySizeGb + 0.5)
	if threshold < 1 {
		threshold = 1
	}
	var serviceCount int64
	_ = handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM assets_application_service WHERE log_retention_tier_id=?`, row.ID).Scan(&serviceCount)
	return retentionTierResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		Code: row.Code, Name: row.Name, DailySizeGB: row.DailySizeGb,
		RetentionDays: int64(row.RetentionDays), RolloverMinIndexAge: row.RolloverMinIndexAge,
		Enabled: row.Enabled, IsDefault: row.IsDefault, Remark: row.Remark,
		EstimatedTotalGB:            row.DailySizeGb * float64(row.RetentionDays),
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

// ---- monitor_opensearch_cluster ----

type openSearchClusterResponse struct {
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

func openSearchClusterResponseFrom(row db.MonitorOpensearchCluster) openSearchClusterResponse {
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
	return openSearchClusterResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
		Name: row.Name, Hosts: row.Hosts, Username: row.Username, PasswordConfigured: row.Password != "",
		VerifyTLS: row.VerifyTls, CACert: row.CaCert, IndexPrefix: row.IndexPrefix,
		RequestTimeout: int64(row.RequestTimeout), Enabled: row.Enabled, IsDefault: row.IsDefault,
		LastCheckTime: lastCheckTime, LastCheckSuccess: lastCheckSuccess, LastCheckMessage: row.LastCheckMessage,
		Remark: row.Remark, StorageSyncError: row.StorageSyncError, StorageSyncStatus: row.StorageSyncStatus,
		StorageSyncTime: storageSyncTime,
	}
}

func (handler *Handler) ListOpenSearchClusters(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	enabled, isDefault, pattern := optionalBoolParam(context, "enabled"), optionalBoolParam(context, "is_default"), searchPatternParam(context)
	count, err := queries.CountOpenSearchClusters(context, db.CountOpenSearchClustersParams{Enabled: enabled, IsDefault: isDefault, Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListOpenSearchClustersTyped(context, db.ListOpenSearchClustersTypedParams{
		Enabled: enabled, IsDefault: isDefault, Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]openSearchClusterResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, openSearchClusterResponseFrom(row))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) respondOpenSearchCluster(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetOpenSearchClusterTyped(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "OpenSearch cluster not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, openSearchClusterResponseFrom(row))
}
func (handler *Handler) GetOpenSearchCluster(context *gin.Context) {
	handler.respondOpenSearchCluster(context, parseID(context.Param("id")))
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
	FlushTimeout        int64           `json:"flush_timeout"`
	PipelineBody        json.RawMessage `json:"pipeline_body"`
	Cluster             int64           `json:"cluster"`
	Application         *int64          `json:"application"`
	ApplicationName     string          `json:"application_name"`
	ApplicationCode     string          `json:"application_code"`
}

func (handler *Handler) processingRuleResponse(context *gin.Context, row db.MonitorLogProcessingRule) processingRuleResponse {
	var application *int64
	var applicationName, applicationCode string
	if row.ApplicationID.Valid {
		application = &row.ApplicationID.Int64
		var name, code sql.NullString
		_ = handler.db.QueryRowContext(context, `SELECT name,code FROM assets_application WHERE id=?`, row.ApplicationID.Int64).Scan(&name, &code)
		applicationName, applicationCode = name.String, code.String
	}
	return processingRuleResponse{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark.String,
		Name: row.Name, Description: row.Description, InputFormat: row.InputFormat,
		MultilineEnabled: row.MultilineEnabled, StartPattern: row.StartPattern, ContinuationPattern: row.ContinuationPattern,
		FlushTimeout: int64(row.FlushTimeout), PipelineBody: row.PipelineBody, Cluster: row.ClusterID,
		Application: application, ApplicationName: applicationName, ApplicationCode: applicationCode,
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
		var name sql.NullString
		_ = handler.db.QueryRowContext(context, `SELECT name FROM assets_application WHERE id=?`, row.ApplicationID.Int64).Scan(&name)
		applicationName = name.String
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

// ---- monitor_alert_media ----

type alertMediaResponse struct {
	ID         int64           `json:"id"`
	CreateTime time.Time       `json:"create_time"`
	UpdateTime time.Time       `json:"update_time"`
	Remark     string          `json:"remark"`
	Name       string          `json:"name"`
	MediaType  string          `json:"media_type"`
	Config     map[string]any  `json:"config"`
	Enabled    bool            `json:"enabled"`
	Recipients json.RawMessage `json:"recipients"`
}

func alertMediaResponseFrom(id int64, createTime, updateTime time.Time, remark sql.NullString, name, mediaType string, config, recipients json.RawMessage, enabled bool) alertMediaResponse {
	var decodedConfig map[string]any
	_ = json.Unmarshal(config, &decodedConfig)
	if decodedConfig == nil {
		decodedConfig = map[string]any{}
	}
	if stringValue(decodedConfig["password"]) != "" {
		decodedConfig["password"] = "********"
	}
	return alertMediaResponse{
		ID: id, CreateTime: createTime, UpdateTime: updateTime, Remark: remark.String,
		Name: name, MediaType: mediaType, Config: decodedConfig, Enabled: enabled, Recipients: recipients,
	}
}

func (handler *Handler) ListAlertMedia(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	mediaType, enabled, pattern := optionalStringParam(context, "media_type"), optionalBoolParam(context, "enabled"), searchPatternParam(context)
	count, err := queries.CountAlertMedia(context, db.CountAlertMediaParams{MediaType: mediaType, Enabled: enabled, Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListAlertMedia(context, db.ListAlertMediaParams{
		MediaType: mediaType, Enabled: enabled, Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]alertMediaResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, alertMediaResponseFrom(row.ID, row.CreateTime, row.UpdateTime, row.Remark, row.Name, row.MediaType, row.Config, row.Recipients, row.Enabled))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) getAlertMedia(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetAlertMediaTyped(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "alert media not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, alertMediaResponseFrom(row.ID, row.CreateTime, row.UpdateTime, row.Remark, row.Name, row.MediaType, row.Config, row.Recipients, row.Enabled))
}
func (handler *Handler) GetAlertMedia(context *gin.Context) {
	handler.getAlertMedia(context, parseID(context.Param("id")))
}

// ---- monitor_software_package ----

type softwarePackageResponse struct {
	ID                            int64     `json:"id"`
	CreateTime                    time.Time `json:"create_time"`
	UpdateTime                    time.Time `json:"update_time"`
	Remark                        string    `json:"remark"`
	Name                          string    `json:"name"`
	Version                       string    `json:"version"`
	OS                            string    `json:"os"`
	Arch                          string    `json:"arch"`
	File                          string    `json:"file"`
	SHA256                        string    `json:"sha256"`
	SizeBytes                     int64     `json:"size_bytes"`
	Enabled                       bool      `json:"enabled"`
	ServiceFileContent            string    `json:"service_file_content"`
	ServiceRunAsGroup             string    `json:"service_run_as_group"`
	ServiceRunAsUser              string    `json:"service_run_as_user"`
	InstallPlaybookTemplateID     *int64    `json:"install_playbook_template_id"`
	UninstallPlaybookTemplateID   *int64    `json:"uninstall_playbook_template_id"`
	WorkDirectory                 string    `json:"work_directory"`
	DefaultPort                   int64     `json:"default_port"`
	PackageFormat                 string    `json:"package_format"`
	PlatformFamily                string    `json:"platform_family"`
	PlatformMajor                 string    `json:"platform_major"`
	PackageType                   string    `json:"package_type"`
	InstallPlaybookTemplateName   string    `json:"install_playbook_template_name"`
	InstallPlaybookContent        string    `json:"install_playbook_content"`
	UninstallPlaybookTemplateName string    `json:"uninstall_playbook_template_name"`
	UninstallPlaybookContent      string    `json:"uninstall_playbook_content"`
	DownloadURL                   string    `json:"download_url"`
	FileName                      string    `json:"file_name"`
	Synced                        bool      `json:"synced"`
}

// softwarePackageResponseFrom 用位置参数而不是接生成的 Row struct，
// 因为 List/Get 两条 sqlc 查询列一致，但各自生成了不同的 Row 类型，写一份公用组装逻辑避免重复。
func softwarePackageResponseFrom(
	id int64, createTime, updateTime time.Time, remark sql.NullString, name, version, os, arch, file, sha256 string,
	sizeBytes int64, enabled bool, serviceFileContent, serviceRunAsGroup, serviceRunAsUser string,
	installTemplateID, uninstallTemplateID sql.NullInt64, workDirectory string, defaultPort uint32,
	packageFormat, platformFamily, platformMajor, packageType string,
	installTemplateName, installContent, uninstallTemplateName, uninstallContent string,
) softwarePackageResponse {
	var installID, uninstallID *int64
	if installTemplateID.Valid {
		installID = &installTemplateID.Int64
	}
	if uninstallTemplateID.Valid {
		uninstallID = &uninstallTemplateID.Int64
	}
	downloadURL, fileName := "", ""
	if file != "" {
		downloadURL = "/media/" + strings.TrimLeft(file, "/")
		parts := strings.Split(file, "/")
		fileName = parts[len(parts)-1]
	}
	return softwarePackageResponse{
		ID: id, CreateTime: createTime, UpdateTime: updateTime, Remark: remark.String,
		Name: name, Version: version, OS: os, Arch: arch, File: file, SHA256: sha256,
		SizeBytes: sizeBytes, Enabled: enabled, ServiceFileContent: serviceFileContent,
		ServiceRunAsGroup: serviceRunAsGroup, ServiceRunAsUser: serviceRunAsUser,
		InstallPlaybookTemplateID: installID, UninstallPlaybookTemplateID: uninstallID,
		WorkDirectory: workDirectory, DefaultPort: int64(defaultPort), PackageFormat: packageFormat,
		PlatformFamily: platformFamily, PlatformMajor: platformMajor, PackageType: packageType,
		InstallPlaybookTemplateName: installTemplateName, InstallPlaybookContent: installContent,
		UninstallPlaybookTemplateName: uninstallTemplateName, UninstallPlaybookContent: uninstallContent,
		DownloadURL: downloadURL, FileName: fileName, Synced: file != "" && sha256 != "",
	}
}

func (handler *Handler) ListPackages(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	packageType, name, version := optionalStringParam(context, "package_type"), optionalStringParam(context, "name"), optionalStringParam(context, "version")
	osParam, arch, enabled, pattern := optionalStringParam(context, "os"), optionalStringParam(context, "arch"), optionalBoolParam(context, "enabled"), searchPatternParam(context)
	count, err := queries.CountSoftwarePackages(context, db.CountSoftwarePackagesParams{
		PackageType: packageType, Name: name, Version: version, Os: osParam, Arch: arch, Enabled: enabled, Pattern: pattern,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListSoftwarePackages(context, db.ListSoftwarePackagesParams{
		PackageType: packageType, Name: name, Version: version, Os: osParam, Arch: arch, Enabled: enabled, Pattern: pattern,
		Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]softwarePackageResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, softwarePackageResponseFrom(
			row.ID, row.CreateTime, row.UpdateTime, row.Remark, row.Name, row.Version, row.Os, row.Arch, row.File, row.Sha256,
			row.SizeBytes, row.Enabled, row.ServiceFileContent, row.ServiceRunAsGroup, row.ServiceRunAsUser,
			row.InstallPlaybookTemplateID, row.UninstallPlaybookTemplateID, row.WorkDirectory, row.DefaultPort,
			row.PackageFormat, row.PlatformFamily, row.PlatformMajor, row.PackageType,
			row.InstallPlaybookTemplateName, row.InstallPlaybookContent, row.UninstallPlaybookTemplateName, row.UninstallPlaybookContent,
		))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) respondSoftwarePackage(context *gin.Context, id int64) {
	row, err := db.New(handler.db).GetSoftwarePackageTyped(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "software package not found", nil)
			return
		}
		response.Error(context, err)
		return
	}
	response.Success(context, softwarePackageResponseFrom(
		row.ID, row.CreateTime, row.UpdateTime, row.Remark, row.Name, row.Version, row.Os, row.Arch, row.File, row.Sha256,
		row.SizeBytes, row.Enabled, row.ServiceFileContent, row.ServiceRunAsGroup, row.ServiceRunAsUser,
		row.InstallPlaybookTemplateID, row.UninstallPlaybookTemplateID, row.WorkDirectory, row.DefaultPort,
		row.PackageFormat, row.PlatformFamily, row.PlatformMajor, row.PackageType,
		row.InstallPlaybookTemplateName, row.InstallPlaybookContent, row.UninstallPlaybookTemplateName, row.UninstallPlaybookContent,
	))
}
func (handler *Handler) GetSoftwarePackage(context *gin.Context) {
	handler.respondSoftwarePackage(context, parseID(context.Param("id")))
}
