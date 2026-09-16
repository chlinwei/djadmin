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
		return nil, translate(err)
	}
	if instanceName == "" {
		return nil, ErrAgentUnavailable
	}
	params["control_action"] = action
	raw, _ := json.Marshal(params)
	response, err := gateway.Execute(ctx, instanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("app-control-%d", time.Now().UnixNano()), Type: "custom", Action: "control_application", ParamsJson: string(raw), TimeoutSeconds: 120})
	if err != nil {
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

var _ = sql.ErrNoRows
