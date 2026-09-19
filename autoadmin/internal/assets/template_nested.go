package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// rawJSONOrObject / rawJSONOrEmptyArray 给 json 列兜底（空值分别回退 {} / []，与迁移前的 scanRaw 同义）。
func rawJSONOrObject(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("{}")
	}
	return raw
}

func rawJSONOrEmptyArray(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}
func (r *Repository) loadTemplateNested(ctx context.Context, id int64) (DeploymentTemplate, error) {
	result := DeploymentTemplate{}
	queries := r.queries
	ports, err := queries.ListTemplatePorts(ctx, id)
	if err != nil {
		return result, err
	}
	for _, row := range ports {
		var item TemplatePort
		item.ID, item.CreateTime, item.UpdateTime = row.ID, timestamp(row.CreateTime), timestamp(row.UpdateTime)
		item.Remark = stringValue(row.Remark)
		item.Name, item.Protocol, item.BindAddress = row.Name, row.Protocol, row.BindAddress
		item.Port, item.Required = int(row.Port), row.Required
		item.ExternalAccess, item.CheckEnabled = row.ExternalAccess, row.CheckEnabled
		result.Ports = append(result.Ports, item)
	}
	paths, err := queries.ListTemplatePaths(ctx, id)
	if err != nil {
		return result, err
	}
	for _, row := range paths {
		var item TemplatePath
		item.ID, item.CreateTime, item.UpdateTime = row.ID, timestamp(row.CreateTime), timestamp(row.UpdateTime)
		item.Remark = stringValue(row.Remark)
		item.Name, item.PathType, item.Path = row.Name, row.PathType, row.Path
		item.Required, item.ExpectedOwner = row.Required, row.ExpectedOwner
		item.ExpectedGroup, item.ExpectedMode = row.ExpectedGroup, row.ExpectedMode
		item.CheckEnabled = row.CheckEnabled
		result.Paths = append(result.Paths, item)
	}
	configFiles, err := queries.ListTemplateConfigFiles(ctx, id)
	if err != nil {
		return result, err
	}
	for _, row := range configFiles {
		var item TemplateConfigFile
		item.ID, item.CreateTime, item.UpdateTime = row.ID, timestamp(row.CreateTime), timestamp(row.UpdateTime)
		item.Remark = stringValue(row.Remark)
		item.Name, item.Path, item.FileFormat, item.Required = row.Name, row.Path, row.FileFormat, row.Required
		result.ConfigFiles = append(result.ConfigFiles, item)
	}
	logs, err := queries.ListTemplateLogDefinitions(ctx, id)
	if err != nil {
		return result, err
	}
	for _, row := range logs {
		var item TemplateLog
		item.ID, item.CreateTime, item.UpdateTime = row.ID, timestamp(row.CreateTime), timestamp(row.UpdateTime)
		item.Remark = stringValue(row.Remark)
		item.Name, item.PathPattern = row.Name, row.PathPattern
		item.ExtraFields = rawJSONOrObject(row.ExtraFields)
		item.ProcessingRule = intPtr(row.ProcessingRuleID)
		item.FilterIncludeRule = intPtr(row.FilterIncludeRuleID)
		item.FilterExcludeRule = intPtr(row.FilterExcludeRuleID)
		result.Logs = append(result.Logs, item)
	}
	controlActions, err := queries.ListTemplateControlActions(ctx, id)
	if err != nil {
		return result, err
	}
	for _, row := range controlActions {
		var item TemplateControlAction
		item.ID, item.CreateTime, item.UpdateTime = row.ID, timestamp(row.CreateTime), timestamp(row.UpdateTime)
		item.Remark = stringValue(row.Remark)
		item.Action, item.Command = row.Action, row.Command
		item.TimeoutSeconds = int(row.TimeoutSeconds)
		item.SuccessExitCodes = rawJSONOrEmptyArray(row.SuccessExitCodes)
		result.ControlActions = append(result.ControlActions, item)
	}
	dockerConfig, err := queries.GetTemplateDockerConfig(ctx, id)
	if err == nil {
		result.DockerConfig = &DockerConfig{
			ID: dockerConfig.ID, CreateTime: timestamp(dockerConfig.CreateTime),
			UpdateTime: timestamp(dockerConfig.UpdateTime), Remark: stringValue(dockerConfig.Remark),
			ContainerName: dockerConfig.ContainerName, DockerHost: dockerConfig.DockerHost,
			ExpectedImage: dockerConfig.ExpectedImage, ExpectedImageTag: dockerConfig.ExpectedImageTag,
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	composeConfig, err := queries.GetTemplateComposeConfig(ctx, id)
	if err == nil {
		result.ComposeConfig = &ComposeConfig{
			ID: composeConfig.ID, CreateTime: timestamp(composeConfig.CreateTime),
			UpdateTime: timestamp(composeConfig.UpdateTime), Remark: stringValue(composeConfig.Remark),
			ProjectName: composeConfig.ProjectName, ServiceName: composeConfig.ServiceName,
			ComposeFilePath: composeConfig.ComposeFilePath, WorkingDirectory: composeConfig.WorkingDirectory,
			EnvFile:       composeConfig.EnvFile,
			ExpectedImage: composeConfig.ExpectedImage, ExpectedImageTag: composeConfig.ExpectedImageTag,
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	return result, nil
}

func intPtr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}
