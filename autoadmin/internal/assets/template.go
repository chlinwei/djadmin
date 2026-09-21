package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"
	"database/sql"
)

type DeploymentTemplate struct {
	ID                 int64                   `json:"id"`
	CreateTime         string                  `json:"create_time"`
	UpdateTime         string                  `json:"update_time"`
	Remark             *string                 `json:"remark"`
	Name               string                  `json:"name"`
	ControlType        string                  `json:"control_type"`
	RunUser            string                  `json:"run_user"`
	RunGroup           string                  `json:"run_group"`
	AppHome            string                  `json:"app_home"`
	WorkDirectory      string                  `json:"work_directory"`
	ServiceName        string                  `json:"service_name"`
	SystemdScope       string                  `json:"systemd_scope"`
	HaSystemName       string                  `json:"ha_system_name"`
	HaClusterName      string                  `json:"ha_cluster_name"`
	HaResourceName     string                  `json:"ha_resource_name"`
	Enabled            bool                    `json:"enabled"`
	Application        int64                   `json:"application"`
	ApplicationName    string                  `json:"application_name"`
	MacroDefinitions   json.RawMessage         `json:"macro_definitions"`
	PortCount          int64                   `json:"port_count"`
	PathCount          int64                   `json:"path_count"`
	ConfigFileCount    int64                   `json:"config_file_count"`
	LogCount           int64                   `json:"log_count"`
	ControlActionCount int64                   `json:"control_action_count"`
	ServiceCount       int64                   `json:"service_count"`
	Ports              []TemplatePort          `json:"ports,omitempty"`
	Paths              []TemplatePath          `json:"paths,omitempty"`
	ConfigFiles        []TemplateConfigFile    `json:"config_files,omitempty"`
	Logs               []TemplateLog           `json:"logs,omitempty"`
	ControlActions     []TemplateControlAction `json:"control_actions,omitempty"`
	DockerConfig       *DockerConfig           `json:"docker_config,omitempty"`
	ComposeConfig      *ComposeConfig          `json:"compose_config,omitempty"`
}

type TemplatePort struct {
	ID             int64   `json:"id"`
	CreateTime     string  `json:"create_time"`
	UpdateTime     string  `json:"update_time"`
	Remark         *string `json:"remark"`
	Name           string  `json:"name"`
	Protocol       string  `json:"protocol"`
	BindAddress    string  `json:"bind_address"`
	Port           int     `json:"port"`
	Required       bool    `json:"required"`
	ExternalAccess bool    `json:"external_access"`
	CheckEnabled   bool    `json:"check_enabled"`
}
type TemplatePath struct {
	ID            int64   `json:"id"`
	CreateTime    string  `json:"create_time"`
	UpdateTime    string  `json:"update_time"`
	Remark        *string `json:"remark"`
	Name          string  `json:"name"`
	PathType      string  `json:"path_type"`
	Path          string  `json:"path"`
	Required      bool    `json:"required"`
	ExpectedOwner string  `json:"expected_owner"`
	ExpectedGroup string  `json:"expected_group"`
	ExpectedMode  string  `json:"expected_mode"`
	CheckEnabled  bool    `json:"check_enabled"`
}
type TemplateConfigFile struct {
	ID         int64   `json:"id"`
	CreateTime string  `json:"create_time"`
	UpdateTime string  `json:"update_time"`
	Remark     *string `json:"remark"`
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	FileFormat string  `json:"file_format"`
	Required   bool    `json:"required"`
}

