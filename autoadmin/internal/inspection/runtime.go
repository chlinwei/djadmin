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
	ID, ServiceID            int64
	Name, Scope, ServiceName string
	Concurrency, Timeout     int
	Groups                   []runGroup
	SelectedHostIDs          []int64
	Enabled                  bool
}

type runGroup struct {
	ID       int64
	Name     string
	Scope    string
	Category string
	Checks   []runCheck
}

type runCheck struct {
	Name     string         `json:"name"`
	Executor string         `json:"executor"`
	Config   map[string]any `json:"config"`
	Severity string         `json:"severity"`
	Order    uint32         `json:"order"`
	GroupID  int64          `json:"group_id"`
	Group    string         `json:"group"`
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
	GroupID                                    int64
	Group                                      string
}

// flattenChecks 把多组的检查项按组顺序摊平成一份检查计划；结果按
// inspection:{execution}:{index} 回查时用同一个摊平序列还原组归属。
func flattenChecks(groups []runGroup) []runCheck {
	checks := make([]runCheck, 0)
	for _, group := range groups {
		for _, check := range group.Checks {
			check.GroupID, check.Group = group.ID, group.Name
			checks = append(checks, check)
		}
	}
	return checks
}

func (handler *Handler) RunTask(context *gin.Context) {
	claims, _ := identity.ClaimsFromContext(context)
	userID, username := int32(0), ""
	if claims != nil {
		userID, username = claims.UserID, claims.Username
	}
	executionID, message, err := handler.startRun(context, parseID(context.Param("id")), "manual", userID, username)
	if err != nil {
		response.Error(context, err)
		return
	}
	if message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	response.Success(context, gin.H{"execution_id": executionID, "status": "pending"})
}

// startRun prepares and dispatches one execution. message is a user-facing
// business rejection (empty when the run was accepted); err is an infrastructure failure.
func (handler *Handler) startRun(ctx context.Context, taskID int64, triggerType string, userID int32, username string) (int64, string, error) {
	task, message, err := handler.prepareRunTask(ctx, taskID)
	if err != nil || message != "" {
		return 0, message, err
	}
	targets, message, err := handler.resolveTargets(ctx, task)
	if err != nil || message != "" {
		return 0, message, err
	}
	executionID, err := handler.createExecution(ctx, task, targets, triggerType, userID, username)
	if err != nil {
		return 0, "", err
	}
	go handler.execute(executionID, task, targets)
	return executionID, "", nil
}

