package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
	db "autoadmin/internal/platform/database/generated"
)

var deploymentGateway *agent.Gateway

func SetDeploymentGateway(gateway *agent.Gateway) { deploymentGateway = gateway }

func (r *Repository) deploymentControl(ctx context.Context, id int64) (string, map[string]any, error) {
	tx, err := r.pool.BeginTx(ctx, nil)
	if err != nil {
		return "", nil, err
	}
	defer tx.Rollback()
	queries := db.New(tx)
	contexts, err := deploymentControlContexts(ctx, queries, id)
	if err != nil {
		return "", nil, err
	}
	rows, err := queries.ListDeploymentControlActions(ctx, contexts[0].TemplateID)
	if err != nil {
		return "", nil, err
	}
	params, err := deploymentControlParams(contexts[0], rows)
	if err != nil {
		return "", nil, err
	}
	// 相同模板仍可能被服务宏覆盖成不同控制目标，不能任取其中一个执行。
	for _, row := range contexts[1:] {
		other, err := deploymentControlParams(row, rows)
		if err != nil {
			return "", nil, err
		}
		if !reflect.DeepEqual(params, other) {
			return "", nil, ErrDeploymentControlConflict
		}
	}
	if err := tx.Commit(); err != nil {
		return "", nil, err
	}
	return contexts[0].InstanceName, params, nil
}

func deploymentControlContext(ctx context.Context, queries *db.Queries, id int64) (db.ListDeploymentControlContextsRow, error) {
	rows, err := deploymentControlContexts(ctx, queries, id)
	if err != nil {
		return db.ListDeploymentControlContextsRow{}, err
	}
	return rows[0], nil
}

func deploymentControlContexts(ctx context.Context, queries *db.Queries, id int64) ([]db.ListDeploymentControlContextsRow, error) {
	if _, err := queries.LockApplicationDeployment(ctx, id); err != nil {
		return nil, err
	}
	rows, err := queries.ListDeploymentControlContexts(ctx, id)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, sql.ErrNoRows
	}
	// 共享实例只有一份运行状态；模板必须唯一，不能任取一个关联服务的命令执行。
	for _, row := range rows[1:] {
		if row.TemplateID != rows[0].TemplateID {
			return nil, ErrDeploymentTemplateConflict
		}
	}
	return rows, nil
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
	controlType, _ := params["control_type"].(string)
	runtime := deploymentControlRuntimeStatus(action, controlType, response)
	output := fmt.Sprintf("action=%s control_type=%v service_name=%v systemd_scope=%v exit_code=%d\n%s",
		action, params["control_type"], params["service_name"], params["systemd_scope"], response.ExitCode,
		strings.TrimSpace(strings.Join([]string{response.Stdout, response.Stderr, response.ErrorMessage}, "\n")))
	if err := s.repository.updateRuntimeStatus(ctx, id, runtime, output); err != nil {
		return nil, err
	}
	return map[string]any{"job_id": response.JobId, "action": action, "status": status, "output": response.Stdout, "exit_code": response.ExitCode, "runtime_status": runtime}, nil
}

func deploymentControlRuntimeStatus(action, controlType string, response *pb.AutomationExecuteResponse) string {
	runtime := "unknown"
	if action == "status" {
		if response.ExitCode == 0 {
			runtime = "running"
		} else {
			runtime = "stopped"
			// systemctl 的未知单元、权限或总线错误不是“已停止”；只接受明确的非活跃状态。
			state := strings.TrimSpace(response.Stdout)
			if controlType == "systemd" && (response.ExitCode != 3 || (state != "inactive" && state != "failed")) {
				runtime = "error"
			}
		}
	}
	if response.Status != "success" {
		runtime = "error"
	}
	return runtime
}

// persistRuntimeCheckFailure 只在 status 检查失败时把 error 与原因写回，start/stop 的失败不改运行状态；
// 否则前端在 Agent 未连接 / 命令执行失败时只会看到"未知"且没有任何报错。
func (s *Service) persistRuntimeCheckFailure(ctx context.Context, id int64, action string, cause error) {
	if action != "status" || cause == nil {
		return
	}
	if err := s.repository.updateRuntimeStatus(ctx, id, "error", cause.Error()); err != nil {
		slog.Error("persist deployment runtime check failure", "deployment_id", id, "cause", cause, "error", err)
	}
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
