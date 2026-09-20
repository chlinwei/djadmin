package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
)

type ApplicationService struct {
	ID                     int64           `json:"id"`
	CreateTime             string          `json:"create_time"`
	UpdateTime             string          `json:"update_time"`
	Remark                 *string         `json:"remark"`
	Name                   string          `json:"name"`
	Code                   string          `json:"code"`
	TopologyType           string          `json:"topology_type"`
	AccessAddress          string          `json:"access_address"`
	Enabled                bool            `json:"enabled"`
	Application            int64           `json:"application"`
	ApplicationName        string          `json:"application_name"`
	BusinessSystem         int64           `json:"business_system"`
	BusinessSystemName     string          `json:"business_system_name"`
	Environment            *int64          `json:"environment"`
	EnvironmentName        string          `json:"environment_name"`
	ApplicationVersion     int64           `json:"application_version"`
	ApplicationVersionName string          `json:"application_version_name"`
	DeploymentTemplate     int64           `json:"deployment_template"`
	DeploymentTemplateName string          `json:"deployment_template_name"`
	ClusterProfile         *int64          `json:"cluster_profile"`
	ClusterProfileName     string          `json:"cluster_profile_name"`
	MacroValues            json.RawMessage `json:"macro_values"`
	LogCollectionEnabled   bool            `json:"log_collection_enabled"`
	LogRetentionTier       *int64          `json:"log_retention_tier"`
	DeploymentCount        int64           `json:"deployment_count"`
	MemberInstances        []int64         `json:"member_instances"`
	// Ports 该服务应监听的端口 —— **定义在部署模板上**（`assets_application_port`），服务侧只继承
	// 不单独维护。服务树的「监听端口」一节读它；不带出来那段就永远是"未配置端口"（2026-09-20 现场）。
	Ports []TemplatePort `json:"ports,omitempty"`
}
type ApplicationDeployment struct {
	ID                    int64           `json:"id"`
	CreateTime            string          `json:"create_time"`
	UpdateTime            string          `json:"update_time"`
	Remark                *string         `json:"remark"`
	InstanceName          string          `json:"instance_name"`
	Enabled               bool            `json:"enabled"`
	Host                  int64           `json:"host"`
	HostIP                string          `json:"host_ip"`
	RuntimeStatus         string          `json:"runtime_status"`
	RuntimeStatusOutput   string          `json:"runtime_status_output"`
	LastStatusCheckTime   *string         `json:"last_status_check_time"`
	HaRole                string          `json:"ha_role"`
	RuntimeVariables      json.RawMessage `json:"runtime_variables"`
	ApplicationServiceIDs []int64         `json:"application_service_ids"`
	ApplicationID         *int64          `json:"application_id"` // 部署关联的首个服务所属应用，供前端按应用过滤实例
}