func (handler *Handler) prepareRunTask(ctx context.Context, id int64) (runTask, string, error) {
	var task runTask
	var selected []byte
	err := handler.db.QueryRowContext(ctx, `SELECT t.id,t.name,COALESCE(t.logical_service_id,0),t.selected_host_ids,t.concurrency,t.timeout_seconds,t.enabled,COALESCE(s.name,'') FROM inspection_task t LEFT JOIN assets_application_service s ON s.id=t.logical_service_id WHERE t.id=?`, id).Scan(&task.ID, &task.Name, &task.ServiceID, &selected, &task.Concurrency, &task.Timeout, &task.Enabled, &task.ServiceName)
	if err == sql.ErrNoRows {
		return task, "巡检任务不存在", nil
	}
	if err != nil {
		return task, "", err
	}
	if !task.Enabled {
		return task, "巡检任务已禁用", nil
	}
	if err = json.Unmarshal(selected, &task.SelectedHostIDs); err != nil {
		return task, "巡检任务的主机范围数据无效", nil
	}
	groupIDs, err := db.New(handler.db).ListInspectionTaskGroupIDs(ctx, task.ID)
	if err != nil {
		return task, "", err
	}
	if len(groupIDs) == 0 {
		return task, "巡检任务没有绑定巡检组", nil
	}
	queries := db.New(handler.db)
	task.Groups = make([]runGroup, 0, len(groupIDs))
	totalChecks := 0
	for _, groupID := range groupIDs {
		groupRow, groupErr := queries.GetInspectionGroup(ctx, groupID)
		if groupErr != nil {
			return task, "", groupErr
		}
		if !groupRow.Enabled {
			return task, "巡检组已禁用: " + groupRow.Name, nil
		}
		checkRows, checkErr := queries.ListEnabledInspectionChecksForRun(ctx, groupID)
		if checkErr != nil {
			return task, "", checkErr
		}
		group := runGroup{ID: groupRow.ID, Name: groupRow.Name, Scope: groupRow.Scope, Category: groupRow.Category, Checks: make([]runCheck, 0, len(checkRows))}
		for _, row := range checkRows {
			var config map[string]any
			if err = json.Unmarshal(row.Config, &config); err != nil {
				return task, "巡检检查项配置数据无效", nil
			}
			if config == nil {
				config = map[string]any{}
			}
			group.Checks = append(group.Checks, runCheck{Name: row.Name, Executor: row.Executor, Config: config, Severity: row.Severity, Order: row.Order})
		}
		totalChecks += len(group.Checks)
		task.Groups = append(task.Groups, group)
	}
	if len(task.Groups) == 0 {
		return task, "巡检任务没有可用的巡检组", nil
	}
	// 保存任务时已校验所有组 scope 一致，这里取第一个作为目标解析依据。
	task.Scope = task.Groups[0].Scope
	if totalChecks == 0 {
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

func (handler *Handler) createExecution(ctx context.Context, task runTask, targets []runTarget, triggerType string, userID int32, username string) (int64, error) {
	businessByHostForTargets := map[int64]gin.H{}
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	taskSnapshot := jsonBytes(gin.H{"id": task.ID, "name": task.Name, "target_type": targetType(task.Scope), "concurrency": task.Concurrency, "timeout_seconds": task.Timeout, "groups": task.Groups})
	groupSnapshots := make([]gin.H, 0, len(task.Groups))
	for _, group := range task.Groups {
		groupSnapshots = append(groupSnapshots, gin.H{"id": group.ID, "name": group.Name, "scope": group.Scope, "category": group.Category, "checks": group.Checks})
	}
	groupSnapshot := jsonBytes(groupSnapshots)
	serviceSnapshot := gin.H{"target_type": targetType(task.Scope), "name": task.ServiceName}
	// 业务链路快照：汇总报表按项目/业务系统/环境聚合的地基（纯冗余，失败不阻断执行）。
	var businessChain gin.H
	if task.Scope != "per_host" && task.ServiceID > 0 {
		if row, err := db.New(handler.db).GetInspectionServiceBusinessChain(ctx, task.ServiceID); err == nil {
			businessChain = businessChainSnapshot(row.ProjectID, row.BusinessSystemID, row.EnvironmentID, row.ProjectName, row.ProjectOwner, row.BusinessSystemName, row.BusinessSystemOwner, row.EnvironmentName)
		}
	} else if task.Scope == "per_host" && len(targets) > 0 {
		hostIDs := make([]int64, 0, len(targets))
		for _, target := range targets {
			hostIDs = append(hostIDs, target.HostID)
		}
		if rows, err := db.New(handler.db).ListHostBusinessChains(ctx, hostIDs); err == nil {
			businessByHost := make(map[int64]gin.H, len(rows))
			for _, row := range rows {
				businessByHost[row.HostID] = businessChainSnapshot(row.ProjectID, row.BusinessSystemID, row.EnvironmentID, row.ProjectName, "", row.BusinessSystemName, row.BusinessSystemOwner, row.EnvironmentName)
			}
			businessByHostForTargets = businessByHost
		}
	}
	if businessChain != nil {
		serviceSnapshot["project"], serviceSnapshot["business_system"], serviceSnapshot["environment"] = businessChain["project"], businessChain["business_system"], businessChain["environment"]
	}
	if task.Scope == "per_host" {
		serviceSnapshot["name"] = fmt.Sprintf("%d 台主机", len(targets))
		serviceSnapshot["selected_host_ids"] = task.SelectedHostIDs
		serviceSnapshot["host_count"] = len(targets)
	}
	targetSnapshot := make([]gin.H, 0, len(targets))
	for _, target := range targets {
		item := gin.H{"deployment_id": target.DeploymentID, "host_id": target.HostID, "host_name": target.HostName, "instance_name": target.InstanceName, "host_ip": target.HostIP, "agent_id": target.AgentID, "agent_online": target.AgentOnline}
		if businessChain != nil {
			item["business"] = businessChain
		} else if business, ok := businessByHostForTargets[target.HostID]; ok {
			item["business"] = business
		}
		targetSnapshot = append(targetSnapshot, item)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO inspection_execution(task_id,status,trigger_type,task_snapshot,group_snapshot,service_snapshot,target_snapshot,summary,requested_user_id,requested_username,start_time,end_time,create_time,update_time) VALUES(?,'pending',?,?,?,?,?,JSON_OBJECT(),?,?,NULL,NULL,NOW(),NOW())`, task.ID, triggerType, taskSnapshot, groupSnapshot, jsonBytes(serviceSnapshot), jsonBytes(targetSnapshot), userID, username)
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
			handler.globalSlots <- struct{}{}
			defer func() { <-handler.globalSlots }()
			handler.executeTarget(executionID, task, target)
		}(target)
	}
	group.Wait()
	var failed, success, canceled, skippedCount, warnings int
	handler.db.QueryRow(`SELECT SUM(status='failed'),SUM(status='success'),SUM(status='canceled'),SUM(status='skipped') FROM inspection_target_execution WHERE execution_id=?`, executionID).Scan(&failed, &success, &canceled, &skippedCount)
	handler.db.QueryRow(`SELECT COUNT(*) FROM inspection_result r JOIN inspection_target_execution t ON t.id=r.target_id WHERE t.execution_id=? AND r.severity='warning' AND r.status NOT IN ('pass','skipped')`, executionID).Scan(&warnings)
	status := "success"
	switch {
	case failed > 0:
		status = "failed"
	case success == 0 && canceled > 0:
		status = "canceled"
	case success == 0 && skippedCount > 0:
		// 没有任何目标真正执行（全部离线跳过）时不算失败。
		status = "skipped"
	case success == 0:
		status = "failed"
	}
	if handler.isCanceled(executionID) {
		status = "canceled"
	}
	summary := jsonBytes(gin.H{"total": len(targets), "success": success, "failed": failed, "canceled": canceled, "skipped": skippedCount, "warning": warnings})
	handler.db.Exec(`UPDATE inspection_execution SET status=?,summary=?,end_time=NOW(),update_time=NOW() WHERE id=?`, status, summary, executionID)
}

func targetType(scope string) string {
	if scope == "per_host" {
		return "host_group"
	}
	return "logical_service"
}

// businessChainSnapshot 把 项目/业务系统/环境 链路整理进快照；ID 为空的层级省略。
func businessChainSnapshot(projectID, businessSystemID, environmentID sql.NullInt64, projectName, projectOwner, businessSystemName, businessSystemOwner, environmentName string) gin.H {
	chain := gin.H{}
	if projectID.Valid {
		chain["project"] = gin.H{"id": projectID.Int64, "name": projectName, "owner": projectOwner}
	}
	if businessSystemID.Valid {
		chain["business_system"] = gin.H{"id": businessSystemID.Int64, "name": businessSystemName, "owner": businessSystemOwner}
	}
	if environmentID.Valid {
		chain["environment"] = gin.H{"id": environmentID.Int64, "name": environmentName}
	}
	if len(chain) == 0 {
		return nil
	}
	return chain
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
	if handler.isCanceled(executionID) {
		return
	}
	handler.db.Exec(`UPDATE inspection_target_execution SET status='running',start_time=NOW(),update_time=NOW() WHERE id=?`, target.ID)
	checks := flattenChecks(task.Groups)
	results := make([]checkResult, 0, len(checks))
	agentChecks := make([]gin.H, 0)
	for index, check := range checks {
		agentChecks = append(agentChecks, compileAgentCheck(task, target, check, index, executionID))
	}
	errorMessage := ""
	if len(agentChecks) > 0 {
		if target.AgentID == "" || !handler.gateway.IsOnline(target.AgentID) {
			// 离线是"未执行"而非"检查失败"：目标置 skipped、不产生检查结果，
			// 避免常态离线的机器污染失败统计。
			handler.db.Exec(`UPDATE inspection_target_execution SET status='skipped',passed=FALSE,error_message='Agent 离线，未执行巡检',end_time=NOW(),update_time=NOW() WHERE id=?`, target.ID)
			return
		}
		// 巡检中心模式：只下发检查计划，基线类的应用控制状态/端口/路径/日志内置检查已移除。
		params := jsonBytes(gin.H{"check_plan": gin.H{"schema_version": 1, "checks": agentChecks}})
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(task.Timeout+45)*time.Second)
		agentResponse, err := handler.gateway.Execute(ctx, target.AgentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("inspection-%d-%d", executionID, target.ID), Type: "custom", Action: "check_application_baseline", ParamsJson: string(params), TimeoutSeconds: int32(task.Timeout)})
		cancel()
		if err != nil {
			errorMessage = err.Error()
			results = append(results, checkResult{Key: "check_plan", Type: "plan", Name: "Agent 检查计划", Status: "error", Severity: "critical", Message: errorMessage})
		} else {
			results = append(results, decodeAgentResults(agentResponse.ResultDataJson, checks)...)
			errorMessage = agentResponse.ErrorMessage
		}
	}
	if handler.isCanceled(executionID) {
		handler.db.Exec(`UPDATE inspection_target_execution SET status='canceled',end_time=NOW(),update_time=NOW() WHERE id=?`, target.ID)
		return
	}
	failed := false
	for _, item := range results {
		if item.Status != "pass" && item.Status != "skipped" && item.Severity == "critical" {
			failed = true
		}
	}
	status := "success"
	if failed || len(results) == 0 {
		status = "failed"
	}
	if insertErr := handler.insertResults(context.Background(), target.ID, results); insertErr != nil {
		errorMessage = strings.TrimSpace(errorMessage + "；巡检结果写入失败: " + insertErr.Error())
		status = "failed"
	}
	handler.db.Exec(`UPDATE inspection_target_execution SET status=?,passed=?,error_message=?,raw_result=?,end_time=NOW(),update_time=NOW() WHERE id=?`, status, status == "success", errorMessage, jsonBytes(gin.H{"passed": status == "success", "checks": results}), target.ID)
}

// insertResults writes check results in multi-row batches instead of one INSERT
// per row; a 500-target × 20-check execution produces ~10k rows and row-by-row
// commits were the tail-latency bottleneck.
func (handler *Handler) insertResults(ctx context.Context, targetID int64, results []checkResult) error {
	const batchSize = 100
	statement := `INSERT INTO inspection_result(target_id,check_key,check_type,name,status,severity,group_id,group_name,expected_value,actual_value,message,create_time,update_time) VALUES `
	for start := 0; start < len(results); start += batchSize {
		end := min(start+batchSize, len(results))
		var builder strings.Builder
		builder.WriteString(statement)
		arguments := make([]any, 0, (end-start)*11)
		for index := start; index < end; index++ {
			if index > start {
				builder.WriteString(",")
			}
			builder.WriteString("(?,?,?,?,?,?,?,?,?,?,?,NOW(),NOW())")
			item := results[index]
			arguments = append(arguments, targetID, item.Key, item.Type, item.Name, item.Status, item.Severity, nullablePositive(item.GroupID), item.Group, nullableJSON(item.Expected), nullableJSON(item.Actual), item.Message)
		}
		if _, err := handler.db.ExecContext(ctx, builder.String(), arguments...); err != nil {
			return err
		}
	}
	return nil
}

func compileAgentCheck(task runTask, target runTarget, check runCheck, index int, executionID int64) gin.H {
	config := check.Config
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
		var groupID int64
		var groupName string
		parts := strings.Split(item.Key, ":")
		if len(parts) == 3 {
			var index int
			if _, err := fmt.Sscanf(parts[2], "%d", &index); err == nil && index < len(checks) {
				severity = first(checks[index].Severity, "critical")
				groupID, groupName = checks[index].GroupID, checks[index].Group
			}
		}
		results = append(results, checkResult{Key: item.Key, Type: item.Type, Name: item.Name, Status: item.Status, Severity: severity, Expected: item.Expected, Actual: item.Actual, Message: item.Message, GroupID: groupID, Group: groupName})
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
