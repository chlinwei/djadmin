package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

type runTask struct {
	ID, GroupID, ServiceID              int64
	Name, GroupName, Scope, ServiceName string
	Concurrency, Timeout                int
	Checks                              []runCheck
	SelectedHostIDs                     []int64
	Enabled, GroupEnabled               bool
}

type runCheck struct {
	Name     string         `json:"name"`
	Executor string         `json:"executor"`
	Config   map[string]any `json:"config"`
	Severity string         `json:"severity"`
	Order    uint32         `json:"order"`
}

type runTarget struct {
	ID, HostID, DeploymentID           int64
	Name, HostName, HostIP, AgentID    string
	AgentOnline                        bool
	AppHome, RunUser, WorkDirectory    string
	InstanceName, Version, ServiceName string
}

type checkResult struct {
	Key, Type, Name, Status, Severity, Message string
	Expected, Actual                           any
}

func (handler *Handler) RunTask(context *gin.Context) {
	task, message, err := handler.prepareRunTask(context, parseID(context.Param("id")))
	if err != nil {
		response.Error(context, err)
		return
	}
	if message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	targets, message, err := handler.resolveTargets(context, task)
	if err != nil {
		response.Error(context, err)
		return
	}
	if message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	claims, _ := identity.ClaimsFromContext(context)
	userID, username := int32(0), ""
	if claims != nil {
		userID, username = claims.UserID, claims.Username
	}
	executionID, err := handler.createExecution(context, task, targets, userID, username)
	if err != nil {
		response.Error(context, err)
		return
	}
	go handler.execute(executionID, task, targets)
	response.Success(context, gin.H{"execution_id": executionID, "status": "pending"})
}

func (handler *Handler) prepareRunTask(ctx context.Context, id int64) (runTask, string, error) {
	var task runTask
	var selected []byte
	err := handler.db.QueryRowContext(ctx, `SELECT t.id,t.name,t.group_id,COALESCE(t.logical_service_id,0),t.selected_host_ids,t.concurrency,t.timeout_seconds,t.enabled,g.name,g.scope,g.enabled,COALESCE(s.name,'') FROM inspection_task t JOIN inspection_group g ON g.id=t.group_id LEFT JOIN assets_application_service s ON s.id=t.logical_service_id WHERE t.id=?`, id).Scan(&task.ID, &task.Name, &task.GroupID, &task.ServiceID, &selected, &task.Concurrency, &task.Timeout, &task.Enabled, &task.GroupName, &task.Scope, &task.GroupEnabled, &task.ServiceName)
	if err == sql.ErrNoRows {
		return task, "巡检任务不存在", nil
	}
	if err != nil {
		return task, "", err
	}
	if !task.Enabled || !task.GroupEnabled {
		return task, "巡检任务或巡检组已禁用", nil
	}
	if err = json.Unmarshal(selected, &task.SelectedHostIDs); err != nil {
		return task, "巡检任务的主机范围数据无效", nil
	}
	checkRows, err := db.New(handler.db).ListEnabledInspectionChecksForRun(ctx, task.GroupID)
	if err != nil {
		return task, "", err
	}
	task.Checks = make([]runCheck, 0, len(checkRows))
	for _, row := range checkRows {
		var config map[string]any
		if err = json.Unmarshal(row.Config, &config); err != nil {
			return task, "巡检检查项配置数据无效", nil
		}
		if config == nil {
			config = map[string]any{}
		}
		task.Checks = append(task.Checks, runCheck{
			Name: row.Name, Executor: row.Executor,
			Config: config, Severity: row.Severity, Order: row.Order,
		})
	}
	if len(task.Checks) == 0 {
		return task, "巡检组没有启用的检查项", nil
	}
	return task, "", nil
}