// TemplateLog 是部署模板的日志定义。**不含采集开关**：是否采集由逻辑服务决定
// （`assets_application_service_log_setting.collection_enabled`，无覆盖行 = 采），
// 模板只描述"这条日志的路径怎么算、挂哪条处理规则"（迁移 000034 删掉了模板级开关）。
type TemplateLog struct {
	ID             int64           `json:"id"`
	CreateTime     string          `json:"create_time"`
	UpdateTime     string          `json:"update_time"`
	Remark         *string         `json:"remark"`
	Name           string          `json:"name"`
	PathPattern    string          `json:"path_pattern"`
	ProcessingRule *int64          `json:"processing_rule"`
	ExtraFields    json.RawMessage `json:"extra_fields"`
	// 采集过滤的模板级默认值（2026-09-19）：include 只采匹配、exclude 丢掉匹配；
	// NULL = 该方向不过滤。服务级可覆盖或显式关闭（见 LOG_COLLECTION_ARCHITECTURE §6）。
	FilterIncludeRule *int64 `json:"filter_include_rule"`
	FilterExcludeRule *int64 `json:"filter_exclude_rule"`
}
type TemplateControlAction struct {
	ID               int64           `json:"id"`
	CreateTime       string          `json:"create_time"`
	UpdateTime       string          `json:"update_time"`
	Remark           *string         `json:"remark"`
	Action           string          `json:"action"`
	Command          string          `json:"command"`
	TimeoutSeconds   int             `json:"timeout_seconds"`
	SuccessExitCodes json.RawMessage `json:"success_exit_codes"`
}
type DockerConfig struct {
	ID               int64   `json:"id"`
	CreateTime       string  `json:"create_time"`
	UpdateTime       string  `json:"update_time"`
	Remark           *string `json:"remark"`
	ContainerName    string  `json:"container_name"`
	DockerHost       string  `json:"docker_host"`
	ExpectedImage    string  `json:"expected_image"`
	ExpectedImageTag string  `json:"expected_image_tag"`
}
type ComposeConfig struct {
	ID               int64   `json:"id"`
	CreateTime       string  `json:"create_time"`
	UpdateTime       string  `json:"update_time"`
	Remark           *string `json:"remark"`
	ProjectName      string  `json:"project_name"`
	ServiceName      string  `json:"service_name"`
	ComposeFilePath  string  `json:"compose_file_path"`
	WorkingDirectory string  `json:"working_directory"`
	EnvFile          string  `json:"env_file"`
	ExpectedImage    string  `json:"expected_image"`
	ExpectedImageTag string  `json:"expected_image_tag"`
}

type DeploymentTemplateInput struct {
	Application      int64                         `json:"application"`
	Name             string                        `json:"name"`
	ControlType      string                        `json:"control_type"`
	RunUser          string                        `json:"run_user"`
	RunGroup         string                        `json:"run_group"`
	AppHome          string                        `json:"app_home"`
	WorkDirectory    string                        `json:"work_directory"`
	ServiceName      string                        `json:"service_name"`
	SystemdScope     string                        `json:"systemd_scope"`
	HaSystemName     string                        `json:"ha_system_name"`
	HaClusterName    string                        `json:"ha_cluster_name"`
	HaResourceName   string                        `json:"ha_resource_name"`
	Enabled          *bool                         `json:"enabled"`
	MacroDefinitions json.RawMessage               `json:"macro_definitions"`
	Ports            *[]TemplatePortInput          `json:"ports"`
	Paths            *[]TemplatePathInput          `json:"paths"`
	ConfigFiles      *[]TemplateConfigFileInput    `json:"config_files"`
	Logs             *[]TemplateLogInput           `json:"logs"`
	ControlActions   *[]TemplateControlActionInput `json:"control_actions"`
	DockerConfig     *DockerConfigInput            `json:"docker_config"`
	ComposeConfig    *ComposeConfigInput           `json:"compose_config"`
	Remark           *string                       `json:"remark"`
}
type TemplatePortInput struct {
	Name           string  `json:"name"`
	Protocol       string  `json:"protocol"`
	BindAddress    string  `json:"bind_address"`
	Port           int     `json:"port"`
	Required       *bool   `json:"required"`
	ExternalAccess *bool   `json:"external_access"`
	CheckEnabled   *bool   `json:"check_enabled"`
	Remark         *string `json:"remark"`
}
type TemplatePathInput struct {
	Name          string  `json:"name"`
	PathType      string  `json:"path_type"`
	Path          string  `json:"path"`
	Required      *bool   `json:"required"`
	ExpectedOwner string  `json:"expected_owner"`
	ExpectedGroup string  `json:"expected_group"`
	ExpectedMode  string  `json:"expected_mode"`
	CheckEnabled  *bool   `json:"check_enabled"`
	Remark        *string `json:"remark"`
}
type TemplateConfigFileInput struct {
	Name       string  `json:"name"`
	Path       string  `json:"path"`
	FileFormat string  `json:"file_format"`
	Required   *bool   `json:"required"`
	Remark     *string `json:"remark"`
}
type TemplateLogInput struct {
	// ID 是模板日志定义行的 id：非 0 表示"更新这一行"（id 保持不变，引用它的服务级覆盖
	// 因此不会失效），0/缺省表示新增。提交的 id 必须属于本模板，否则整个保存被拒。
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	PathPattern    string          `json:"path_pattern"`
	ProcessingRule *int64          `json:"processing_rule"`
	ExtraFields    json.RawMessage `json:"extra_fields"`
	Remark         *string         `json:"remark"`
	// 采集过滤默认值：规则类型必须与槽位一致（include 槽只接 include 规则），
	// 渲染时还会再校验一次方向（见 logcollect/log_collection_filter.go）。
	FilterIncludeRule *int64 `json:"filter_include_rule"`
	FilterExcludeRule *int64 `json:"filter_exclude_rule"`
}
type TemplateControlActionInput struct {
	Action           string          `json:"action"`
	Command          string          `json:"command"`
	TimeoutSeconds   int             `json:"timeout_seconds"`
	SuccessExitCodes json.RawMessage `json:"success_exit_codes"`
	Remark           *string         `json:"remark"`
}
type DockerConfigInput struct {
	ContainerName    string  `json:"container_name"`
	DockerHost       string  `json:"docker_host"`
	ExpectedImage    string  `json:"expected_image"`
	ExpectedImageTag string  `json:"expected_image_tag"`
	Remark           *string `json:"remark"`
}
type ComposeConfigInput struct {
	ProjectName      string  `json:"project_name"`
	ServiceName      string  `json:"service_name"`
	ComposeFilePath  string  `json:"compose_file_path"`
	WorkingDirectory string  `json:"working_directory"`
	EnvFile          string  `json:"env_file"`
	ExpectedImage    string  `json:"expected_image"`
	ExpectedImageTag string  `json:"expected_image_tag"`
	Remark           *string `json:"remark"`
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}
func jsonValue(value json.RawMessage, fallback string) []byte {
	if len(value) == 0 {
		return []byte(fallback)
	}
	return value
}

