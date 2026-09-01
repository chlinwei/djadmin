package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
)

var deploymentGateway *agent.Gateway

func SetDeploymentGateway(gateway *agent.Gateway) { deploymentGateway = gateway }

func (r *Repository) deploymentControl(ctx context.Context, id int64) (string, map[string]any, error) {
	var agentID, controlType, runUser, workDirectory, appHome, serviceName, systemdScope, instanceName string
	var macro []byte
	err := r.pool.QueryRowContext(ctx, `SELECT h.agent_id,t.control_type,t.run_user,t.work_directory,t.app_home,t.service_name,t.systemd_scope,t.macro_definitions,d.instance_name FROM assets_application_deployment d JOIN assets_host h ON h.id=d.host_id JOIN assets_application_service_deployment l ON l.deployment_id=d.id JOIN assets_application_service s ON s.id=l.service_id JOIN assets_application_deployment_template t ON t.id=s.deployment_template_id WHERE d.id=? LIMIT 1`, id).Scan(&agentID, &controlType, &runUser, &workDirectory, &appHome, &serviceName, &systemdScope, &macro, &instanceName)
	if err != nil {
		return "", nil, err
	}
	params := map[string]any{"control_type": controlType, "run_user": runUser, "work_directory": workDirectory, "app_home": appHome, "service_name": serviceName, "systemd_scope": systemdScope, "instance_name": instanceName}
	var macroValues any
	if json.Unmarshal(macro, &macroValues) == nil {
		params["macro_definitions"] = macroValues
	}
	actions := map[string]any{}
	rows, err := r.pool.QueryContext(ctx, `SELECT action,command,timeout_seconds,success_exit_codes FROM assets_application_control_action WHERE deployment_template_id=(SELECT deployment_template_id FROM assets_application_service s JOIN assets_application_service_deployment l ON l.service_id=s.id WHERE l.deployment_id=? LIMIT 1)`, id)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var action, command string
		var timeout int
		var exits []byte
		if err = rows.Scan(&action, &command, &timeout, &exits); err != nil {
			return "", nil, err
		}
		var codes any
		_ = json.Unmarshal(exits, &codes)
		actions[action] = map[string]any{"command": command, "timeout_seconds": timeout, "success_exit_codes": codes}
	}
	if err = rows.Err(); err != nil {
		return "", nil, err
	}
	params["control_actions"] = actions
	return agentID, params, nil
}
func (r *Repository) updateRuntimeStatus(ctx context.Context, id int64, status, output string) error {
	_, err := r.pool.ExecContext(ctx, `UPDATE assets_application_deployment SET update_time=?,runtime_status=?,runtime_status_output=?,last_status_check_time=? WHERE id=?`, time.Now().UTC(), status, output, time.Now().UTC(), id)
	return err
}
func (s *Service) executeDeploymentControl(ctx context.Context, gateway *agent.Gateway, id int64, action string) (map[string]any, error) {
	if gateway == nil {
		gateway = deploymentGateway
	}
	agentID, params, err := s.repository.deploymentControl(ctx, id)
	if err != nil {
		return nil, translate(err)
	}
	if agentID == "" {
		return nil, ErrAgentUnavailable
	}
	params["control_action"] = action
	raw, _ := json.Marshal(params)
	response, err := gateway.Execute(ctx, agentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("app-control-%d", time.Now().UnixNano()), Type: "custom", Action: "control_application", ParamsJson: string(raw), TimeoutSeconds: 120})
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