func (handler *Handler) resolveTargets(ctx context.Context, task runTask) ([]runTarget, string, error) {
	targets := make([]runTarget, 0)
	if task.Scope == "per_host" {
		if len(task.SelectedHostIDs) == 0 {
			return nil, "巡检任务的主机范围为空", nil
		}
		placeholders, arguments := make([]string, len(task.SelectedHostIDs)), make([]any, len(task.SelectedHostIDs))
		for index, id := range task.SelectedHostIDs {
			placeholders[index], arguments[index] = "?", id
		}
		rows, err := handler.db.QueryContext(ctx, `SELECT id,COALESCE(instance_name,''),COALESCE(ip,''),COALESCE(agent_id,''),agent_online FROM assets_host WHERE is_deleted_in_cloud=FALSE AND id IN (`+strings.Join(placeholders, ",")+`) ORDER BY instance_name,id`, arguments...)
		if err != nil {
			return nil, "", err
		}
		defer rows.Close()
		for rows.Next() {
			var target runTarget
			if err = rows.Scan(&target.HostID, &target.HostName, &target.HostIP, &target.AgentID, &target.AgentOnline); err != nil {
				return nil, "", err
			}
			target.Name, target.RunUser, target.WorkDirectory = target.HostName, "root", "/"
			if target.Name == "" {
				target.Name = target.HostIP
			}
			targets = append(targets, target)
		}
		if err = rows.Err(); err != nil {
			return nil, "", err
		}
	} else {
		if task.ServiceID == 0 {
			return nil, "巡检任务未绑定逻辑服务", nil
		}
		rows, err := handler.db.QueryContext(ctx, `SELECT d.id,d.instance_name,h.id,COALESCE(h.instance_name,''),COALESCE(h.ip,''),COALESCE(h.agent_id,''),h.agent_online,t.app_home,t.run_user,t.work_directory,v.version,t.service_name FROM assets_application_deployment d JOIN assets_host h ON h.id=d.host_id JOIN assets_application_service_deployment l ON l.deployment_id=d.id AND l.enabled=TRUE JOIN assets_application_service s ON s.id=l.service_id JOIN assets_application_deployment_template t ON t.id=s.deployment_template_id JOIN assets_application_version v ON v.id=s.application_version_id WHERE s.id=? AND d.enabled=TRUE ORDER BY d.id`, task.ServiceID)
		if err != nil {
			return nil, "", err
		}
		defer rows.Close()
		for rows.Next() {
			var target runTarget
			if err = rows.Scan(&target.DeploymentID, &target.InstanceName, &target.HostID, &target.HostName, &target.HostIP, &target.AgentID, &target.AgentOnline, &target.AppHome, &target.RunUser, &target.WorkDirectory, &target.Version, &target.ServiceName); err != nil {
				return nil, "", err
			}
			target.Name = target.InstanceName
			targets = append(targets, target)
		}
		if err = rows.Err(); err != nil {
			return nil, "", err
		}
	}
	if len(targets) == 0 {
		return nil, "逻辑服务没有启用的部署实例", nil
	}
	if task.Scope == "service_once" {
		selected := -1
		for index := range targets {
			if targets[index].AgentOnline && targets[index].AgentID != "" {
				selected = index
				break
			}
		}
		if selected < 0 {
			selected = 0
		}
		targets = []runTarget{targets[selected]}
		targets[0].Name = fmt.Sprintf("%s (%s)", task.ServiceName, targets[0].InstanceName)
	}
	return targets, "", nil
}

