package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
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
	LogRetentionTier     *int64                    `json:"log_retention_tier"`
	MemberConfigs        *[]ServiceMemberInput     `json:"member_configs"`
	Remark               *string                   `json:"remark"`
	LogSettings          *[]ServiceLogSettingInput `json:"log_settings"`
}
type ServiceMemberInput struct {
	Deployment int64 `json:"deployment"`
	Enabled    *bool `json:"enabled"`
}
// ServiceLogSettingInput 是 (逻辑服务 × 日志定义) 的覆盖值写入口。
// **不含解析规则**：规则只由部署模板的日志定义决定（迁移 000035 删掉了服务级覆盖），
// 提交体里带 processing_rule 会被静默忽略。采集开关/档位/过滤规则仍可覆盖。
type ServiceLogSettingInput struct {
	LogDefinition        int64  `json:"log_definition"`
	RetentionTier        *int64 `json:"retention_tier"`
	CollectionEnabled    *bool  `json:"collection_enabled"`
	CollectionFilterRule *int64 `json:"collection_filter_rule"`
}

// ApplicationDeploymentInput 是部署实例的写入口。注意**不含**逻辑服务关联：
// 实例与服务的绑定由服务侧的 member_configs 维护（SaveApplicationService，删除重建整组关联），
// 实例侧改它会与"服务侧全量重建"的语义冲突（同一个实例可能同时属于多个服务）。
// 前端 DeploymentDialog 曾把 application_service 放进提交体，后端结构体没有这个字段 →
// 静默忽略，用户以为"绑定成功"其实没有写库；该字段已从前端 payload 移除（2026-09-18）。
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
	queries := db.New(tx)
	name, code := strings.TrimSpace(input.Name), strings.TrimSpace(input.Code)
	var serviceID int64
	if id == 0 {
		serviceID, err = queries.CreateApplicationService(ctx, db.CreateApplicationServiceParams{
			CreateTime: now, UpdateTime: now, Remark: nullableString(input.Remark), Name: name, Code: code,
			TopologyType: input.TopologyType, AccessAddress: input.AccessAddress, Enabled: enabled,
			ApplicationID: input.Application, ClusterProfileID: profile, EnvironmentID: env,
			ApplicationVersionID: input.ApplicationVersion, DeploymentTemplateID: input.DeploymentTemplate,
			BusinessSystemID: input.BusinessSystem, MacroValues: macro, LogCollectionEnabled: logs,
			LogRetentionTierID: nullableInt(input.LogRetentionTier),
		})
	} else {
		err = queries.UpdateApplicationService(ctx, db.UpdateApplicationServiceParams{
			UpdateTime: now, Remark: nullableString(input.Remark), Name: name, Code: code,
			TopologyType: input.TopologyType, AccessAddress: input.AccessAddress, Enabled: enabled,
			ApplicationID: input.Application, ClusterProfileID: profile, EnvironmentID: env,
			ApplicationVersionID: input.ApplicationVersion, DeploymentTemplateID: input.DeploymentTemplate,
			BusinessSystemID: input.BusinessSystem, MacroValues: macro, LogCollectionEnabled: logs,
			LogRetentionTierID: nullableInt(input.LogRetentionTier), ID: id,
		})
		serviceID = id
	}
	if err != nil {
		return 0, err
	}
	if input.MemberConfigs != nil {
		if err = queries.DeleteServiceDeployments(ctx, serviceID); err != nil {
			return 0, err
		}
		seen := map[int64]bool{}
		for _, member := range *input.MemberConfigs {
			if member.Deployment < 1 || seen[member.Deployment] {
				return 0, fmt.Errorf("invalid service member")
			}
			seen[member.Deployment] = true
			if err = queries.CreateServiceDeployment(ctx, db.CreateServiceDeploymentParams{
				CreateTime: now, UpdateTime: now, Enabled: boolValue(member.Enabled, true),
				DeploymentID: member.Deployment, ServiceID: serviceID,
			}); err != nil {
				return 0, err
			}
		}
	}
	if input.LogSettings != nil {
		if err = queries.DeleteServiceLogSettings(ctx, serviceID); err != nil {
			return 0, err
		}
		for _, setting := range *input.LogSettings {
			if setting.LogDefinition < 1 {
				return 0, fmt.Errorf("invalid service log definition")
			}
			var collection *bool
			if setting.CollectionEnabled != nil {
				collection = setting.CollectionEnabled
			}
			if err = queries.CreateServiceLogSetting(ctx, db.CreateServiceLogSettingParams{
				CreateTime: now, UpdateTime: now, CollectionEnabled: collection,
				LogDefinitionID: setting.LogDefinition, RetentionTierID: nullableInt(setting.RetentionTier),
				ServiceID:              serviceID,
				CollectionFilterRuleID: nullableInt(setting.CollectionFilterRule),
			}); err != nil {
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
	return r.queries.DeleteApplicationService(ctx, id)
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
	queries := db.New(tx)
	instanceName := strings.TrimSpace(input.InstanceName)
	var deploymentID int64
	if id == 0 {
		deploymentID, err = queries.CreateApplicationDeployment(ctx, db.CreateApplicationDeploymentParams{
			CreateTime: now, UpdateTime: now, Remark: nullableString(input.Remark), InstanceName: instanceName,
			Enabled: enabled, HostID: input.Host, RuntimeStatus: status,
			RuntimeStatusOutput: input.RuntimeStatusOutput, HaRole: role, RuntimeVariables: raw,
		})
	} else {
		err = queries.UpdateApplicationDeployment(ctx, db.UpdateApplicationDeploymentParams{
			UpdateTime: now, Remark: nullableString(input.Remark), InstanceName: instanceName,
			Enabled: enabled, HostID: input.Host, RuntimeStatus: status,
			RuntimeStatusOutput: input.RuntimeStatusOutput, HaRole: role, RuntimeVariables: raw, ID: id,
		})
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
	return r.queries.DeleteApplicationDeployment(ctx, id)
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
	// 新增服务 / 开启采集时抽样认证一次（架构文档 §4.8）。后台 best-effort，不影响本次保存结果；
	// 是否真的需要认证由 MaybeAutoVerifyServiceLogFormats 按行状态自己判断（已 verified 的跳过）。
	if boolValue(input.LogCollectionEnabled, false) {
		s.MaybeAutoVerifyServiceLogFormats(ctx, saved)
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
	// 回读必须按 id。历史缺陷：这里曾用列表查询的"第 1 页、每页 1 条"来取刚保存的行，
	// 而列表是 `ORDER BY id DESC LIMIT ?`，Size: 1 恒返回**全库 id 最大的一台**——
	// 于是编辑任何不是最新的一台实例，写入虽然成功，接口却返回 404「资产不存在」
	// （2026-09-18 用户在 yilake nginx 上撞到）。
	item, err := s.repository.GetApplicationDeployment(ctx, saved)
	if err != nil {
		// 目标不存在时 UPDATE 影响 0 行且不报错，这里按 id 读会拿到 sql.ErrNoRows：
		// 翻译成 ErrNotFound（404「资产不存在」），保持原来的语义而不是变成 500。
		return ApplicationDeployment{}, translate(err)
	}
	return item, nil
}
func (s *Service) DeleteApplicationDeployment(ctx context.Context, id int64) error {
	return translate(s.repository.DeleteApplicationDeployment(ctx, id))
}
