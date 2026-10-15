package assets

import (
	"encoding/json"
	"fmt"
	"strings"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/logmacro"
)

func deploymentControlParams(row db.ListDeploymentControlContextsRow, actions []db.ListDeploymentControlActionsRow) (map[string]any, error) {
	values := logmacro.Merge(
		logmacro.TemplateDefaults(string(row.MacroDefinitions)),
		logmacro.ParseValues(string(row.MacroValues)),
		logmacro.ParseValues(string(row.RuntimeVariables)),
	)
	if _, exists := values["APP_HOME"]; !exists && row.AppHome != "" {
		values["APP_HOME"] = row.AppHome
	}
	resolved := map[string]string{}
	visiting := map[string]bool{}
	var resolve func(string) (string, error)
	resolve = func(name string) (string, error) {
		if value, ok := resolved[name]; ok {
			return value, nil
		}
		if visiting[name] {
			return "", apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("应用控制宏存在循环引用：%s", name))
		}
		visiting[name] = true
		value := values[name]
		for _, dependency := range logmacro.Pending(value) {
			if _, exists := values[dependency]; !exists {
				continue
			}
			replacement, err := resolve(dependency)
			if err != nil {
				return "", err
			}
			value = strings.ReplaceAll(value, "${"+dependency+"}", replacement)
		}
		visiting[name] = false
		resolved[name] = value
		return value, nil
	}
	render := func(value, field string, shell bool) (string, error) {
		for _, name := range logmacro.Pending(value) {
			if _, exists := values[name]; !exists {
				continue
			}
			replacement, err := resolve(name)
			if err != nil {
				return "", err
			}
			value = strings.ReplaceAll(value, "${"+name+"}", replacement)
		}
		// 命令中的未声明变量留给目标用户的 shell；用户名和路径等结构字段必须完整解析。
		if pending := logmacro.Pending(value); !shell && len(pending) != 0 {
			return "", apperror.New(apperror.CodeInvalidArgument, fmt.Sprintf("应用控制字段 %s 存在未解析的宏：%s", field, strings.Join(pending, ", ")))
		}
		return value, nil
	}
	params := map[string]any{
		"control_type": row.ControlType, "systemd_scope": row.SystemdScope,
		"instance_name": row.DeploymentInstanceName,
	}
	for _, field := range []struct{ name, value string }{
		{"run_user", row.RunUser}, {"work_directory", row.WorkDirectory},
		{"app_home", row.AppHome}, {"service_name", row.ServiceName},
	} {
		value, err := render(field.value, field.name, false)
		if err != nil {
			return nil, err
		}
		if field.name == "run_user" && field.value != "" && strings.TrimSpace(value) == "" {
			return nil, apperror.New(apperror.CodeInvalidArgument, "应用控制运行用户的宏解析结果为空")
		}
		params[field.name] = value
	}
	controlActions := map[string]any{}
	for _, action := range actions {
		command, err := render(action.Command, action.Action, true)
		if err != nil {
			return nil, err
		}
		var codes []int
		if err := json.Unmarshal(action.SuccessExitCodes, &codes); err != nil {
			return nil, fmt.Errorf("decode %s control success exit codes: %w", action.Action, err)
		}
		controlActions[action.Action] = map[string]any{
			"command": command, "timeout_seconds": action.TimeoutSeconds, "success_exit_codes": codes,
		}
	}
	params["control_actions"] = controlActions
	return params, nil
}