// nullableString / nullableInt 把可空指针转成 sqlc 的参数类型（nil 即 NULL）。
func nullableString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

// nullableIntPtr 用于"非可空 int64 字段写可空列"的场景（如 application_id 由前端保证非空）。
func nullableIntPtr(value int64) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: true}
}

func nullableInt(value *int64) sql.NullInt64 {
	if value == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

func (r *Repository) SaveDeploymentTemplate(ctx context.Context, id int64, input DeploymentTemplateInput) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin template transaction: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	queries := db.New(tx)
	name := strings.TrimSpace(input.Name)
	enabled := boolValue(input.Enabled, true)
	macro := jsonValue(input.MacroDefinitions, "[]")
	var templateID int64
	if id == 0 {
		templateID, err = queries.CreateDeploymentTemplate(ctx, db.CreateDeploymentTemplateParams{
			CreateTime: now, UpdateTime: now, Remark: nullableString(input.Remark), Name: name,
			ControlType: input.ControlType, RunUser: input.RunUser, RunGroup: input.RunGroup,
			AppHome: input.AppHome, WorkDirectory: input.WorkDirectory, ServiceName: input.ServiceName,
			HaSystemName: input.HaSystemName, HaClusterName: input.HaClusterName,
			HaResourceName: input.HaResourceName, Enabled: enabled, ApplicationID: nullableIntPtr(input.Application),
			SystemdScope: input.SystemdScope, MacroDefinitions: macro,
		})
	} else {
		err = queries.UpdateDeploymentTemplate(ctx, db.UpdateDeploymentTemplateParams{
			UpdateTime: now, Remark: nullableString(input.Remark), Name: name,
			ControlType: input.ControlType, RunUser: input.RunUser, RunGroup: input.RunGroup,
			AppHome: input.AppHome, WorkDirectory: input.WorkDirectory, ServiceName: input.ServiceName,
			HaSystemName: input.HaSystemName, HaClusterName: input.HaClusterName,
			HaResourceName: input.HaResourceName, Enabled: enabled, ApplicationID: nullableIntPtr(input.Application),
			SystemdScope: input.SystemdScope, MacroDefinitions: macro, ID: id,
		})
		templateID = id
	}
	if err != nil {
		return 0, err
	}
	// 嵌套子表按表名分派到各自的显式语句（原实现是运行时拼表名，sqlc 表达不了）。
	//
	// **日志定义不在这个"删后重建"的名单里**（2026-09-19）：它按 id 增量更新，见
	// applyTemplateLogWrites。整表删重建会让每条定义都换 id，从而（a）"改名"与"删除"
	// 无法区分，（b）引用了旧 id 的服务级覆盖行把删除挡住（外键无级联），
	// （c）只是改个路径也会让覆盖值失效。
	deleteNested := func(table string) error {
		switch table {
		case "assets_application_port":
			return queries.DeleteTemplatePorts(ctx, templateID)
		case "assets_application_path":
			return queries.DeleteTemplatePaths(ctx, templateID)
		case "assets_application_config_file":
			return queries.DeleteTemplateConfigFiles(ctx, templateID)
		case "assets_application_control_action":
			return queries.DeleteTemplateControlActions(ctx, templateID)
		case "assets_docker_control_config":
			return queries.DeleteTemplateDockerConfig(ctx, templateID)
		case "assets_docker_compose_control_config":
			return queries.DeleteTemplateComposeConfig(ctx, templateID)
		default:
			return fmt.Errorf("未知的模板子表: %s", table)
		}
	}
	if input.Ports != nil {
		if err = deleteNested("assets_application_port"); err != nil {
			return 0, err
		}
	}
	if input.Paths != nil {
		if err = deleteNested("assets_application_path"); err != nil {
			return 0, err
		}
	}
	if input.ConfigFiles != nil {
		if err = deleteNested("assets_application_config_file"); err != nil {
			return 0, err
		}
	}
	if input.Logs != nil {
		if err = applyTemplateLogWrites(ctx, queries, templateID, id == 0, *input.Logs, now); err != nil {
			return 0, err
		}
	}
	if input.ControlActions != nil {
		if err = deleteNested("assets_application_control_action"); err != nil {
			return 0, err
		}
	}
	if input.DockerConfig != nil {
		if err = deleteNested("assets_docker_control_config"); err != nil {
			return 0, err
		}
	}
	if input.ComposeConfig != nil {
		if err = deleteNested("assets_docker_compose_control_config"); err != nil {
			return 0, err
		}
	}
	if input.Ports != nil {
		for _, item := range *input.Ports {
			err = queries.CreateTemplatePort(ctx, db.CreateTemplatePortParams{
				CreateTime: now, UpdateTime: now, Remark: nullableString(item.Remark), Name: item.Name,
				Protocol: item.Protocol, BindAddress: item.BindAddress, Port: uint32(item.Port),
				Required: boolValue(item.Required, true), ExternalAccess: boolValue(item.ExternalAccess, false),
				CheckEnabled: boolValue(item.CheckEnabled, true), DeploymentTemplateID: templateID,
			})
			if err != nil {
				return 0, err
			}
		}
	}
	if input.Paths != nil {
		for _, item := range *input.Paths {
			err = queries.CreateTemplatePath(ctx, db.CreateTemplatePathParams{
				CreateTime: now, UpdateTime: now, Remark: nullableString(item.Remark), Name: item.Name,
				PathType: item.PathType, Path: item.Path, Required: boolValue(item.Required, true),
				ExpectedOwner: item.ExpectedOwner, ExpectedGroup: item.ExpectedGroup,
				ExpectedMode: item.ExpectedMode, CheckEnabled: boolValue(item.CheckEnabled, true),
				DeploymentTemplateID: templateID,
			})
			if err != nil {
				return 0, err
			}
		}
	}
	if input.ConfigFiles != nil {
		for _, item := range *input.ConfigFiles {
			err = queries.CreateTemplateConfigFile(ctx, db.CreateTemplateConfigFileParams{
				CreateTime: now, UpdateTime: now, Remark: nullableString(item.Remark), Name: item.Name,
				Path: item.Path, FileFormat: item.FileFormat, Required: boolValue(item.Required, true),
				DeploymentTemplateID: templateID,
			})
			if err != nil {
				return 0, err
			}
		}
	}
	if input.ControlActions != nil {
		for _, item := range *input.ControlActions {
			err = queries.CreateTemplateControlAction(ctx, db.CreateTemplateControlActionParams{
				CreateTime: now, UpdateTime: now, Remark: nullableString(item.Remark), Action: item.Action,
				Command: item.Command, TimeoutSeconds: uint32(item.TimeoutSeconds),
				SuccessExitCodes: jsonValue(item.SuccessExitCodes, "[]"), DeploymentTemplateID: templateID,
			})
			if err != nil {
				return 0, err
			}
		}
	}
	if input.DockerConfig != nil {
		item := input.DockerConfig
		err = queries.CreateTemplateDockerConfig(ctx, db.CreateTemplateDockerConfigParams{
			CreateTime: now, UpdateTime: now, Remark: nullableString(item.Remark), ContainerName: item.ContainerName,
			DockerHost: item.DockerHost, ExpectedImage: item.ExpectedImage, ExpectedImageTag: item.ExpectedImageTag,
			DeploymentTemplateID: templateID,
		})
	}
	if err != nil {
		return 0, err
	}
	if input.ComposeConfig != nil {
		item := input.ComposeConfig
		err = queries.CreateTemplateComposeConfig(ctx, db.CreateTemplateComposeConfigParams{
			CreateTime: now, UpdateTime: now, Remark: nullableString(item.Remark), ProjectName: item.ProjectName,
			ServiceName: item.ServiceName, ComposeFilePath: item.ComposeFilePath,
			WorkingDirectory: item.WorkingDirectory, EnvFile: item.EnvFile, ExpectedImage: item.ExpectedImage,
			ExpectedImageTag: item.ExpectedImageTag, DeploymentTemplateID: templateID,
		})
	}
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit template transaction: %w", err)
	}
	return templateID, nil
}

