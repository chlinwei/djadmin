package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"autoadmin/internal/agent"
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
	ID, HostID, DeploymentID int64
	Name, HostName, HostIP   string
	AgentOnline              bool
	// HostInstanceName 是主机 instance_name（= assets_host.instance_name），
	// 也是 gRPC 网关会话的路由 key；与逻辑服务的 InstanceName 不是一回事。
	HostInstanceName                   string
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
	current, err := db.New(handler.db).GetInspectionTaskRunState(ctx, id)
	if err == sql.ErrNoRows {
		return task, "巡检任务不存在", nil
	}
	if err != nil {
		return task, "", err
	}
	task.ID, task.Name = current.ID, current.Name
	task.Concurrency, task.Timeout, task.Enabled = int(current.Concurrency), int(current.TimeoutSeconds), current.Enabled
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
		item := gin.H{"deployment_id": target.DeploymentID, "host_id": target.HostID, "host_name": target.HostName, "instance_name": target.InstanceName, "host_ip": target.HostIP, "agent_online": target.AgentOnline}
		if business, ok := businessByHostForTargets[target.HostID]; ok {
			item["business"] = business
		}
		if len(target.ServicesSnapshot) > 0 {
			item["services"] = target.ServicesSnapshot
		}
		targetSnapshot = append(targetSnapshot, item)
	}
	queries := db.New(tx)
	now := time.Now().UTC()
	executionID, err := queries.CreateInspectionExecution(ctx, db.CreateInspectionExecutionParams{
		TaskID:            sql.NullInt64{Int64: task.ID, Valid: true},
		TriggerType:       triggerType,
		TaskSnapshot:      taskSnapshot,
		GroupSnapshot:     groupSnapshot,
		ServiceSnapshot:   jsonBytes(serviceSnapshot),
		TargetSnapshot:    jsonBytes(targetSnapshot),
		RequestedUserID:   sql.NullInt32{Int32: userID, Valid: true},
		RequestedUsername: username,
		CreateTime:        now,
		UpdateTime:        now,
	})
	if err != nil {
		return 0, err
	}
	for index := range targets {
		targetID, insertErr := queries.CreateInspectionTargetExecution(ctx, db.CreateInspectionTargetExecutionParams{
			ExecutionID:          executionID,
			DeploymentID:         nullablePositiveID(targets[index].DeploymentID),
			HostID:               sql.NullInt64{Int64: targets[index].HostID, Valid: true},
			TargetName:           targets[index].Name,
			HostIDSnapshot:       sql.NullInt32{Int32: int32(targets[index].HostID), Valid: true},
			HostIpSnapshot:       targets[index].HostIP,
			InstanceNameSnapshot: targets[index].HostInstanceName,
			CreateTime:           now,
			UpdateTime:           now,
		})
		if insertErr != nil {
			return 0, insertErr
		}
		targets[index].ID = targetID
	}
	if err = queries.TouchInspectionTaskLastRun(ctx, db.TouchInspectionTaskLastRunParams{
		LastRunTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: task.ID,
	}); err != nil {
		return 0, err
	}
	return executionID, tx.Commit()
}