func nullableID(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
func nullableTime(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := timestamp(value.Time)
	return &formatted
}

func (r *Repository) ListApplicationServices(ctx context.Context, search string, page pagination.Page, businessSystemID int64) ([]ApplicationService, int64, error) {
	// 搜索为空传 NULL、业务系统为 0 传 NULL，走查询里"不过滤"的分支。
	filter := db.CountApplicationServicesParams{Pattern: pattern(search)}
	if businessSystemID > 0 {
		// 与 Django 版 DRF filter 的 business_system 字段对齐：
		// 服务树的业务系统/环境节点靠它收敛到当前业务下的逻辑服务。
		filter.BusinessSystemID = sql.NullInt64{Int64: businessSystemID, Valid: true}
	}
	count, err := r.queries.CountApplicationServices(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListApplicationServices(ctx, db.ListApplicationServicesParams{
		Pattern: filter.Pattern, BusinessSystemID: filter.BusinessSystemID,
		Limit: page.Size, Offset: page.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]ApplicationService, 0, len(rows))
	for _, row := range rows {
		items = append(items, ApplicationService{
			ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime),
			Remark: stringValue(row.Remark), Name: row.Name, Code: row.Code, TopologyType: row.TopologyType,
			AccessAddress: row.AccessAddress, Enabled: row.Enabled, Application: row.ApplicationID,
			ApplicationName: row.ApplicationName, BusinessSystem: row.BusinessSystemID,
			BusinessSystemName: row.BusinessSystemName, Environment: nullableID(row.EnvironmentID),
			EnvironmentName: row.EnvironmentName, ApplicationVersion: row.ApplicationVersionID,
			ApplicationVersionName: row.ApplicationVersionName, DeploymentTemplate: row.DeploymentTemplateID,
			DeploymentTemplateName: row.DeploymentTemplateName, ClusterProfile: nullableID(row.ClusterProfileID),
			ClusterProfileName: row.ClusterProfileName, MacroValues: row.MacroValues,
			LogCollectionEnabled: row.LogCollectionEnabled, LogRetentionTier: nullableID(row.LogRetentionTierID),
			DeploymentCount: row.DeploymentCount,
		})
	}
	return items, count, nil
}
func (r *Repository) GetApplicationService(ctx context.Context, id int64) (ApplicationService, error) {
	row, err := r.queries.GetApplicationServiceDetail(ctx, id)
	if err != nil {
		return ApplicationService{}, err
	}
	item := ApplicationService{
		ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime),
		Remark: stringValue(row.Remark), Name: row.Name, Code: row.Code, TopologyType: row.TopologyType,
		AccessAddress: row.AccessAddress, Enabled: row.Enabled, Application: row.ApplicationID,
		ApplicationName: row.ApplicationName, BusinessSystem: row.BusinessSystemID,
		BusinessSystemName: row.BusinessSystemName, Environment: nullableID(row.EnvironmentID),
		EnvironmentName: row.EnvironmentName, ApplicationVersion: row.ApplicationVersionID,
		ApplicationVersionName: row.ApplicationVersionName, DeploymentTemplate: row.DeploymentTemplateID,
		DeploymentTemplateName: row.DeploymentTemplateName, ClusterProfile: nullableID(row.ClusterProfileID),
		ClusterProfileName: row.ClusterProfileName, MacroValues: row.MacroValues,
		LogCollectionEnabled: row.LogCollectionEnabled, LogRetentionTier: nullableID(row.LogRetentionTierID),
		DeploymentCount: row.DeploymentCount,
	}
	deploymentIDs, err := r.queries.ListServiceDeploymentIDs(ctx, id)
	if err != nil {
		return item, err
	}
	item.MemberInstances = deploymentIDs
	return item, nil
}

// ApplicationDeploymentFilter 部署实例列表的过滤条件，对应 Django 版 DRF filter 字段：
// application_service（经 M2M 关联表）、application_service__business_system、application_service__environment。
// 服务树选中逻辑服务/业务系统/环境节点时依赖这些参数收敛右侧列表，缺失会导致返回全量实例。
type ApplicationDeploymentFilter struct {
	ApplicationServiceID int64
	BusinessSystemID     int64
	EnvironmentID        *int64
}