// templateLogWritePlan 是一次模板保存对日志定义的写计划（按 id 增量，不整表重建）。
type templateLogWritePlan struct {
	Updates []TemplateLogInput // ID 指向本模板已有行 → 原地更新（id 不变，服务级覆盖不失效）
	Inserts []TemplateLogInput // 无 ID → 新增
	Removed []int64            // 库里有、本次没提交 → 删除
}

// planTemplateLogWrites 纯函数：把"已有行 + 提交的日志列表"折算成增/改/删三组。
//
// 校验：提交的 ID 必须属于本模板（防跨模板误改），否则 ErrInvalidRelation。
// 注意 **不接受空名字**：模板日志名会直接变成片段文件名与文档的 log_name 维度值，
// 空名会生成 `<app>__<svc>__.yml` 这种监听不到文件的片段。
//
// creating=true（新建模板，含"复制模板"）：提交里带的 id 一律当新增——复制时前端会把源模板的
// 行原样提交，那些 id 属于源模板，不能拿来做本模板的行。
func planTemplateLogWrites(existingIDs []int64, submitted []TemplateLogInput, creating bool) (templateLogWritePlan, error) {
	plan := templateLogWritePlan{}
	existing := make(map[int64]bool, len(existingIDs))
	for _, id := range existingIDs {
		existing[id] = true
	}
	kept := make(map[int64]bool, len(submitted))
	for _, item := range submitted {
		if creating {
			item.ID = 0
		}
		if strings.TrimSpace(item.Name) == "" {
			return plan, apperror.New(apperror.CodeInvalidArgument, "日志定义名称不能为空")
		}
		switch {
		case item.ID <= 0:
			plan.Inserts = append(plan.Inserts, item)
		case !existing[item.ID]:
			return plan, ErrInvalidRelation
		default:
			kept[item.ID] = true
			plan.Updates = append(plan.Updates, item)
		}
	}
	for _, id := range existingIDs {
		if !kept[id] {
			plan.Removed = append(plan.Removed, id)
		}
	}
	return plan, nil
}