func (handler *Handler) createExecution(ctx context.Context, task runTask, targets []runTarget, userID int32, username string) (int64, error) {
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	taskSnapshot := jsonBytes(gin.H{"id": task.ID, "name": task.Name, "target_type": targetType(task.Scope), "concurrency": task.Concurrency, "timeout_seconds": task.Timeout})
	groupSnapshot := jsonBytes(gin.H{"id": task.GroupID, "name": task.GroupName, "scope": task.Scope, "checks": task.Checks})
	serviceSnapshot := gin.H{"target_type": targetType(task.Scope), "name": task.ServiceName}
	if task.Scope == "per_host" {
		serviceSnapshot["name"] = fmt.Sprintf("%d 台主机", len(targets))
		serviceSnapshot["selected_host_ids"] = task.SelectedHostIDs
		serviceSnapshot["host_count"] = len(targets)
	}
	targetSnapshot := make([]gin.H, 0, len(targets))
	for _, target := range targets {
		targetSnapshot = append(targetSnapshot, gin.H{"deployment_id": target.DeploymentID, "host_id": target.HostID, "host_name": target.HostName, "instance_name": target.InstanceName, "host_ip": target.HostIP, "agent_id": target.AgentID, "agent_online": target.AgentOnline})
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO inspection_execution(task_id,status,trigger_type,task_snapshot,group_snapshot,service_snapshot,target_snapshot,summary,requested_user_id,requested_username,start_time,end_time,create_time,update_time) VALUES(?,'pending','manual',?,?,?,?,JSON_OBJECT(),?,?,NULL,NULL,NOW(),NOW())`, task.ID, taskSnapshot, groupSnapshot, jsonBytes(serviceSnapshot), jsonBytes(targetSnapshot), userID, username)
	if err != nil {
		return 0, err
	}
	executionID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	for index := range targets {
		var hostID any
		if task.Scope == "per_host" {
			hostID = targets[index].HostID
		}
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO inspection_target_execution(execution_id,deployment_id,host_id,target_name,host_id_snapshot,host_ip_snapshot,agent_id_snapshot,status,passed,error_message,raw_result,start_time,end_time,create_time,update_time) VALUES(?,?,?,?,?,?,?,'pending',NULL,'',JSON_OBJECT(),NULL,NULL,NOW(),NOW())`, executionID, nullablePositive(targets[index].DeploymentID), hostID, targets[index].Name, targets[index].HostID, targets[index].HostIP, targets[index].AgentID)
		if insertErr != nil {
			return 0, insertErr
		}
		targets[index].ID, err = result.LastInsertId()
		if err != nil {
			return 0, err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE inspection_task SET last_run_time=NOW(),update_time=NOW() WHERE id=?`, task.ID); err != nil {
		return 0, err
	}
	return executionID, tx.Commit()
}

func (handler *Handler) execute(executionID int64, task runTask, targets []runTarget) {
	if result, _ := handler.db.Exec(`UPDATE inspection_execution SET status='running',start_time=NOW(),update_time=NOW() WHERE id=? AND status='pending'`, executionID); rowsAffected(result) == 0 {
		return
	}
	limit := task.Concurrency
	if limit < 1 {
		limit = 1
	}
	if limit > 100 {
		limit = 100
	}
	semaphore, group := make(chan struct{}, limit), sync.WaitGroup{}
	for _, target := range targets {
		group.Add(1)
		go func(target runTarget) {
			defer group.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			handler.executeTarget(executionID, task, target)
		}(target)
	}
	group.Wait()
	var failed, success, canceled, warnings int
	handler.db.QueryRow(`SELECT SUM(status='failed'),SUM(status='success'),SUM(status='canceled') FROM inspection_target_execution WHERE execution_id=?`, executionID).Scan(&failed, &success, &canceled)
	handler.db.QueryRow(`SELECT COUNT(*) FROM inspection_result r JOIN inspection_target_execution t ON t.id=r.target_id WHERE t.execution_id=? AND r.severity='warning' AND r.status NOT IN ('pass','skipped')`, executionID).Scan(&warnings)
	status := "success"
	if failed > 0 || success == 0 {
		status = "failed"
	}
	var current string
	handler.db.QueryRow(`SELECT status FROM inspection_execution WHERE id=?`, executionID).Scan(&current)
	if current == "canceled" {
		status = "canceled"
	}
	summary := jsonBytes(gin.H{"total": len(targets), "success": success, "failed": failed, "canceled": canceled, "warning": warnings})
	handler.db.Exec(`UPDATE inspection_execution SET status=?,summary=?,end_time=NOW(),update_time=NOW() WHERE id=?`, status, summary, executionID)
}

func targetType(scope string) string {
	if scope == "per_host" {
		return "host_group"
	}
	return "logical_service"
}
func jsonBytes(value any) []byte { raw, _ := json.Marshal(value); return raw }
func nullablePositive(value int64) any {
	if value > 0 {
		return value
	}
	return nil
}
func rowsAffected(result sql.Result) int64 {
	if result == nil {
		return 0
	}
	count, _ := result.RowsAffected()
	return count
}

func (handler *Handler) executeTarget(executionID int64, task runTask, target runTarget) {
	var executionStatus string
	if handler.db.QueryRow(`SELECT status FROM inspection_execution WHERE id=?`, executionID).Scan(&executionStatus) != nil || executionStatus == "canceled" {
		return
	}
	handler.db.Exec(`UPDATE inspection_target_execution SET status='running',start_time=NOW(),update_time=NOW() WHERE id=?`, target.ID)
	results := make([]checkResult, 0, len(task.Checks))
	agentChecks := make([]gin.H, 0)
	for index, check := range task.Checks {
		agentChecks = append(agentChecks, compileAgentCheck(task, target, check, index, executionID))
	}
	errorMessage := ""
	if len(agentChecks) > 0 {
		if target.AgentID == "" || !handler.gateway.IsOnline(target.AgentID) {
			results = append(results, checkResult{Key: "check_plan", Type: "plan", Name: "Agent 检查计划", Status: "error", Severity: "critical", Message: "Agent 离线，未执行 Agent 检查"})
			errorMessage = "Agent 离线，未执行 Agent 检查"
		} else {
			// 巡检中心模式：只下发检查计划，基线类的应用控制状态/端口/路径/日志内置检查已移除。
			params := jsonBytes(gin.H{"check_plan": gin.H{"schema_version": 1, "checks": agentChecks}})
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(task.Timeout+45)*time.Second)
			agentResponse, err := handler.gateway.Execute(ctx, target.AgentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("inspection-%d-%d", executionID, target.ID), Type: "custom", Action: "check_application_baseline", ParamsJson: string(params), TimeoutSeconds: int32(task.Timeout)})
			cancel()
			if err != nil {
				errorMessage = err.Error()
				results = append(results, checkResult{Key: "check_plan", Type: "plan", Name: "Agent 检查计划", Status: "error", Severity: "critical", Message: errorMessage})
			} else {
				results = append(results, decodeAgentResults(agentResponse.ResultDataJson, task.Checks)...)
				errorMessage = agentResponse.ErrorMessage
			}
		}
	}
	var current string
	handler.db.QueryRow(`SELECT status FROM inspection_execution WHERE id=?`, executionID).Scan(&current)
	if current == "canceled" {
		handler.db.Exec(`UPDATE inspection_target_execution SET status='canceled',end_time=NOW(),update_time=NOW() WHERE id=?`, target.ID)
		return
	}
	failed := false
	for _, item := range results {
		if item.Status != "pass" && item.Status != "skipped" && item.Severity == "critical" {
			failed = true
		}
		handler.db.Exec(`INSERT INTO inspection_result(target_id,check_key,check_type,name,status,severity,expected_value,actual_value,message,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?,NOW(),NOW())`, target.ID, item.Key, item.Type, item.Name, item.Status, item.Severity, nullableJSON(item.Expected), nullableJSON(item.Actual), item.Message)
	}
	status := "success"
	if failed || len(results) == 0 {
		status = "failed"
	}
	handler.db.Exec(`UPDATE inspection_target_execution SET status=?,passed=?,error_message=?,raw_result=?,end_time=NOW(),update_time=NOW() WHERE id=?`, status, !failed, errorMessage, jsonBytes(gin.H{"passed": !failed, "checks": results}), target.ID)
}

func compileAgentCheck(task runTask, target runTarget, check runCheck, index int, executionID int64) gin.H {	config := check.Config
	executor := check.Executor
	compiled := gin.H{"key": fmt.Sprintf("inspection:%d:%d", executionID, index), "type": executor, "executor": executor, "name": check.Name, "requires_running": false}
	resolve := func(value any) string { return resolveVariables(fmt.Sprint(value), target) }
	switch executor {
	case "schema_validate":
		compiled["path"] = resolve(config["path"])
		compiled["document_type"] = config["document_type"]
		compiled["schema"] = gin.H{"type": config["schema_type"], "content": config["schema_content"]}
	case "goss":
		compiled["spec"] = resolve(config["spec"])
		compiled["run_user"] = first(resolve(config["run_user"]), target.RunUser, "root")
		if vars, ok := config["vars"]; ok {
			compiled["vars"] = vars
		}
		if environment, ok := config["environment"]; ok {
			compiled["environment"] = environment
		}
	}
	return compiled
}

func resolveVariables(value string, target runTarget) string {
	for key, replacement := range map[string]string{"${APP_HOME}": target.AppHome, "${RUN_USER}": target.RunUser, "${INSTANCE_NAME}": target.InstanceName, "${APPLICATION_VERSION}": target.Version, "${HOST_IP}": target.HostIP, "${HOST_NAME}": target.HostName, "${SERVICE_NAME}": target.ServiceName} {
		value = strings.ReplaceAll(value, key, replacement)
	}
	return value
}

func decodeAgentResults(raw string, checks []runCheck) []checkResult {
	var data struct {
		Checks []struct {
			Key, Type, Name, Status, Message string
			Expected, Actual                 any
		} `json:"checks"`
	}
	if json.Unmarshal([]byte(raw), &data) != nil {
		return []checkResult{{Key: "check_plan", Type: "plan", Name: "Agent 检查计划", Status: "error", Severity: "critical", Message: "Agent 返回结果格式无效"}}
	}
	results := make([]checkResult, 0, len(data.Checks))
	for _, item := range data.Checks {
		severity := "critical"
		parts := strings.Split(item.Key, ":")
		if len(parts) == 3 {
			var index int
			if _, err := fmt.Sscanf(parts[2], "%d", &index); err == nil && index < len(checks) {
				severity = first(checks[index].Severity, "critical")
			}
		}
		results = append(results, checkResult{Key: item.Key, Type: item.Type, Name: item.Name, Status: item.Status, Severity: severity, Expected: item.Expected, Actual: item.Actual, Message: item.Message})
	}
	return results
}

func nullableJSON(value any) any {
	if value == nil {
		return nil
	}
	return jsonBytes(value)
}
func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" && value != "<nil>" {
			return value
		}
	}
	return ""
}