func (r *Repository) ListApplicationDeployments(ctx context.Context, page pagination.Page, filter ApplicationDeploymentFilter) ([]ApplicationDeployment, int64, error) {
	// 三个过滤条件都是可选的：0 / nil 传 NULL，走查询里"不过滤"的分支。
	params := db.CountApplicationDeploymentsParams{}
	if filter.ApplicationServiceID > 0 {
		params.ServiceID = sql.NullInt64{Int64: filter.ApplicationServiceID, Valid: true}
	}
	if filter.BusinessSystemID > 0 {
		params.BusinessSystemID = sql.NullInt64{Int64: filter.BusinessSystemID, Valid: true}
	}
	if filter.EnvironmentID != nil {
		params.EnvironmentID = sql.NullInt64{Int64: *filter.EnvironmentID, Valid: true}
	}
	count, err := r.queries.CountApplicationDeployments(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListApplicationDeployments(ctx, db.ListApplicationDeploymentsParams{
		ServiceID: params.ServiceID, BusinessSystemID: params.BusinessSystemID,
		EnvironmentID: params.EnvironmentID, Limit: page.Size, Offset: page.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	items := make([]ApplicationDeployment, 0, len(rows))
	for _, row := range rows {
		items = append(items, buildApplicationDeployment(applicationDeploymentFields{
			ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark,
			InstanceName: row.InstanceName, Enabled: row.Enabled, HostID: row.HostID, HostIP: row.HostIp,
			RuntimeStatus: row.RuntimeStatus, RuntimeStatusOutput: row.RuntimeStatusOutput,
			LastStatusCheckTime: row.LastStatusCheckTime, HaRole: row.HaRole,
			RuntimeVariables: row.RuntimeVariables, ApplicationID: row.ApplicationID,
		}))
	}
	if err = r.attachApplicationServiceIDs(ctx, items); err != nil {
		return nil, 0, err
	}
	return items, count, nil
}

// GetApplicationDeployment 按 id 取单个部署实例（附带服务关联 id）。
//
// 不要用列表查询 + 内存里找目标行的写法代替它：列表是 `ORDER BY id DESC LIMIT ?`，
// "取一页一行"会恒得到 id 最大的一台，保存后回读就会把刚保存的实例判成"不存在"。
func (r *Repository) GetApplicationDeployment(ctx context.Context, id int64) (ApplicationDeployment, error) {
	row, err := r.queries.GetApplicationDeploymentDetail(ctx, id)
	if err != nil {
		return ApplicationDeployment{}, err
	}
	items := []ApplicationDeployment{buildApplicationDeployment(applicationDeploymentFields{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark,
		InstanceName: row.InstanceName, Enabled: row.Enabled, HostID: row.HostID, HostIP: row.HostIp,
		RuntimeStatus: row.RuntimeStatus, RuntimeStatusOutput: row.RuntimeStatusOutput,
		LastStatusCheckTime: row.LastStatusCheckTime, HaRole: row.HaRole,
		RuntimeVariables: row.RuntimeVariables, ApplicationID: row.ApplicationID,
	})}
	if err = r.attachApplicationServiceIDs(ctx, items); err != nil {
		return ApplicationDeployment{}, err
	}
	return items[0], nil
}

// applicationDeploymentFields 是部署实例各查询结果里列名相同的字段集合。
// 单独抽出来是因为列表行与按 id 行在 sqlc 里是两个不同的类型，但对外必须是同一份结构
// （字段映射只写一次，避免两处漂移）。
type applicationDeploymentFields struct {
	ID                  int64
	CreateTime          time.Time
	UpdateTime          time.Time
	Remark              sql.NullString
	InstanceName        string
	Enabled             bool
	HostID              int64
	HostIP              string
	RuntimeStatus       string
	RuntimeStatusOutput string
	LastStatusCheckTime sql.NullTime
	HaRole              string
	RuntimeVariables    json.RawMessage
	ApplicationID       int64
}

func buildApplicationDeployment(fields applicationDeploymentFields) ApplicationDeployment {
	return ApplicationDeployment{
		ID: fields.ID, CreateTime: timestamp(fields.CreateTime), UpdateTime: timestamp(fields.UpdateTime),
		Remark: stringValue(fields.Remark), InstanceName: fields.InstanceName, Enabled: fields.Enabled,
		Host: fields.HostID, HostIP: fields.HostIP, RuntimeStatus: fields.RuntimeStatus,
		RuntimeStatusOutput: fields.RuntimeStatusOutput, LastStatusCheckTime: nullableTime(fields.LastStatusCheckTime),
		HaRole: fields.HaRole, RuntimeVariables: fields.RuntimeVariables,
		ApplicationID: nullableInt64Ptr(fields.ApplicationID), ApplicationServiceIDs: []int64{},
	}
}

// nullableInt64Ptr 把"子查询取回的 application_id"（0 表示没有关联服务）转成 nil。
func nullableInt64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

// attachApplicationServiceIDs 批量补齐部署与逻辑服务的 M2M 关联（assets_application_service_deployment），
// 服务树的"资源占比"按部署→服务→业务系统/项目归集资源，缺了这层关联饼图恒为空。
func (r *Repository) attachApplicationServiceIDs(ctx context.Context, items []ApplicationDeployment) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(items))
	for index := range items {
		ids = append(ids, items[index].ID)
	}
	rows, err := r.queries.ListServiceDeploymentLinks(ctx, ids)
	if err != nil {
		return err
	}
	linksByDeployment := make(map[int64][]int64)
	for _, row := range rows {
		linksByDeployment[row.DeploymentID] = append(linksByDeployment[row.DeploymentID], row.ServiceID)
	}
	for index := range items {
		if links, ok := linksByDeployment[items[index].ID]; ok {
			items[index].ApplicationServiceIDs = links
		}
	}
	return nil
}
