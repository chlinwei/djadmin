package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

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
	DeploymentCount        int64           `json:"deployment_count"`
	MemberInstances        []int64         `json:"member_instances"`
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

func (r *Repository) ListApplicationServices(ctx context.Context, search string, page pagination.Page) ([]ApplicationService, int64, error) {
	pattern := "%" + search + "%"
	var count int64
	if err := r.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets_application_service s WHERE (?='' OR s.name LIKE ? OR s.code LIKE ?)`, search, pattern, pattern).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.QueryContext(ctx, `SELECT s.id,s.create_time,s.update_time,s.remark,s.name,s.code,s.topology_type,s.access_address,s.enabled,s.application_id,a.name,s.business_system_id,b.name,s.environment_id,COALESCE(e.name,''),s.application_version_id,v.version,s.deployment_template_id,t.name,s.cluster_profile_id,COALESCE(c.name,''),s.macro_values,s.log_collection_enabled,(SELECT COUNT(*) FROM assets_application_service_deployment l WHERE l.service_id=s.id) FROM assets_application_service s JOIN assets_application a ON a.id=s.application_id JOIN assets_business_system b ON b.id=s.business_system_id LEFT JOIN assets_business_environment e ON e.id=s.environment_id JOIN assets_application_version v ON v.id=s.application_version_id JOIN assets_application_deployment_template t ON t.id=s.deployment_template_id LEFT JOIN assets_cluster_profile c ON c.id=s.cluster_profile_id WHERE (?='' OR s.name LIKE ? OR s.code LIKE ?) ORDER BY s.business_system_id,s.environment_id,s.name LIMIT ? OFFSET ?`, search, pattern, pattern, page.Size, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]ApplicationService, 0)
	for rows.Next() {
		var item ApplicationService
		var created, updated time.Time
		var remark sql.NullString
		var env, profile sql.NullInt64
		var raw []byte
		if err = rows.Scan(&item.ID, &created, &updated, &remark, &item.Name, &item.Code, &item.TopologyType, &item.AccessAddress, &item.Enabled, &item.Application, &item.ApplicationName, &item.BusinessSystem, &item.BusinessSystemName, &env, &item.EnvironmentName, &item.ApplicationVersion, &item.ApplicationVersionName, &item.DeploymentTemplate, &item.DeploymentTemplateName, &profile, &item.ClusterProfileName, &raw, &item.LogCollectionEnabled, &item.DeploymentCount); err != nil {
			return nil, 0, err
		}
		item.CreateTime = timestamp(created)
		item.UpdateTime = timestamp(updated)
		item.Remark = stringValue(remark)
		item.Environment = nullableID(env)
		item.ClusterProfile = nullableID(profile)
		item.MacroValues = json.RawMessage(raw)
		items = append(items, item)
	}
	return items, count, rows.Err()
}
func (r *Repository) GetApplicationService(ctx context.Context, id int64) (ApplicationService, error) {
	var item ApplicationService
	var created, updated time.Time
	var remark sql.NullString
	var env, profile sql.NullInt64
	var raw []byte
	err := r.pool.QueryRowContext(ctx, `SELECT s.id,s.create_time,s.update_time,s.remark,s.name,s.code,s.topology_type,s.access_address,s.enabled,s.application_id,a.name,s.business_system_id,b.name,s.environment_id,COALESCE(e.name,''),s.application_version_id,v.version,s.deployment_template_id,t.name,s.cluster_profile_id,COALESCE(c.name,''),s.macro_values,s.log_collection_enabled,(SELECT COUNT(*) FROM assets_application_service_deployment l WHERE l.service_id=s.id) FROM assets_application_service s JOIN assets_application a ON a.id=s.application_id JOIN assets_business_system b ON b.id=s.business_system_id LEFT JOIN assets_business_environment e ON e.id=s.environment_id JOIN assets_application_version v ON v.id=s.application_version_id JOIN assets_application_deployment_template t ON t.id=s.deployment_template_id LEFT JOIN assets_cluster_profile c ON c.id=s.cluster_profile_id WHERE s.id=?`, id).Scan(&item.ID, &created, &updated, &remark, &item.Name, &item.Code, &item.TopologyType, &item.AccessAddress, &item.Enabled, &item.Application, &item.ApplicationName, &item.BusinessSystem, &item.BusinessSystemName, &env, &item.EnvironmentName, &item.ApplicationVersion, &item.ApplicationVersionName, &item.DeploymentTemplate, &item.DeploymentTemplateName, &profile, &item.ClusterProfileName, &raw, &item.LogCollectionEnabled, &item.DeploymentCount)
	if err != nil {
		return item, err
	}
	item.CreateTime = timestamp(created)
	item.UpdateTime = timestamp(updated)
	item.Remark = stringValue(remark)
	item.Environment = nullableID(env)
	item.ClusterProfile = nullableID(profile)
	item.MacroValues = json.RawMessage(raw)
	rows, err := r.pool.QueryContext(ctx, `SELECT deployment_id FROM assets_application_service_deployment WHERE service_id=? ORDER BY id`, id)
	if err != nil {
		return item, err
	}
	defer rows.Close()
	for rows.Next() {
		var deploymentID int64
		if err = rows.Scan(&deploymentID); err != nil {
			return item, err
		}
		item.MemberInstances = append(item.MemberInstances, deploymentID)
	}
	return item, rows.Err()
}
func (r *Repository) ListApplicationDeployments(ctx context.Context, page pagination.Page) ([]ApplicationDeployment, int64, error) {
	var count int64
	if err := r.pool.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets_application_deployment`).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.QueryContext(ctx, `SELECT d.id,d.create_time,d.update_time,d.remark,d.instance_name,d.enabled,d.host_id,COALESCE(h.ip,''),d.runtime_status,d.runtime_status_output,d.last_status_check_time,d.ha_role,d.runtime_variables FROM assets_application_deployment d JOIN assets_host h ON h.id=d.host_id ORDER BY d.id DESC LIMIT ? OFFSET ?`, page.Size, page.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]ApplicationDeployment, 0)
	for rows.Next() {
		var item ApplicationDeployment
		var created, updated time.Time
		var remark sql.NullString
		var checked sql.NullTime
		var raw []byte
		if err = rows.Scan(&item.ID, &created, &updated, &remark, &item.InstanceName, &item.Enabled, &item.Host, &item.HostIP, &item.RuntimeStatus, &item.RuntimeStatusOutput, &checked, &item.HaRole, &raw); err != nil {
			return nil, 0, err
		}
		item.CreateTime = timestamp(created)
		item.UpdateTime = timestamp(updated)
		item.Remark = stringValue(remark)
		item.LastStatusCheckTime = nullableTime(checked)
		item.RuntimeVariables = json.RawMessage(raw)
		item.ApplicationServiceIDs = []int64{}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, err
	}
	if err = r.attachApplicationServiceIDs(ctx, items); err != nil {
		return nil, 0, err
	}
	return items, count, nil
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
	placeholders := strings.Repeat("?,", len(ids))
	arguments := make([]any, 0, len(ids))
	for _, id := range ids {
		arguments = append(arguments, id)
	}
	rows, err := r.pool.QueryContext(ctx, `SELECT deployment_id,service_id FROM assets_application_service_deployment WHERE deployment_id IN (`+placeholders[:len(placeholders)-1]+`) ORDER BY deployment_id,service_id`, arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	linksByDeployment := make(map[int64][]int64)
	for rows.Next() {
		var deploymentID, serviceID int64
		if err = rows.Scan(&deploymentID, &serviceID); err != nil {
			return err
		}
		linksByDeployment[deploymentID] = append(linksByDeployment[deploymentID], serviceID)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for index := range items {
		if links, ok := linksByDeployment[items[index].ID]; ok {
			items[index].ApplicationServiceIDs = links
		}
	}
	return nil
}