// applyTemplateLogWrites 把写计划落库。顺序是**先删后改再插**：
//   - 先删：删掉的行腾出的名字可以被同一次保存里的改名复用（不然唯一键会打回）；
//     删定义前先清掉引用它的服务级覆盖行（外键无级联，不清就删不掉；覆盖值依附于定义，
//     定义没了它也就没有意义了）；
//   - 再改：按 id 原地更新，`WHERE deployment_template_id` 兜住跨模板 id，影响 0 行按关联不存在报错；
//   - 后插：新增的行拿新 id。
func applyTemplateLogWrites(
	context context.Context, queries *db.Queries, templateID int64, creating bool,
	submitted []TemplateLogInput, now time.Time,
) error {
	var existingIDs []int64
	if !creating {
		rows, err := queries.ListTemplateLogDefinitions(context, templateID)
		if err != nil {
			return err
		}
		for _, row := range rows {
			existingIDs = append(existingIDs, row.ID)
		}
	}
	plan, err := planTemplateLogWrites(existingIDs, submitted, creating)
	if err != nil {
		return err
	}
	if len(plan.Removed) > 0 {
		if err = queries.DeleteServiceLogSettingsByDefinitionIDs(context, plan.Removed); err != nil {
			return err
		}
		if err = queries.DeleteTemplateLogDefinitionsByIDs(context, db.DeleteTemplateLogDefinitionsByIDsParams{
			DeploymentTemplateID: templateID, Ids: plan.Removed,
		}); err != nil {
			return err
		}
	}
	for _, item := range plan.Updates {
		result, updateErr := queries.UpdateTemplateLogDefinition(context, db.UpdateTemplateLogDefinitionParams{
			UpdateTime: now, Remark: nullableString(item.Remark), Name: strings.TrimSpace(item.Name),
			PathPattern: item.PathPattern, ExtraFields: jsonValue(item.ExtraFields, "{}"),
			ProcessingRuleID: nullableInt(item.ProcessingRule),
			// 模板级采集过滤默认值（服务级可覆盖/关闭，见 logcollect/log_collection_filter.go）
			FilterIncludeRuleID: nullableInt(item.FilterIncludeRule),
			FilterExcludeRuleID: nullableInt(item.FilterExcludeRule),
			ID:                  item.ID, DeploymentTemplateID: templateID,
		})
		if updateErr != nil {
			return updateErr
		}
		if affected, affectedErr := result.RowsAffected(); affectedErr == nil && affected == 0 {
			// 计划里已经确认该 id 属于本模板，走到这里说明并发下被删了：报"关联不存在"而不是静默跳过。
			return ErrInvalidRelation
		}
	}
	for _, item := range plan.Inserts {
		if err = queries.CreateTemplateLogDefinition(context, db.CreateTemplateLogDefinitionParams{
			CreateTime: now, UpdateTime: now, Remark: nullableString(item.Remark),
			Name: strings.TrimSpace(item.Name), PathPattern: item.PathPattern,
			DeploymentTemplateID: templateID, ExtraFields: jsonValue(item.ExtraFields, "{}"),
			ProcessingRuleID:    nullableInt(item.ProcessingRule),
			FilterIncludeRuleID: nullableInt(item.FilterIncludeRule),
			FilterExcludeRuleID: nullableInt(item.FilterExcludeRule),
		}); err != nil {
			return err
		}
	}
	return nil
}

