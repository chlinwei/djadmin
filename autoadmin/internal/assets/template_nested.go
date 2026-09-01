package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"
)

func scanRaw(raw *[]byte) json.RawMessage {
	if raw == nil {
		return json.RawMessage("{}")
	}
	return json.RawMessage(*raw)
}
func (r *Repository) loadTemplateNested(ctx context.Context, id int64) (DeploymentTemplate, error) {
	result := DeploymentTemplate{}
	rows, err := r.pool.QueryContext(ctx, `SELECT id,create_time,update_time,remark,name,protocol,bind_address,port,required,external_access,check_enabled FROM assets_application_port WHERE deployment_template_id=? ORDER BY protocol,port`, id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item TemplatePort
		var remark sql.NullString
		var created, updated time.Time
		if err = rows.Scan(&item.ID, &created, &updated, &remark, &item.Name, &item.Protocol, &item.BindAddress, &item.Port, &item.Required, &item.ExternalAccess, &item.CheckEnabled); err != nil {
			rows.Close()
			return result, err
		}
		item.Remark = stringValue(remark)
		item.CreateTime, item.UpdateTime = timestamp(created), timestamp(updated)
		result.Ports = append(result.Ports, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	rows, err = r.pool.QueryContext(ctx, `SELECT id,create_time,update_time,remark,name,path_type,path,required,expected_owner,expected_group,expected_mode,check_enabled FROM assets_application_path WHERE deployment_template_id=? ORDER BY path_type,id`, id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item TemplatePath
		var remark sql.NullString
		var created, updated time.Time
		if err = rows.Scan(&item.ID, &created, &updated, &remark, &item.Name, &item.PathType, &item.Path, &item.Required, &item.ExpectedOwner, &item.ExpectedGroup, &item.ExpectedMode, &item.CheckEnabled); err != nil {
			rows.Close()
			return result, err
		}
		item.Remark = stringValue(remark)
		item.CreateTime, item.UpdateTime = timestamp(created), timestamp(updated)
		result.Paths = append(result.Paths, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	rows, err = r.pool.QueryContext(ctx, `SELECT id,create_time,update_time,remark,name,path,file_format,required FROM assets_application_config_file WHERE deployment_template_id=? ORDER BY id`, id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item TemplateConfigFile
		var remark sql.NullString
		var created, updated time.Time
		if err = rows.Scan(&item.ID, &created, &updated, &remark, &item.Name, &item.Path, &item.FileFormat, &item.Required); err != nil {
			rows.Close()
			return result, err
		}
		item.Remark = stringValue(remark)
		item.CreateTime, item.UpdateTime = timestamp(created), timestamp(updated)
		result.ConfigFiles = append(result.ConfigFiles, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	rows, err = r.pool.QueryContext(ctx, `SELECT id,create_time,update_time,remark,name,path_pattern,collection_enabled,extra_fields,processing_rule_id FROM assets_application_log_definition WHERE deployment_template_id=? ORDER BY id`, id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item TemplateLog
		var remark sql.NullString
		var raw []byte
		var rule sql.NullInt64
		var created, updated time.Time
		if err = rows.Scan(&item.ID, &created, &updated, &remark, &item.Name, &item.PathPattern, &item.CollectionEnabled, &raw, &rule); err != nil {
			rows.Close()
			return result, err
		}
		item.Remark = stringValue(remark)
		item.CreateTime, item.UpdateTime = timestamp(created), timestamp(updated)
		item.ExtraFields = scanRaw(&raw)
		item.ProcessingRule = intPtr(rule)
		result.Logs = append(result.Logs, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	rows, err = r.pool.QueryContext(ctx, `SELECT id,create_time,update_time,remark,action,command,timeout_seconds,success_exit_codes FROM assets_application_control_action WHERE deployment_template_id=? ORDER BY id`, id)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var item TemplateControlAction
		var remark sql.NullString
		var raw []byte
		var created, updated time.Time
		if err = rows.Scan(&item.ID, &created, &updated, &remark, &item.Action, &item.Command, &item.TimeoutSeconds, &raw); err != nil {
			rows.Close()
			return result, err
		}
		item.Remark = stringValue(remark)
		item.CreateTime, item.UpdateTime = timestamp(created), timestamp(updated)
		item.SuccessExitCodes = scanRaw(&raw)
		result.ControlActions = append(result.ControlActions, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	var item DockerConfig
	var remark sql.NullString
	var created, updated time.Time
	err = r.pool.QueryRowContext(ctx, `SELECT id,create_time,update_time,remark,container_name,docker_host,expected_image,expected_image_tag FROM assets_docker_control_config WHERE deployment_template_id=?`, id).Scan(&item.ID, &created, &updated, &remark, &item.ContainerName, &item.DockerHost, &item.ExpectedImage, &item.ExpectedImageTag)
	if err == nil {
		item.CreateTime, item.UpdateTime = timestamp(created), timestamp(updated)
		item.Remark = stringValue(remark)
		result.DockerConfig = &item
	} else if err != sql.ErrNoRows {
		return result, err
	}
	var compose ComposeConfig
	remark = sql.NullString{}
	err = r.pool.QueryRowContext(ctx, `SELECT id,create_time,update_time,remark,project_name,service_name,compose_file_path,working_directory,env_file,expected_image,expected_image_tag FROM assets_docker_compose_control_config WHERE deployment_template_id=?`, id).Scan(&compose.ID, &created, &updated, &remark, &compose.ProjectName, &compose.ServiceName, &compose.ComposeFilePath, &compose.WorkingDirectory, &compose.EnvFile, &compose.ExpectedImage, &compose.ExpectedImageTag)
	if err == nil {
		compose.CreateTime, compose.UpdateTime = timestamp(created), timestamp(updated)
		compose.Remark = stringValue(remark)
		result.ComposeConfig = &compose
	} else if err != sql.ErrNoRows {
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
