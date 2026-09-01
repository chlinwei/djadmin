package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"autoadmin/internal/shared/pagination"
)

type ApplicationServiceInput struct {
	Name                 string                    `json:"name"`
	Code                 string                    `json:"code"`
	TopologyType         string                    `json:"topology_type"`
	AccessAddress        string                    `json:"access_address"`
	Enabled              *bool                     `json:"enabled"`
	Application          int64                     `json:"application"`
	BusinessSystem       int64                     `json:"business_system"`
	Environment          *int64                    `json:"environment"`
	ApplicationVersion   int64                     `json:"application_version"`
	DeploymentTemplate   int64                     `json:"deployment_template"`
	ClusterProfile       *int64                    `json:"cluster_profile"`
	MacroValues          json.RawMessage           `json:"macro_values"`
	LogCollectionEnabled *bool                     `json:"log_collection_enabled"`
	MemberConfigs        *[]ServiceMemberInput     `json:"member_configs"`
	Remark               *string                   `json:"remark"`
	LogSettings          *[]ServiceLogSettingInput `json:"log_settings"`
}
type ServiceMemberInput struct {
	Deployment int64 `json:"deployment"`
	Enabled    *bool `json:"enabled"`
}
type ServiceLogSettingInput struct {
	LogDefinition        int64  `json:"log_definition"`
	RetentionTier        *int64 `json:"retention_tier"`
	CollectionEnabled    *bool  `json:"collection_enabled"`
	CollectionFilterRule *int64 `json:"collection_filter_rule"`
	ProcessingRule       *int64 `json:"processing_rule"`
}
type ApplicationDeploymentInput struct {
	InstanceName        string          `json:"instance_name"`
	Enabled             *bool           `json:"enabled"`
	Host                int64           `json:"host"`
	RuntimeStatus       string          `json:"runtime_status"`
	RuntimeStatusOutput string          `json:"runtime_status_output"`
	HaRole              string          `json:"ha_role"`
	RuntimeVariables    json.RawMessage `json:"runtime_variables"`
	Remark              *string         `json:"remark"`
}

