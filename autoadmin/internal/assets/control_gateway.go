package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
	db "autoadmin/internal/platform/database/generated"
)

var deploymentGateway *agent.Gateway

func SetDeploymentGateway(gateway *agent.Gateway) { deploymentGateway = gateway }

func (r *Repository) deploymentControl(ctx context.Context, id int64) (string, map[string]any, error) {
	contextRow, err := r.queries.GetDeploymentControlContext(ctx, id)
	if err != nil {
		return "", nil, err
	}
	params := map[string]any{"control_type": contextRow.ControlType, "run_user": contextRow.RunUser,
		"work_directory": contextRow.WorkDirectory, "app_home": contextRow.AppHome,
		"service_name": contextRow.ServiceName, "systemd_scope": contextRow.SystemdScope,
		"instance_name": contextRow.DeploymentInstanceName}
	var macroValues any
	if json.Unmarshal(contextRow.MacroDefinitions, &macroValues) == nil {
		params["macro_definitions"] = macroValues
	}
	rows, err := r.queries.ListDeploymentControlActions(ctx, id)
	if err != nil {
		return "", nil, err
	}
	actions := map[string]any{}
	for _, row := range rows {
		var codes any
		_ = json.Unmarshal(row.SuccessExitCodes, &codes)
		actions[row.Action] = map[string]any{"command": row.Command, "timeout_seconds": row.TimeoutSeconds, "success_exit_codes": codes}
	}
	params["control_actions"] = actions
	return contextRow.InstanceName, params, nil
}
func (r *Repository) updateRuntimeStatus(ctx context.Context, id int64, status, output string) error {
	now := time.Now().UTC()
	return r.queries.UpdateDeploymentRuntimeStatus(ctx, db.UpdateDeploymentRuntimeStatusParams{
		UpdateTime: now, RuntimeStatus: status, RuntimeStatusOutput: output,
		LastStatusCheckTime: sql.NullTime{Time: now, Valid: true}, ID: id,
	})
}
func (s *Service) executeDeploymentControl(ctx context.Context, gateway *agent.Gateway, id int64, action string) (map[string]any, error) {
	if gateway == nil {
		gateway = deploymentGateway
	}
	instanceName, params, err := s.repository.deploymentControl(ctx, id)
	if err != nil {
		s.persistRuntimeCheckFailure(ctx, id, action, err)
		return nil, translate(err)
	}
	if instanceName == "" {
		s.persistRuntimeCheckFailure(ctx, id, action, ErrAgentUnavailable)
		return nil, ErrAgentUnavailable
	}
	params["control_action"] = action
	raw, _ := json.Marshal(params)
	response, err := gateway.Execute(ctx, instanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("app-control-%d", time.Now().UnixNano()), Type: "custom", Action: "control_application", ParamsJson: string(raw), TimeoutSeconds: 120})
	if err != nil {
		// 保留原始错误原因，便于前端区分"Agent 未连接"与其他执行失败。
		s.persistRuntimeCheckFailure(ctx, id, action, err)
		return nil, ErrAgentUnavailable
	}
	status := response.Status
	runtime := "unknown"
	if action == "status" {
		if response.ExitCode == 0 {
			runtime = "running"
		} else {
			runtime = "stopped"
		}
	}
	if status != "success" {
		runtime = "error"
	}
	_ = s.repository.updateRuntimeStatus(ctx, id, runtime, response.ErrorMessage+response.Stderr)
	return map[string]any{"job_id": response.JobId, "action": action, "status": status, "output": response.Stdout, "exit_code": response.ExitCode, "runtime_status": runtime}, nil
}

// persistRuntimeCheckFailure 只在 status 检查失败时把 error 与原因写回，start/stop 的失败不改运行状态；
// 否则前端在 Agent 未连接 / 命令执行失败时只会看到"未知"且没有任何报错。
func (s *Service) persistRuntimeCheckFailure(ctx context.Context, id int64, action string, cause error) {
	if action != "status" || cause == nil {
		return
	}
	_ = s.repository.updateRuntimeStatus(ctx, id, "error", cause.Error())
}

// checkDeploymentRuntimeStatus 对单个部署实例执行一次 status 检查并返回落库后的状态。
func (s *Service) checkDeploymentRuntimeStatus(ctx context.Context, id int64) string {
	result, err := s.executeDeploymentControl(ctx, nil, id, "status")
	if err != nil {
		return "error"
	}
	status, _ := result["runtime_status"].(string)
	if status == "" {
		return "unknown"
	}
	return status
}

var _ = sql.ErrNoRows
