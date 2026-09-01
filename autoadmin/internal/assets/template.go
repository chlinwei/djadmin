package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
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
type TemplateLog struct {
	ID                int64           `json:"id"`
	CreateTime        string          `json:"create_time"`
	UpdateTime        string          `json:"update_time"`
	Remark            *string         `json:"remark"`
	Name              string          `json:"name"`
	PathPattern       string          `json:"path_pattern"`
	CollectionEnabled bool            `json:"collection_enabled"`
	ProcessingRule    *int64          `json:"processing_rule"`
	ExtraFields       json.RawMessage `json:"extra_fields"`
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
	Name              string          `json:"name"`
	PathPattern       string          `json:"path_pattern"`
	CollectionEnabled bool            `json:"collection_enabled"`
	ProcessingRule    *int64          `json:"processing_rule"`
	ExtraFields       json.RawMessage `json:"extra_fields"`
	Remark            *string         `json:"remark"`
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
func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullableInt(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}

func (r *Repository) SaveDeploymentTemplate(ctx context.Context, id int64, input DeploymentTemplateInput) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin template transaction: %w", err)
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	args := []any{now, now, nullableString(input.Remark), strings.TrimSpace(input.Name), input.ControlType, input.RunUser, input.RunGroup, input.AppHome, input.WorkDirectory, input.ServiceName, input.HaSystemName, input.HaClusterName, input.HaResourceName, boolValue(input.Enabled, true), input.Application, input.SystemdScope, jsonValue(input.MacroDefinitions, "[]")}
	var templateID int64
	if id == 0 {
		result, execErr := tx.ExecContext(ctx, `INSERT INTO assets_application_deployment_template (create_time,update_time,remark,name,control_type,run_user,run_group,app_home,work_directory,service_name,ha_system_name,ha_cluster_name,ha_resource_name,enabled,application_id,systemd_scope,macro_definitions) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, args...)
		if execErr != nil {
			return 0, execErr
		}
		templateID, err = result.LastInsertId()
	} else {
		args = append(args, id)
		_, err = tx.ExecContext(ctx, `UPDATE assets_application_deployment_template SET update_time=?,remark=?,name=?,control_type=?,run_user=?,run_group=?,app_home=?,work_directory=?,service_name=?,ha_system_name=?,ha_cluster_name=?,ha_resource_name=?,enabled=?,application_id=?,systemd_scope=?,macro_definitions=? WHERE id=?`, args[1:]...)
		templateID = id
	}
	if err != nil {
		return 0, err
	}
	deleteNested := func(table string) error {
		_, deleteErr := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE deployment_template_id=?", templateID)
		return deleteErr
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
		if err = deleteNested("assets_application_log_definition"); err != nil {
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
			_, err = tx.ExecContext(ctx, `INSERT INTO assets_application_port (create_time,update_time,remark,name,protocol,bind_address,port,required,external_access,check_enabled,deployment_template_id) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, now, now, nullableString(item.Remark), item.Name, item.Protocol, item.BindAddress, item.Port, boolValue(item.Required, true), boolValue(item.ExternalAccess, false), boolValue(item.CheckEnabled, true), templateID)
			if err != nil {
				return 0, err
			}
		}
	}
	if input.Paths != nil {
		for _, item := range *input.Paths {
			_, err = tx.ExecContext(ctx, `INSERT INTO assets_application_path (create_time,update_time,remark,name,path_type,path,required,expected_owner,expected_group,expected_mode,check_enabled,deployment_template_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`, now, now, nullableString(item.Remark), item.Name, item.PathType, item.Path, boolValue(item.Required, true), item.ExpectedOwner, item.ExpectedGroup, item.ExpectedMode, boolValue(item.CheckEnabled, true), templateID)
			if err != nil {
				return 0, err
			}
		}
	}
	if input.ConfigFiles != nil {
		for _, item := range *input.ConfigFiles {
			_, err = tx.ExecContext(ctx, `INSERT INTO assets_application_config_file (create_time,update_time,remark,name,path,file_format,required,deployment_template_id) VALUES (?,?,?,?,?,?,?,?)`, now, now, nullableString(item.Remark), item.Name, item.Path, item.FileFormat, boolValue(item.Required, true), templateID)
			if err != nil {
				return 0, err
			}
		}
	}
	if input.Logs != nil {
		for _, item := range *input.Logs {
			_, err = tx.ExecContext(ctx, `INSERT INTO assets_application_log_definition (create_time,update_time,remark,name,path_pattern,collection_enabled,deployment_template_id,extra_fields,processing_rule_id) VALUES (?,?,?,?,?,?,?,?,?)`, now, now, nullableString(item.Remark), item.Name, item.PathPattern, item.CollectionEnabled, templateID, jsonValue(item.ExtraFields, "{}"), nullableInt(item.ProcessingRule))
			if err != nil {
				return 0, err
			}
		}
	}
	if input.ControlActions != nil {
		for _, item := range *input.ControlActions {
			_, err = tx.ExecContext(ctx, `INSERT INTO assets_application_control_action (create_time,update_time,remark,action,command,timeout_seconds,success_exit_codes,deployment_template_id) VALUES (?,?,?,?,?,?,?,?)`, now, now, nullableString(item.Remark), item.Action, item.Command, item.TimeoutSeconds, jsonValue(item.SuccessExitCodes, "[]"), templateID)
			if err != nil {
				return 0, err
			}
		}
	}
	if input.DockerConfig != nil {
		item := input.DockerConfig
		_, err = tx.ExecContext(ctx, `INSERT INTO assets_docker_control_config (create_time,update_time,remark,container_name,docker_host,expected_image,expected_image_tag,deployment_template_id) VALUES (?,?,?,?,?,?,?,?)`, now, now, nullableString(item.Remark), item.ContainerName, item.DockerHost, item.ExpectedImage, item.ExpectedImageTag, templateID)
	}
	if err != nil {
		return 0, err
	}
	if input.ComposeConfig != nil {
		item := input.ComposeConfig
		_, err = tx.ExecContext(ctx, `INSERT INTO assets_docker_compose_control_config (create_time,update_time,remark,project_name,service_name,compose_file_path,working_directory,env_file,expected_image,expected_image_tag,deployment_template_id) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, now, now, nullableString(item.Remark), item.ProjectName, item.ServiceName, item.ComposeFilePath, item.WorkingDirectory, item.EnvFile, item.ExpectedImage, item.ExpectedImageTag, templateID)
	}
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit template transaction: %w", err)
	}
	return templateID, nil
}

func (r *Repository) DeleteDeploymentTemplate(ctx context.Context, id int64) error {
	_, err := r.pool.ExecContext(ctx, `DELETE FROM assets_application_deployment_template WHERE id=?`, id)
	return err
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
	return DeploymentTemplate{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, ControlType: row.ControlType, RunUser: row.RunUser, RunGroup: row.RunGroup, AppHome: row.AppHome, WorkDirectory: row.WorkDirectory, ServiceName: row.ServiceName, SystemdScope: row.SystemdScope, HaSystemName: row.HaSystemName, HaClusterName: row.HaClusterName, HaResourceName: row.HaResourceName, Enabled: row.Enabled, Application: row.ApplicationID, ApplicationName: row.ApplicationName, MacroDefinitions: row.MacroDefinitions, PortCount: row.PortCount, PathCount: row.PathCount, ConfigFileCount: row.ConfigFileCount, LogCount: row.LogCount, ControlActionCount: row.ControlActionCount}
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
	return DeploymentTemplate{ID: row.ID, CreateTime: timestamp(row.CreateTime), UpdateTime: timestamp(row.UpdateTime), Remark: stringValue(row.Remark), Name: row.Name, ControlType: row.ControlType, RunUser: row.RunUser, RunGroup: row.RunGroup, AppHome: row.AppHome, WorkDirectory: row.WorkDirectory, ServiceName: row.ServiceName, SystemdScope: row.SystemdScope, HaSystemName: row.HaSystemName, HaClusterName: row.HaClusterName, HaResourceName: row.HaResourceName, Enabled: row.Enabled, Application: row.ApplicationID, ApplicationName: row.ApplicationName, MacroDefinitions: row.MacroDefinitions, PortCount: row.PortCount, PathCount: row.PathCount, ConfigFileCount: row.ConfigFileCount, LogCount: row.LogCount, ControlActionCount: row.ControlActionCount}
}

func (r *Repository) ListDeploymentTemplates(ctx context.Context, search string, page pagination.Page) ([]db.ListDeploymentTemplatesRow, int64, error) {
	patternValue := pattern(search)
	args := db.CountDeploymentTemplatesParams{Column1: search, Name: patternValue, Name_2: patternValue}
	count, err := r.queries.CountDeploymentTemplates(ctx, args)
	if err != nil {
		return nil, 0, err
	}
	rows, err := r.queries.ListDeploymentTemplates(ctx, db.ListDeploymentTemplatesParams{Column1: search, Name: patternValue, Name_2: patternValue, Limit: page.Size, Offset: page.Offset})
	return rows, count, err
}

func (r *Repository) GetDeploymentTemplate(ctx context.Context, id int64) (db.GetDeploymentTemplateRow, error) {
	return r.queries.GetDeploymentTemplate(ctx, id)
}

func (s *Service) ListDeploymentTemplates(ctx context.Context, search string, page pagination.Page) ([]DeploymentTemplate, int64, error) {
	rows, count, err := s.repository.ListDeploymentTemplates(ctx, search, page)
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