func (r *Repository) SaveApplicationService(ctx context.Context, id int64, input ApplicationServiceInput) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	env, profile := nullableInt(input.Environment), nullableInt(input.ClusterProfile)
	macro := jsonValue(input.MacroValues, "{}")
	enabled := boolValue(input.Enabled, true)
	logs := boolValue(input.LogCollectionEnabled, false)
	var serviceID int64
	if id == 0 {
		result, execErr := tx.ExecContext(ctx, `INSERT INTO assets_application_service (create_time,update_time,remark,name,code,topology_type,access_address,enabled,application_id,cluster_profile_id,environment_id,application_version_id,deployment_template_id,business_system_id,macro_values,log_collection_enabled) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, now, now, nullableString(input.Remark), strings.TrimSpace(input.Name), strings.TrimSpace(input.Code), input.TopologyType, input.AccessAddress, enabled, input.Application, profile, env, input.ApplicationVersion, input.DeploymentTemplate, input.BusinessSystem, macro, logs)
		if execErr != nil {
			return 0, execErr
		}
		serviceID, err = result.LastInsertId()
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE assets_application_service SET update_time=?,remark=?,name=?,code=?,topology_type=?,access_address=?,enabled=?,application_id=?,cluster_profile_id=?,environment_id=?,application_version_id=?,deployment_template_id=?,business_system_id=?,macro_values=?,log_collection_enabled=? WHERE id=?`, now, nullableString(input.Remark), strings.TrimSpace(input.Name), strings.TrimSpace(input.Code), input.TopologyType, input.AccessAddress, enabled, input.Application, profile, env, input.ApplicationVersion, input.DeploymentTemplate, input.BusinessSystem, macro, logs, id)
		serviceID = id
	}
	if err != nil {
		return 0, err
	}
	if input.MemberConfigs != nil {
		if _, err = tx.ExecContext(ctx, `DELETE FROM assets_application_service_deployment WHERE service_id=?`, serviceID); err != nil {
			return 0, err
		}
		seen := map[int64]bool{}
		for _, member := range *input.MemberConfigs {
			if member.Deployment < 1 || seen[member.Deployment] {
				return 0, fmt.Errorf("invalid service member")
			}
			seen[member.Deployment] = true
			if _, err = tx.ExecContext(ctx, `INSERT INTO assets_application_service_deployment (create_time,update_time,remark,enabled,deployment_id,service_id) VALUES (?,?,?,?,?,?)`, now, now, nil, boolValue(member.Enabled, true), member.Deployment, serviceID); err != nil {
				return 0, err
			}
		}
	}
	if input.LogSettings != nil {
		if _, err = tx.ExecContext(ctx, `DELETE FROM assets_application_service_log_setting WHERE service_id=?`, serviceID); err != nil {
			return 0, err
		}
		for _, setting := range *input.LogSettings {
			if setting.LogDefinition < 1 {
				return 0, fmt.Errorf("invalid service log definition")
			}
			var collection any
			if setting.CollectionEnabled != nil {
				collection = *setting.CollectionEnabled
			}
			if _, err = tx.ExecContext(ctx, `INSERT INTO assets_application_service_log_setting (create_time,update_time,remark,collection_enabled,log_definition_id,retention_tier_id,service_id,processing_rule_id,collection_filter_rule_id) VALUES (?,?,?,?,?,?,?,?,?)`, now, now, nil, collection, setting.LogDefinition, nullableInt(setting.RetentionTier), serviceID, nullableInt(setting.ProcessingRule), nullableInt(setting.CollectionFilterRule)); err != nil {
				return 0, err
			}
		}
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return serviceID, nil
}
func (r *Repository) DeleteApplicationService(ctx context.Context, id int64) error {
	_, err := r.pool.ExecContext(ctx, `DELETE FROM assets_application_service WHERE id=?`, id)
	return err
}
func (r *Repository) SaveApplicationDeployment(ctx context.Context, id int64, input ApplicationDeploymentInput) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	status := input.RuntimeStatus
	if status == "" {
		status = "unknown"
	}
	role := input.HaRole
	if role == "" {
		role = "unknown"
	}
	raw := jsonValue(input.RuntimeVariables, "{}")
	enabled := boolValue(input.Enabled, true)
	var deploymentID int64
	if id == 0 {
		result, execErr := tx.ExecContext(ctx, `INSERT INTO assets_application_deployment (create_time,update_time,remark,instance_name,enabled,host_id,runtime_status,runtime_status_output,ha_role,runtime_variables) VALUES (?,?,?,?,?,?,?,?,?,?)`, now, now, nullableString(input.Remark), strings.TrimSpace(input.InstanceName), enabled, input.Host, status, input.RuntimeStatusOutput, role, raw)
		if execErr != nil {
			return 0, execErr
		}
		deploymentID, err = result.LastInsertId()
	} else {
		_, err = tx.ExecContext(ctx, `UPDATE assets_application_deployment SET update_time=?,remark=?,instance_name=?,enabled=?,host_id=?,runtime_status=?,runtime_status_output=?,ha_role=?,runtime_variables=? WHERE id=?`, now, nullableString(input.Remark), strings.TrimSpace(input.InstanceName), enabled, input.Host, status, input.RuntimeStatusOutput, role, raw, id)
		deploymentID = id
	}
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return deploymentID, nil
}
func (r *Repository) DeleteApplicationDeployment(ctx context.Context, id int64) error {
	_, err := r.pool.ExecContext(ctx, `DELETE FROM assets_application_deployment WHERE id=?`, id)
	return err
}

func (s *Service) SaveApplicationService(ctx context.Context, id int64, input ApplicationServiceInput) (ApplicationService, error) {
	if input.Application < 1 || input.BusinessSystem < 1 || input.ApplicationVersion < 1 || input.DeploymentTemplate < 1 || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Code) == "" {
		return ApplicationService{}, ErrInvalid
	}
	if input.MemberConfigs != nil && len(*input.MemberConfigs) == 0 && input.TopologyType != "" {
		return ApplicationService{}, ErrInvalid
	}
	saved, err := s.repository.SaveApplicationService(ctx, id, input)
	if err != nil {
		return ApplicationService{}, translate(err)
	}
	return s.repository.GetApplicationService(ctx, saved)
}
func (s *Service) DeleteApplicationService(ctx context.Context, id int64) error {
	return translate(s.repository.DeleteApplicationService(ctx, id))
}
func (s *Service) SaveApplicationDeployment(ctx context.Context, id int64, input ApplicationDeploymentInput) (ApplicationDeployment, error) {
	if input.Host < 1 || strings.TrimSpace(input.InstanceName) == "" {
		return ApplicationDeployment{}, ErrInvalid
	}
	saved, err := s.repository.SaveApplicationDeployment(ctx, id, input)
	if err != nil {
		return ApplicationDeployment{}, translate(err)
	}
	items, _, err := s.repository.ListApplicationDeployments(ctx, pagination.Page{Number: 1, Size: 1, Offset: 0})
	if err != nil {
		return ApplicationDeployment{}, translate(err)
	}
	for _, item := range items {
		if item.ID == saved {
			return item, nil
		}
	}
	return ApplicationDeployment{}, ErrNotFound
}
func (s *Service) DeleteApplicationDeployment(ctx context.Context, id int64) error {
	return translate(s.repository.DeleteApplicationDeployment(ctx, id))
}