// DeleteDeploymentTemplate 在一个事务里自底向上删掉模板及其全部子表，再删模板本身。
//
// 外键无级联，直接删父行会被子表（端口/路径/配置文件/日志定义/控制动作/Docker 配置）
// 挡在外键 1451 上——前端就会看到"提示成功但记录还在"。所以必须先删子表：
//   - 日志定义还被服务级覆盖行（assets_application_service_log_setting）引用，
//     删定义前先按定义 id 清覆盖行（与 applyTemplateLogWrites 的删除顺序一致）；
//   - 其余子表按模板 id 直接删。
//
// 若模板仍被逻辑服务引用（assets_application_service.deployment_template_id），
// 删模板父行同样命中 1451，translate 会转成 ErrDeleteProtected；因为整段在事务里，
// 前面的子表删除随事务一起回滚，不会留下"子表删了、模板还在"的半成品。
func (r *Repository) DeleteDeploymentTemplate(ctx context.Context, id int64) error {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin template transaction: %w", err)
	}
	defer tx.Rollback()
	queries := db.New(tx)
	if err = deleteTemplateChildren(ctx, queries, id); err != nil {
		return err
	}
	if err = queries.DeleteDeploymentTemplate(ctx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteTemplateChildren 删除一个模板的全部嵌套子表行（自底向上，满足外键顺序）。
func deleteTemplateChildren(ctx context.Context, queries *db.Queries, templateID int64) error {
	definitions, err := queries.ListTemplateLogDefinitions(ctx, templateID)
	if err != nil {
		return err
	}
	if len(definitions) > 0 {
		definitionIDs := make([]int64, 0, len(definitions))
		for _, row := range definitions {
			definitionIDs = append(definitionIDs, row.ID)
		}
		if err = queries.DeleteServiceLogSettingsByDefinitionIDs(ctx, definitionIDs); err != nil {
			return err
		}
	}
	for _, remove := range []func() error{
		func() error { return queries.DeleteTemplateLogDefinitions(ctx, templateID) },
		func() error { return queries.DeleteTemplatePorts(ctx, templateID) },
		func() error { return queries.DeleteTemplatePaths(ctx, templateID) },
		func() error { return queries.DeleteTemplateConfigFiles(ctx, templateID) },
		func() error { return queries.DeleteTemplateControlActions(ctx, templateID) },
		func() error { return queries.DeleteTemplateDockerConfig(ctx, templateID) },
		func() error { return queries.DeleteTemplateComposeConfig(ctx, templateID) },
	} {
		if err = remove(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) SaveDeploymentTemplate(ctx context.Context, id int64, input DeploymentTemplateInput) (DeploymentTemplate, error) {
	if input.Application < 1 || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.ControlType) == "" {
		return DeploymentTemplate{}, ErrInvalid
	}
	if err := validateDeploymentTemplate(input, id == 0); err != nil {
		return DeploymentTemplate{}, err
	}
	if input.MacroDefinitions == nil {
		input.MacroDefinitions = json.RawMessage("[]")
	}
	templateID, err := s.repository.SaveDeploymentTemplate(ctx, id, input)
	if err != nil {
		return DeploymentTemplate{}, translate(err)
	}
	return s.GetDeploymentTemplate(ctx, templateID)
}
func (s *Service) DeleteDeploymentTemplate(ctx context.Context, id int64) error {
	return translate(s.repository.DeleteDeploymentTemplate(ctx, id))
}

func templateFromList(row db.ListDeploymentTemplatesRow) DeploymentTemplate {
	return DeploymentTemplate{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, ControlType: row.ControlType, RunUser: row.RunUser, RunGroup: row.RunGroup, AppHome: row.AppHome, WorkDirectory: row.WorkDirectory, ServiceName: row.ServiceName, SystemdScope: row.SystemdScope, HaSystemName: row.HaSystemName, HaClusterName: row.HaClusterName, HaResourceName: row.HaResourceName, Enabled: row.Enabled, Application: row.ApplicationID, ApplicationName: row.ApplicationName, MacroDefinitions: row.MacroDefinitions, PortCount: row.PortCount, PathCount: row.PathCount, ConfigFileCount: row.ConfigFileCount, LogCount: row.LogCount, ControlActionCount: row.ControlActionCount, ServiceCount: row.ServiceCount}
}

func validateDeploymentTemplate(input DeploymentTemplateInput, creating bool) error {
	validTypes := map[string]bool{"systemd": true, "command": true, "external_ha": true, "docker": true, "docker_compose": true}
	if !validTypes[input.ControlType] {
		return ErrInvalid
	}
	if input.SystemdScope != "" && input.SystemdScope != "system" && input.SystemdScope != "user" {
		return ErrInvalid
	}
	var definitions []map[string]any
	if err := json.Unmarshal(jsonValue(input.MacroDefinitions, "[]"), &definitions); err != nil {
		return ErrInvalid
	}
	macroName := regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	seen := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		name, ok := definition["name"].(string)
		if !ok || !macroName.MatchString(name) || seen[name] {
			return ErrInvalid
		}
		seen[name] = true
		for key := range definition {
			if key != "name" && key != "value" && key != "description" {
				return ErrInvalid
			}
		}
		if strings.ContainsAny(fmt.Sprint(definition["value"]), "\r\n") {
			return ErrInvalid
		}
	}
	actions := make(map[string]bool)
	if input.ControlActions != nil {
		for _, action := range *input.ControlActions {
			if action.Action == "" || action.Command == "" || actions[action.Action] {
				return ErrInvalid
			}
			actions[action.Action] = true
		}
	}
	if input.ControlType == "systemd" && strings.TrimSpace(input.ServiceName) == "" {
		return ErrInvalid
	}
	if input.ControlType == "command" && (creating || input.ControlActions != nil) && !(actions["start"] && actions["stop"] && actions["status"]) {
		return ErrInvalid
	}
	if input.ControlType == "external_ha" && (strings.TrimSpace(input.HaResourceName) == "" || ((creating || input.ControlActions != nil) && !actions["status"])) {
		return ErrInvalid
	}
	if input.ControlType == "docker" && (input.DockerConfig == nil || strings.TrimSpace(input.DockerConfig.ContainerName) == "") {
		return ErrInvalid
	}
	if input.ControlType == "docker_compose" && (input.ComposeConfig == nil || strings.TrimSpace(input.ComposeConfig.ProjectName) == "" || strings.TrimSpace(input.ComposeConfig.ServiceName) == "") {
		return ErrInvalid
	}
	return nil
}
func templateFromDetail(row db.GetDeploymentTemplateRow) DeploymentTemplate {
	return DeploymentTemplate{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, ControlType: row.ControlType, RunUser: row.RunUser, RunGroup: row.RunGroup, AppHome: row.AppHome, WorkDirectory: row.WorkDirectory, ServiceName: row.ServiceName, SystemdScope: row.SystemdScope, HaSystemName: row.HaSystemName, HaClusterName: row.HaClusterName, HaResourceName: row.HaResourceName, Enabled: row.Enabled, Application: row.ApplicationID, ApplicationName: row.ApplicationName, MacroDefinitions: row.MacroDefinitions, PortCount: row.PortCount, PathCount: row.PathCount, ConfigFileCount: row.ConfigFileCount, LogCount: row.LogCount, ControlActionCount: row.ControlActionCount, ServiceCount: row.ServiceCount}
}

func (r *Repository) ListDeploymentTemplates(ctx context.Context, applicationID sql.NullInt64, search string, page pagination.Page) ([]db.ListDeploymentTemplatesRow, int64, error) {
	patternValue := pattern(search) // 位置参数版本的两个 LIKE 用 string 字段，取 .String（Valid 恒 true）
	args := db.CountDeploymentTemplatesParams{ApplicationID: applicationID, Column3: search, Name: patternValue.String, Name_2: patternValue.String}
	count, err := r.queries.CountDeploymentTemplates(ctx, args)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListDeploymentTemplates(ctx, db.ListDeploymentTemplatesParams{ApplicationID: applicationID, Column3: search, Name: patternValue.String, Name_2: patternValue.String, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}

func (r *Repository) GetDeploymentTemplate(ctx context.Context, id int64) (db.GetDeploymentTemplateRow, error) {
	return r.queries.GetDeploymentTemplate(ctx, id)
}

func (r *Repository) ListApplicationServicesByTemplate(ctx context.Context, templateID int64) ([]db.ListApplicationServicesByTemplateRow, error) {
	return r.queries.ListApplicationServicesByTemplate(ctx, templateID)
}

func (s *Service) ListDeploymentTemplates(ctx context.Context, applicationID sql.NullInt64, search string, page pagination.Page) ([]DeploymentTemplate, int64, error) {
	rows, count, err := s.repository.ListDeploymentTemplates(ctx, applicationID, search, page)
	result := make([]DeploymentTemplate, 0, len(rows))
	for _, row := range rows {
		result = append(result, templateFromList(row))
	}
	return result, count, translate(err)
}

func (s *Service) GetDeploymentTemplate(ctx context.Context, id int64) (DeploymentTemplate, error) {
	row, err := s.repository.GetDeploymentTemplate(ctx, id)
	if err != nil {
		return templateFromDetail(row), translate(err)
	}
	item := templateFromDetail(row)
	nested, err := s.repository.loadTemplateNested(ctx, id)
	if err != nil {
		return DeploymentTemplate{}, translate(err)
	}
	item.Ports, item.Paths, item.ConfigFiles, item.Logs, item.ControlActions = nested.Ports, nested.Paths, nested.ConfigFiles, nested.Logs, nested.ControlActions
	item.DockerConfig, item.ComposeConfig = nested.DockerConfig, nested.ComposeConfig
	return item, nil
}
