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
	ID             int64
	Name           string
	Concurrency    int
	Timeout        int
	Bindings       []mountBinding
	Enabled        bool
	MountSummaries []gin.H
}

type runGroup struct {
	ID       int64
	Name     string
	Category string
	Checks   []runCheck
}

type runCheck struct {
	Name     string         `json:"name"`
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
	// Macros 是逻辑服务实例化宏（assets_application_service.macro_values，
	// 如 ORACLE_SID），随部署实例注入检查计划变量展开。
	Macros map[string]string
	// CheckSets 是本目标的检查计划分片：[通用组(主机级), 应用组×各逻辑服务实例]。
	CheckSets []checkSet
	// ServicesSnapshot 记录本主机涉及的逻辑服务实例（快照用）。
	ServicesSnapshot []gin.H
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
	targets, err := handler.resolveMounts(ctx, task.Bindings)
	if err != nil {
		return 0, "", err
	}
	if len(targets) == 0 {
		return 0, "巡检挂载点没有解析到任何目标", nil
	}
	task.MountSummaries = make([]gin.H, 0, len(task.Bindings))
	for _, binding := range task.Bindings {
		task.MountSummaries = append(task.MountSummaries, binding.mountSummary())
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
	err := handler.db.QueryRowContext(ctx, `SELECT t.id,t.name,t.concurrency,t.timeout_seconds,t.enabled FROM inspection_task t WHERE t.id=?`, id).Scan(&task.ID, &task.Name, &task.Concurrency, &task.Timeout, &task.Enabled)
	if err == sql.ErrNoRows {
		return task, "巡检任务不存在", nil
	}
	if err != nil {
		return task, "", err
	}
	if !task.Enabled {
		return task, "巡检任务已禁用", nil
	}
	bindingRows, err := db.New(handler.db).ListInspectionTaskBindings(ctx, task.ID)
	if err != nil {
		return task, "", err
	}
	if len(bindingRows) == 0 {
		return task, "巡检任务没有绑定巡检组", nil
	}
	queries := db.New(handler.db)
	task.Bindings = make([]mountBinding, 0, len(bindingRows))
	totalChecks := 0
	for _, row := range bindingRows {
		binding := mountBindingFromRow(row)
		if !binding.Enabled {
			return task, "巡检组已禁用: " + binding.Name, nil
		}
		checkRows, checkErr := queries.ListEnabledInspectionChecksForRun(ctx, binding.ID)
		if checkErr != nil {
			return task, "", checkErr
		}
		binding.Checks = make([]runCheck, 0, len(checkRows))
		for _, checkRow := range checkRows {
			var config map[string]any
			if err = json.Unmarshal(checkRow.Config, &config); err != nil {
				return task, "巡检检查项配置数据无效", nil
			}
			if config == nil {
				config = map[string]any{}
			}
			binding.Checks = append(binding.Checks, runCheck{Name: checkRow.Name, Config: config, Severity: checkRow.Severity, Order: checkRow.Order})
		}
		totalChecks += len(binding.Checks)
		task.Bindings = append(task.Bindings, binding)
	}
	if totalChecks == 0 {
		return task, "巡检组没有启用的检查项", nil
	}
	return task, "", nil
}

// groupsFromBindings 提取绑定里的组（快照用）。
func groupsFromBindings(bindings []mountBinding) []runGroup {
	groups := make([]runGroup, 0, len(bindings))
	for _, binding := range bindings {
		groups = append(groups, binding.runGroup)
	}
	return groups
}

func (handler *Handler) createExecution(ctx context.Context, task runTask, targets []runTarget, triggerType string, userID int32, username string) (int64, error) {
	businessByHostForTargets := map[int64]gin.H{}
	tx, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	targetName := handler.mountTargetName(ctx, task.Bindings)
	taskSnapshot := jsonBytes(gin.H{"id": task.ID, "name": task.Name, "concurrency": task.Concurrency, "timeout_seconds": task.Timeout, "groups": groupsFromBindings(task.Bindings)})
	groupSnapshots := make([]gin.H, 0, len(task.Bindings))
	for _, binding := range task.Bindings {
		entry := gin.H{"id": binding.ID, "name": binding.Name, "category": binding.Category, "checks": binding.Checks, "mount": binding.mountSummary()}
		groupSnapshots = append(groupSnapshots, entry)
	}
	groupSnapshot := jsonBytes(groupSnapshots)
	// 业务链路快照：按目标主机的部署归属解析（纯冗余，失败不阻断执行）。
	serviceSnapshot := gin.H{"name": targetName}
	if len(targets) > 0 {
		hostIDs := make([]int64, 0, len(targets))
		for _, target := range targets {
			hostIDs = append(hostIDs, target.HostID)
		}
		if rows, err := db.New(handler.db).ListHostBusinessChains(ctx, hostIDs); err == nil {
			for _, row := range rows {
				businessByHostForTargets[row.HostID] = businessChainSnapshot(row.ProjectID, row.BusinessSystemID, row.EnvironmentID, row.ProjectName, "", row.BusinessSystemName, row.BusinessSystemOwner, row.EnvironmentName)
			}
		}
	}
	if len(task.MountSummaries) > 0 {
		serviceSnapshot["mounts"] = task.MountSummaries
	}
	targetSnapshot := make([]gin.H, 0, len(targets))
	for _, target := range targets {
		item := gin.H{"deployment_id": target.DeploymentID, "host_id": target.HostID, "host_name": target.HostName, "instance_name": target.InstanceName, "host_ip": target.HostIP, "agent_id": target.AgentID, "agent_online": target.AgentOnline}
		if business, ok := businessByHostForTargets[target.HostID]; ok {
			item["business"] = business
		}
		if len(target.ServicesSnapshot) > 0 {
			item["services"] = target.ServicesSnapshot
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
		result, insertErr := tx.ExecContext(ctx, `INSERT INTO inspection_target_execution(execution_id,deployment_id,host_id,target_name,host_id_snapshot,host_ip_snapshot,agent_id_snapshot,status,passed,error_message,raw_result,start_time,end_time,create_time,update_time) VALUES(?,?,?,?,?,?,?,'pending',NULL,'',JSON_OBJECT(),NULL,NULL,NOW(),NOW())`, executionID, nullablePositive(targets[index].DeploymentID), targets[index].HostID, targets[index].Name, targets[index].HostID, targets[index].HostIP, targets[index].AgentID)
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

// mountTargetName 把首个挂载点绑定转成人类可读的目标名，如
// "逻辑服务 artemis"、"业务 cdm @ test"、"项目 kul @ test"；查不到名称时回退 ID。
// mountTargetName 把首个挂载点绑定转成带完整业务链路的目标名，如
// "项目 kul · 业务 cdm @ test · 逻辑服务 artemis"；查不到名称时回退 ID。
func (handler *Handler) mountTargetName(ctx context.Context, bindings []mountBinding) string {
	if len(bindings) == 0 {
		return ""
	}
	binding := bindings[0]
	switch binding.MountType {
	case mountService:
		if !binding.ServiceID.Valid {
			return ""
		}
		var serviceName string
		if err := handler.db.QueryRowContext(ctx, `SELECT name FROM assets_application_service WHERE id=?`, binding.ServiceID.Int64).Scan(&serviceName); err != nil || serviceName == "" {
			return fmt.Sprintf("逻辑服务 #%d", binding.ServiceID.Int64)
		}
		chain, err := db.New(handler.db).GetInspectionServiceBusinessChain(ctx, binding.ServiceID.Int64)
		if err != nil {
			return "逻辑服务 " + serviceName
		}
		name := "逻辑服务 " + serviceName
		if prefix := chainPrefix(chain.ProjectName, chain.BusinessSystemName, chain.EnvironmentName); prefix != "" {
			name = prefix + " · " + name
		}
		return name
	case mountBusiness:
		if !binding.BusinessSystemID.Valid {
			return ""
		}
		var businessName, projectName string
		err := handler.db.QueryRowContext(ctx, `SELECT b.name, COALESCE(p.name,'') FROM assets_business_system b LEFT JOIN assets_project p ON p.id=b.project_id WHERE b.id=?`, binding.BusinessSystemID.Int64).Scan(&businessName, &projectName)
		if err != nil || businessName == "" {
			return fmt.Sprintf("业务 #%d", binding.BusinessSystemID.Int64)
		}
		text := "业务 " + businessName
		if binding.EnvironmentID.Valid {
			var environmentName string
			_ = handler.db.QueryRowContext(ctx, `SELECT name FROM assets_business_environment WHERE id=?`, binding.EnvironmentID.Int64).Scan(&environmentName)
			if environmentName != "" {
				text += " @ " + environmentName
			}
		}
		if projectName != "" {
			text = "项目 " + projectName + " · " + text
		}
		return text
	case mountProject, mountEnv:
		if !binding.ProjectID.Valid {
			return ""
		}
		projectName := ""
		_ = handler.db.QueryRowContext(ctx, `SELECT name FROM assets_project WHERE id=?`, binding.ProjectID.Int64).Scan(&projectName)
		if projectName == "" {
			return fmt.Sprintf("项目 #%d", binding.ProjectID.Int64)
		}
		if binding.MountType == mountEnv && binding.EnvironmentID.Valid {
			var environmentName string
			_ = handler.db.QueryRowContext(ctx, `SELECT name FROM assets_business_environment WHERE id=?`, binding.EnvironmentID.Int64).Scan(&environmentName)
			if environmentName != "" {
				return fmt.Sprintf("项目 %s @ %s", projectName, environmentName)
			}
		}
		return "项目 " + projectName
	}
	return ""
}

// chainPrefix 组合 "项目 X · 业务 Y @ 环境 Z" 前缀，空层级跳过。
func chainPrefix(projectName, businessName, environmentName string) string {
	parts := make([]string, 0, 3)
	if projectName != "" {
		parts = append(parts, "项目 "+projectName)
	}
	if businessName != "" {
		text := "业务 " + businessName
		if environmentName != "" {
			text += " @ " + environmentName
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " · ")
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
	results := make([]checkResult, 0)
	agentChecks, checks, missingParams := buildCheckPlan(task, target, executionID)
	errorMessage := ""
	if len(missingParams) > 0 {
		// 缺参属于配置错误：目标失败并写明缺什么，而不是拿错误路径静默执行。
		message := "巡检参数缺失: " + strings.Join(missingParams, "、")
		results = append(results, checkResult{Key: "check_plan", Type: "plan", Name: "检查参数", Status: "error", Severity: "critical", Message: message})
		errorMessage = message
	}
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

func compileAgentCheck(context runTarget, check runCheck, index int, executionID int64, params map[string]string) gin.H {
	config := check.Config
	// 唯一执行器 OPA：check_plan 不再携带 executor 字段（Agent 侧恒按 opa 处理）。
	compiled := gin.H{"key": fmt.Sprintf("inspection:%d:%d", executionID, index), "name": check.Name, "requires_running": false}
	resolve := func(value any) string { return resolveVariables(fmt.Sprint(value), context, params) }
	compiled["config"] = gin.H{
		"input_commands": resolveOpaInputs(config["input_commands"], resolve),
		"input_files":    resolveOpaInputs(config["input_files"], resolve),
		"policy":         config["policy"], // Rego 是代码不是路径，不做变量展开
	}
	compiled["run_user"] = first(resolve(config["run_user"]), context.RunUser, "root")
	// 主机上下文走独立字段（input 信封的 host），不混入 policy。
	compiled["host_ip"] = context.HostIP
	compiled["host_name"] = context.HostName
	if vars, ok := config["vars"]; ok {
		compiled["vars"] = vars
	}
	return compiled
}

// resolveOpaInputs 展开采集条目 exec/path 里的 ${变量}（HOST_IP、APP_HOME 等），
// 其余字段原样透传。
func resolveOpaInputs(raw any, resolve func(any) string) []any {
	items, ok := raw.([]any)
	if !ok {
		return []any{}
	}
	resolved := make([]any, 0, len(items))
	for _, item := range items {
		entry, valid := item.(map[string]any)
		if !valid {
			continue
		}
		clone := gin.H{}
		for key, value := range entry {
			if key == "exec" || key == "path" {
				clone[key] = resolve(value)
			} else {
				clone[key] = value
			}
		}
		resolved = append(resolved, clone)
	}
	return resolved
}

// standardVars 是部署实例内置的变量表（来自部署模板/资产，每个实例各异）。
func standardVars(target runTarget) map[string]string {
	return map[string]string{
		"APP_HOME":            target.AppHome,
		"RUN_USER":            target.RunUser,
		"INSTANCE_NAME":       target.InstanceName,
		"APPLICATION_VERSION": target.Version,
		"HOST_IP":             target.HostIP,
		"HOST_NAME":           target.HostName,
		"SERVICE_NAME":        target.ServiceName,
	}
}

// resolveVariables 展开 ${变量}：检查参数（任务绑定时赋值的实参）优先，
// 其次内置变量（部署模板/资产上下文）。两者都没有的变量保持字面量。
func resolveVariables(value string, target runTarget, params map[string]string) string {
	for key, replacement := range params {
		value = strings.ReplaceAll(value, "${"+key+"}", replacement)
	}
	for key, replacement := range standardVars(target) {
		value = strings.ReplaceAll(value, "${"+key+"}", replacement)
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