func (handler *Handler) execute(executionID int64, task runTask, targets []runTarget) {
	ctx, queries := context.Background(), db.New(handler.db)
	now := time.Now().UTC()
	claimed, claimErr := queries.MarkInspectionExecutionRunning(ctx, db.MarkInspectionExecutionRunningParams{
		StartTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: executionID,
	})
	if claimErr != nil || claimed == 0 {
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
	// 统计失败不影响执行记录收尾（与迁移前一致：查询出错时各计数保持 0）。
	outcomes, _ := queries.CountInspectionTargetOutcomes(ctx, executionID)
	warnings, _ := queries.CountInspectionWarningResults(ctx, executionID)
	status := "success"
	switch {
	case outcomes.Failed > 0:
		status = "failed"
	case outcomes.Success == 0 && outcomes.Canceled > 0:
		status = "canceled"
	case outcomes.Success == 0 && outcomes.Skipped > 0:
		// 没有任何目标真正执行（全部离线跳过）时不算失败。
		status = "skipped"
	case outcomes.Success == 0:
		status = "failed"
	}
	if handler.isCanceled(executionID) {
		status = "canceled"
	}
	summary := jsonBytes(gin.H{"total": len(targets), "success": outcomes.Success, "failed": outcomes.Failed,
		"canceled": outcomes.Canceled, "skipped": outcomes.Skipped, "warning": warnings})
	now = time.Now().UTC()
	_, _ = queries.FinishInspectionExecution(ctx, db.FinishInspectionExecutionParams{
		Status: status, Summary: summary, EndTime: sql.NullTime{Time: now, Valid: true},
		UpdateTime: now, ID: executionID,
	})
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
	queries := db.New(handler.db)
	switch binding.MountType {
	case mountService:
		if !binding.ServiceID.Valid {
			return ""
		}
		serviceName, err := queries.GetApplicationServiceName(ctx, binding.ServiceID.Int64)
		if err != nil || serviceName == "" {
			return fmt.Sprintf("逻辑服务 #%d", binding.ServiceID.Int64)
		}
		chain, err := queries.GetInspectionServiceBusinessChain(ctx, binding.ServiceID.Int64)
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
		business, err := queries.GetBusinessSystem(ctx, binding.BusinessSystemID.Int64)
		if err != nil || business.Name == "" {
			return fmt.Sprintf("业务 #%d", binding.BusinessSystemID.Int64)
		}
		text := "业务 " + business.Name
		if binding.EnvironmentID.Valid {
			if environmentName, envErr := queries.GetBusinessEnvironmentNameByID(ctx, binding.EnvironmentID.Int64); envErr == nil && environmentName != "" {
				text += " @ " + environmentName
			}
		}
		if business.ProjectName != "" {
			text = "项目 " + business.ProjectName + " · " + text
		}
		return text
	case mountProject, mountEnv:
		if !binding.ProjectID.Valid {
			return ""
		}
		projectName, _ := queries.GetProjectNameByID(ctx, binding.ProjectID.Int64)
		if projectName == "" {
			return fmt.Sprintf("项目 #%d", binding.ProjectID.Int64)
		}
		if binding.MountType == mountEnv && binding.EnvironmentID.Valid {
			if environmentName, envErr := queries.GetBusinessEnvironmentNameByID(ctx, binding.EnvironmentID.Int64); envErr == nil && environmentName != "" {
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

// nullablePositiveID 把非正的 ID 当成"没有值"写 NULL。
func nullablePositiveID(value int64) sql.NullInt64 {
	if value > 0 {
		return sql.NullInt64{Int64: value, Valid: true}
	}
	return sql.NullInt64{}
}

func (handler *Handler) executeTarget(executionID int64, task runTask, target runTarget) {
	if handler.isCanceled(executionID) {
		return
	}
	ctx, queries := context.Background(), db.New(handler.db)
	now := func() time.Time { return time.Now().UTC() }
	_ = queries.MarkInspectionTargetRunning(ctx, db.MarkInspectionTargetRunningParams{
		StartTime: sql.NullTime{Time: now(), Valid: true}, UpdateTime: now(), ID: target.ID,
	})
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
		if target.HostInstanceName == "" || !handler.gateway.IsOnline(target.HostInstanceName) {
			// 离线是"未执行"而非"检查失败"：目标置 skipped、不产生检查结果，
			// 避免常态离线的机器污染失败统计。
			_ = queries.SkipInspectionTarget(ctx, db.SkipInspectionTargetParams{
				ErrorMessage: "Agent 离线，未执行巡检", EndTime: sql.NullTime{Time: now(), Valid: true},
				UpdateTime: now(), ID: target.ID,
			})
			return
		}
		// 巡检中心模式：只下发检查计划，基线类的应用控制状态/端口/路径/日志内置检查已移除。
		params := jsonBytes(gin.H{"check_plan": gin.H{"schema_version": 1, "checks": agentChecks}})
		requestCtx, cancel := context.WithTimeout(ctx, time.Duration(task.Timeout+45)*time.Second)
		agentResponse, err := handler.gateway.Execute(requestCtx, target.HostInstanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("inspection-%d-%d", executionID, target.ID), Type: "custom", Action: "check_application_baseline", ParamsJson: string(params), TimeoutSeconds: int32(task.Timeout)})
		cancel()
		if err != nil {
			// 与前置在线检查一致：下发瞬间掉线按"未执行"跳过，不计为检查失败。
			if errors.Is(err, agent.ErrAgentOffline) {
				_ = queries.SkipInspectionTarget(ctx, db.SkipInspectionTargetParams{
					ErrorMessage: "Agent 离线，未执行巡检", EndTime: sql.NullTime{Time: now(), Valid: true},
					UpdateTime: now(), ID: target.ID,
				})
				return
			}
			errorMessage = err.Error()
			results = append(results, checkResult{Key: "check_plan", Type: "plan", Name: "Agent 检查计划", Status: "error", Severity: "critical", Message: errorMessage})
		} else {
			results = append(results, decodeAgentResults(agentResponse.ResultDataJson, checks)...)
			errorMessage = agentResponse.ErrorMessage
		}
	}
	if handler.isCanceled(executionID) {
		_ = queries.CancelInspectionTarget(ctx, db.CancelInspectionTargetParams{
			EndTime: sql.NullTime{Time: now(), Valid: true}, UpdateTime: now(), ID: target.ID,
		})
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
	if insertErr := handler.insertResults(ctx, target.ID, results); insertErr != nil {
		errorMessage = strings.TrimSpace(errorMessage + "；巡检结果写入失败: " + insertErr.Error())
		status = "failed"
	}
	passed := status == "success"
	_ = queries.FinishInspectionTarget(ctx, db.FinishInspectionTargetParams{
		Status: status, Passed: &passed, ErrorMessage: errorMessage,
		RawResult: jsonBytes(gin.H{"passed": passed, "checks": results}),
		EndTime:   sql.NullTime{Time: now(), Valid: true}, UpdateTime: now(), ID: target.ID,
	})
}

// insertResults 逐条落检查结果。
//
// 原实现按 100 行/批拼一条多行 INSERT：占位符个数随入参变化、且用 `?` 占位符——是 sqlc
// 表达不了、PG 变体（pgx）也跑不通的形状。改为逐条 sqlc INSERT：结果落在"每主机一次远端
// 检查"之后，条数由检查项数量决定（几十到几百），摊在以远端命令为主的耗时里可以忽略；
// 真要回到批量插入就按方言各写一份。与 baseline 的 flushResults 同一取舍（SQL_DESIGN §6.3）。
func (handler *Handler) insertResults(ctx context.Context, targetID int64, results []checkResult) error {
	queries := db.New(handler.db)
	now := time.Now().UTC()
	for _, item := range results {
		if err := queries.CreateInspectionResult(ctx, db.CreateInspectionResultParams{
			TargetID: targetID, CheckKey: item.Key, CheckType: item.Type, Name: item.Name, Status: item.Status,
			Severity: item.Severity, GroupID: nullablePositiveID(item.GroupID), GroupName: item.Group,
			ExpectedValue: nullableJSON(item.Expected), ActualValue: nullableJSON(item.Actual),
			Message: item.Message, CreateTime: now, UpdateTime: now,
		}); err != nil {
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

func nullableJSON(value any) json.RawMessage {
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
