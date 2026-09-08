package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 构想 5（组挂载点模型）：任务不再直接选目标，每个「巡检组 → 挂载点」绑定在执行时
// 动态解析出巡检对象：
//   - 通用组 @ 项目 / 环境（项目×环境）→ 主机集合（去重，主机上下文，禁用应用变量）
//   - 应用组 @ 业务（×环境）+ 实例模式（all 全部实例 | once 主实例/HA）→ 逻辑服务（部署实例）
//
// 目标 = 各挂载点解析结果的并集（按主机去重）；每台主机的检查计划 =
// 通用组检查项 ×1（主机级）+ 每个适用逻辑服务的应用组检查项 ×1（各自变量上下文）。

const (
	mountProject  = "project"
	mountEnv      = "environment"
	mountBusiness = "business"
	mountService  = "service"
	instanceAll   = "all"
	instanceOnce  = "once"
)

type mountBinding struct {
	runGroup
	Enabled          bool
	MountType        string
	ProjectID        sql.NullInt64
	EnvironmentID    sql.NullInt64
	BusinessSystemID sql.NullInt64
	ServiceID        sql.NullInt64
	Params           json.RawMessage
	ParamValues      json.RawMessage
	InstanceMode     string
	// 解析产物（快照用）
	SelectedInstance string
	TargetCount      int
	SkippedReason    string
}

// checkSet 是一个目标检查计划的分片：同上下文的一组检查项。
// Instance 为空表示主机级分片（通用组），否则为逻辑服务实例分片（应用组）。
type checkSet struct {
	Context  runTarget
	Instance string
	Groups   []runGroup
}

func mountBindingFromRow(row db.ListInspectionTaskBindingsRow) mountBinding {
	binding := mountBinding{
		runGroup:         runGroup{ID: row.GroupID, Name: row.GroupName, Category: row.Category},
		Enabled:          row.Enabled,
		MountType:        row.MountType,
		ProjectID:        row.ProjectID,
		EnvironmentID:    row.EnvironmentID,
		BusinessSystemID: row.BusinessSystemID,
		ServiceID:        row.ServiceID,
		Params:           row.Params,
		ParamValues:      row.ParamValues,
	}
	if row.InstanceMode.Valid {
		binding.InstanceMode = row.InstanceMode.String
	}
	return binding
}

func (binding mountBinding) mountSummary() gin.H {
	summary := gin.H{"group_id": binding.ID, "group_name": binding.Name, "category": binding.Category, "mount_type": binding.MountType}
	switch binding.MountType {
	case mountProject:
		summary["project_id"] = binding.ProjectID.Int64
	case mountEnv:
		summary["project_id"], summary["environment_id"] = binding.ProjectID.Int64, binding.EnvironmentID.Int64
	case mountBusiness:
		summary["business_system_id"], summary["instance_mode"] = binding.BusinessSystemID.Int64, binding.InstanceMode
	case mountService:
		summary["service_id"], summary["instance_mode"] = binding.ServiceID.Int64, binding.InstanceMode
	}
	if binding.SelectedInstance != "" {
		summary["selected_instance"] = binding.SelectedInstance
	}
	summary["targets"] = binding.TargetCount
	if binding.SkippedReason != "" {
		summary["skipped"] = binding.SkippedReason
	}
	return summary
}

// resolveMounts 把全部挂载点绑定解析成目标列表（按主机去重，ID 升序稳定排序）。
func (handler *Handler) resolveMounts(ctx context.Context, bindings []mountBinding) ([]runTarget, error) {
	queries := db.New(handler.db)
	targetsByHost := make(map[int64]*runTarget)
	hostSetGroups := make(map[int64]map[int64]runGroup) // hostID -> 通用组（去重）
	instanceSets := make(map[int64]*checkSet)           // deploymentID -> 实例分片
	instanceOrder := make([]int64, 0)

	ensureTarget := func(hostID int64, name, hostName, ip, agentID string, online bool) *runTarget {
		target, exists := targetsByHost[hostID]
		if !exists {
			target = &runTarget{HostID: hostID, Name: name, HostName: hostName, HostIP: ip, AgentID: agentID, AgentOnline: online}
			if target.Name == "" {
				target.Name = target.HostIP
			}
			targetsByHost[hostID] = target
		}
		return target
	}

	for index := range bindings {
		binding := &bindings[index]
		switch binding.MountType {
		case mountProject, mountEnv:
			if binding.MountType == mountProject {
				rows, err := queries.ListMountProjectHosts(ctx, binding.ProjectID)
				if err != nil {
					return nil, err
				}
				if len(rows) == 0 {
					binding.SkippedReason = "挂载点没有解析到主机"
					continue
				}
				for _, row := range rows {
					target := ensureTarget(row.ID, row.InstanceName, row.InstanceName, row.Ip, row.AgentID, row.AgentOnline)
					if hostSetGroups[row.ID] == nil {
						hostSetGroups[row.ID] = make(map[int64]runGroup)
					}
					hostSetGroups[row.ID][binding.ID] = binding.runGroup
					target.RunUser, target.WorkDirectory = "root", "/"
					binding.TargetCount++
				}
			} else {
				rows, err := queries.ListMountProjectEnvironmentHosts(ctx, db.ListMountProjectEnvironmentHostsParams{ProjectID: binding.ProjectID, EnvironmentID: binding.EnvironmentID})
				if err != nil {
					return nil, err
				}
				if len(rows) == 0 {
					binding.SkippedReason = "挂载点没有解析到主机"
					continue
				}
				for _, row := range rows {
					target := ensureTarget(row.ID, row.InstanceName, row.InstanceName, row.Ip, row.AgentID, row.AgentOnline)
					if hostSetGroups[row.ID] == nil {
						hostSetGroups[row.ID] = make(map[int64]runGroup)
					}
					hostSetGroups[row.ID][binding.ID] = binding.runGroup
					target.RunUser, target.WorkDirectory = "root", "/"
					binding.TargetCount++
				}
			}
		case mountBusiness, mountService:
			var instances []db.ListMountBusinessInstancesRow
			if binding.MountType == mountService {
				serviceRows, queryErr := queries.ListMountServiceInstances(ctx, binding.ServiceID.Int64)
				if queryErr != nil {
					return nil, queryErr
				}
				// 两个查询列集一致，统一到同一行类型处理。
				for _, row := range serviceRows {
					instances = append(instances, db.ListMountBusinessInstancesRow(row))
				}
			} else {
				rows, queryErr := queries.ListMountBusinessInstances(ctx, db.ListMountBusinessInstancesParams{
					BusinessSystemID: binding.BusinessSystemID.Int64,
					EnvironmentID:    binding.EnvironmentID,
				})
				if queryErr != nil {
					return nil, queryErr
				}
				instances = rows
			}
			rows := instances
			// 通用组挂业务/逻辑服务：解析为**主机去重**（主机上下文，禁应用变量）；
			// 应用组：解析为**部署实例**（每实例变量各自展开），once 只选一台在线实例。
			applicationGroup := binding.Category == "application"
			if applicationGroup && binding.InstanceMode == instanceOnce {
				// HA 场景：只选一台在线实例（优先 online，按 service/deployment 稳定排序）。
				selected := -1
				for i, row := range rows {
					if row.AgentOnline && row.AgentID != "" {
						selected = i
						break
					}
				}
				if selected < 0 && len(rows) > 0 {
					selected = 0
				}
				if selected >= 0 {
					binding.SelectedInstance = instanceDisplayName(rows[selected])
					rows = rows[selected : selected+1]
				}
			}
			if len(rows) == 0 {
				binding.SkippedReason = "挂载点没有解析到部署实例"
				continue
			}
			for _, row := range rows {
				target := ensureTarget(row.HostID, row.HostName, row.HostName, row.Ip, row.AgentID, row.AgentOnline)
				target.RunUser, target.WorkDirectory = "root", "/"
				if !applicationGroup {
					if hostSetGroups[row.HostID] == nil {
						hostSetGroups[row.HostID] = make(map[int64]runGroup)
					}
					hostSetGroups[row.HostID][binding.ID] = binding.runGroup
					binding.TargetCount++
					continue
				}
				set, exists := instanceSets[row.DeploymentID]
				if !exists {
					set = &checkSet{
						Context:  runTarget{HostID: row.HostID, HostName: row.HostName, HostIP: row.Ip, AgentID: row.AgentID, AgentOnline: row.AgentOnline, DeploymentID: row.DeploymentID, InstanceName: row.InstanceName, ServiceName: row.ServiceName, AppHome: row.AppHome, RunUser: row.RunUser, WorkDirectory: row.WorkDirectory, Version: row.Version, Name: instanceDisplayName(row), Macros: macrosFromRaw(row.MacroValues)},
						Instance: instanceDisplayName(row),
					}
					instanceSets[row.DeploymentID] = set
					instanceOrder = append(instanceOrder, row.DeploymentID)
					target.ServicesSnapshot = append(target.ServicesSnapshot, gin.H{"service_id": row.ServiceID, "service_name": row.ServiceName, "instance": set.Instance, "deployment_id": row.DeploymentID, "mode": binding.InstanceMode})
				}
				set.Groups = append(set.Groups, binding.runGroup)
				binding.TargetCount++
			}
		default:
			binding.SkippedReason = "未知挂载类型: " + binding.MountType
		}
	}

	hostIDs := make([]int64, 0, len(targetsByHost))
	for hostID := range targetsByHost {
		hostIDs = append(hostIDs, hostID)
	}
	sort.Slice(hostIDs, func(i, j int) bool { return hostIDs[i] < hostIDs[j] })
	sort.Slice(instanceOrder, func(i, j int) bool { return instanceOrder[i] < instanceOrder[j] })

	targets := make([]runTarget, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		target := targetsByHost[hostID]
		if groups := hostSetGroups[hostID]; len(groups) > 0 {
			set := checkSet{Context: *target}
			for _, groupID := range sortedGroupIDs(groups) {
				set.Groups = append(set.Groups, groups[groupID])
			}
			target.CheckSets = append(target.CheckSets, set)
		}
		for _, deploymentID := range instanceOrder {
			if set := instanceSets[deploymentID]; set.Context.HostID == hostID {
				target.CheckSets = append(target.CheckSets, *set)
			}
		}
		if len(target.CheckSets) == 0 {
			continue
		}
		targets = append(targets, *target)
	}
	if len(targets) == 0 {
		return nil, nil
	}
	return targets, nil
}

// macrosFromRaw 解析逻辑服务 macro_values（JSON 对象，值统一转字符串）。
// 资产侧宏 Key 有两种存法：裸名（ORACLE_SID）和模板渲染形式（${ORACLE_SID}），
// 读取时统一归一为裸名，供检查参数引用。
func macrosFromRaw(raw json.RawMessage) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var decoded map[string]any
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	macros := make(map[string]string, len(decoded))
	for key, value := range decoded {
		switch typed := value.(type) {
		case string:
			macros[normalizeMacroKey(key)] = typed
		default:
			macros[normalizeMacroKey(key)] = fmt.Sprint(typed)
		}
	}
	return macros
}

// normalizeMacroKey 剥掉 ${...} 包裹（如资产侧宏 Key 存成 ${ORACLE_SID}）。
func normalizeMacroKey(key string) string {
	if strings.HasPrefix(key, "${") && strings.HasSuffix(key, "}") {
		return key[2 : len(key)-1]
	}
	return key
}

func instanceDisplayName(row db.ListMountBusinessInstancesRow) string {
	if row.InstanceName != "" {
		return row.InstanceName
	}
	return row.ServiceName
}

func sortedGroupIDs(groups map[int64]runGroup) []int64 {
	ids := make([]int64, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// buildCheckPlan 把目标的检查分片组装成 Agent 检查计划。每个分片先按任务绑定
// 实参构造检查参数表（字面量直接取值；引用按该分片实例从内置变量/服务宏展开），
// 再以参数表展开各检查项。任何分片缺参 → 整个目标失败并返回缺失清单。
func buildCheckPlan(task runTask, target runTarget, executionID int64) ([]gin.H, []runCheck, []string) {
	agentChecks := make([]gin.H, 0)
	checks := make([]runCheck, 0)
	allMissing := make([]string, 0)
	for _, set := range target.CheckSets {
		// 参数解析必须用分片自己的实例上下文（内置变量/服务宏都在 Context 上）
		params, missing := resolveCheckParams(task.Bindings, set.Context)
		if len(missing) > 0 {
			allMissing = append(allMissing, missing...)
			continue
		}
		for _, group := range set.Groups {
			for _, check := range group.Checks {
				check.GroupID, check.Group = group.ID, group.Name
				compiled := compileAgentCheck(set.Context, check, len(agentChecks), executionID, params)
				if set.Instance != "" {
					if name, ok := compiled["name"].(string); ok {
						compiled["name"] = fmt.Sprintf("%s · %s", set.Instance, name)
					}
				}
				agentChecks = append(agentChecks, compiled)
				checks = append(checks, check)
			}
		}
	}
	if len(allMissing) > 0 {
		return nil, nil, allMissing
	}
	return agentChecks, checks, nil
}

// resolveCheckParams 计算检查参数最终值（单组模型：取第一个绑定的实参）。
// 字面量直接取值；引用先查内置变量、再查实例宏；都取不到记入 missing。
func resolveCheckParams(bindings []mountBinding, target runTarget) (map[string]string, []string) {
	params := make(map[string]string)
	if len(bindings) == 0 {
		return params, nil
	}
	binding := bindings[0]
	var declarations []struct {
		Name     string `json:"name"`
		Required bool   `json:"required"`
		Default  string `json:"default"`
	}
	if len(binding.Params) > 0 {
		_ = json.Unmarshal(binding.Params, &declarations)
	}
	assignments := map[string]paramAssignment{}
	if len(binding.ParamValues) > 0 {
		_ = json.Unmarshal(binding.ParamValues, &assignments)
	}
	standard := standardVars(target)
	missing := make([]string, 0)
	for _, declaration := range declarations {
		assignment, provided := assignments[declaration.Name]
		switch {
		case provided && assignment.Value != nil:
			params[declaration.Name] = *assignment.Value
		case provided && assignment.Ref != "":
			if value, exists := standard[assignment.Ref]; exists {
				params[declaration.Name] = value
			} else if value, exists := target.Macros[assignment.Ref]; exists {
				params[declaration.Name] = value
			} else {
				missing = append(missing, fmt.Sprintf("%s（引用 %s 在此实例不存在）", declaration.Name, assignment.Ref))
			}
		case declaration.Default != "":
			params[declaration.Name] = declaration.Default
		case declaration.Required:
			missing = append(missing, fmt.Sprintf("%s（必填，未赋值）", declaration.Name))
		}
	}
	return params, missing
}

// validateMountBinding 校验单个「组 → 挂载点」绑定的合法性（保存时调用）。
// 通用组只允许 项目/环境；应用组只允许 业务（×环境）+实例模式。
func (handler *Handler) validateMountBinding(ctx *gin.Context, binding *groupBindingInput, category string) (mountBinding, string, error) {
	resolved := mountBinding{MountType: binding.MountType}
	switch {
	case binding.MountType == mountProject || binding.MountType == mountEnv:
		if category != "general" {
			return resolved, "通用挂载点（项目/环境）只能绑定通用类型巡检组", nil
		}
		if binding.ProjectID == nil || *binding.ProjectID <= 0 {
			return resolved, "挂载到项目/环境时必须选择项目", nil
		}
		resolved.ProjectID = sql.NullInt64{Int64: *binding.ProjectID, Valid: true}
		if binding.MountType == mountEnv {
			if binding.EnvironmentID == nil || *binding.EnvironmentID <= 0 {
				return resolved, "挂载到环境时必须选择环境", nil
			}
			resolved.EnvironmentID = sql.NullInt64{Int64: *binding.EnvironmentID, Valid: true}
		}
	case binding.MountType == mountBusiness || binding.MountType == mountService:
		// 挂载点由树节点决定，与组类型无关：通用组挂业务/服务解析为主机去重，
		// 应用组解析为部署实例；instance_mode 仅对应用组有意义。
		if binding.MountType == mountBusiness {
			if binding.BusinessSystemID == nil || *binding.BusinessSystemID <= 0 {
				return resolved, "挂载到业务时必须选择业务系统", nil
			}
			resolved.BusinessSystemID = sql.NullInt64{Int64: *binding.BusinessSystemID, Valid: true}
		} else {
			if binding.ServiceID == nil || *binding.ServiceID <= 0 {
				return resolved, "挂载到逻辑服务时必须选择逻辑服务", nil
			}
			var count int
			if err := handler.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM assets_application_service WHERE id=?`, *binding.ServiceID).Scan(&count); err != nil {
				return resolved, "", err
			}
			if count == 0 {
				return resolved, "挂载的逻辑服务不存在", nil
			}
			resolved.ServiceID = sql.NullInt64{Int64: *binding.ServiceID, Valid: true}
		}
		if category == "application" {
			if binding.InstanceMode != instanceAll && binding.InstanceMode != instanceOnce {
				return resolved, "实例模式无效，仅支持全部实例（all）或主实例（once）", nil
			}
			resolved.InstanceMode = binding.InstanceMode
			resolved.Category = category
		}
	default:
		return resolved, "挂载类型无效", nil
	}
	return resolved, "", nil
}

// ---- 检查参数（形参/实参）----
//
// 巡检组用 params 声明形参（检查项里 ${name} 引用）；任务绑定用 param_values
// 赋实参：{"value":"85"} 固定值 | {"ref":"ORACLE_SID"} 引用（内置变量或服务宏，
// 按实例展开）。

type paramAssignment struct {
	Value *string `json:"value"`
	Ref   string  `json:"ref"`
}

// validateParamValues 校验实参 JSON 形态：每个值要么固定值要么引用名。
func validateParamValues(raw json.RawMessage, category string) string {
	if len(raw) == 0 {
		return ""
	}
	var assignments map[string]paramAssignment
	if err := json.Unmarshal(raw, &assignments); err != nil {
		return "巡检参数赋值格式无效"
	}
	for name, assignment := range assignments {
		if assignment.Value != nil && assignment.Ref != "" {
			return fmt.Sprintf("巡检参数 %s 不能同时设置固定值和引用", name)
		}
		if assignment.Value == nil && assignment.Ref == "" {
			return fmt.Sprintf("巡检参数 %s 缺少赋值（固定值或引用）", name)
		}
		if assignment.Ref != "" && category != "application" {
			return fmt.Sprintf("巡检参数 %s：只有应用类型巡检组才能引用变量（通用组的挂载没有实例上下文）", name)
		}
	}
	return ""
}

// missingRequiredParams 校验组声明的必填参数在任务绑定里都有赋值（任务保存时）。
func (handler *Handler) missingRequiredParams(ctx context.Context, groupID int64, paramValues json.RawMessage) string {
	declared, err := db.New(handler.db).GetInspectionGroup(ctx, groupID)
	if err != nil {
		return ""
	}
	var params []struct {
		Name     string `json:"name"`
		Required bool   `json:"required"`
	}
	if len(declared.Params) > 0 {
		if json.Unmarshal(declared.Params, &params) != nil {
			return ""
		}
	}
	assignments := map[string]paramAssignment{}
	if len(paramValues) > 0 {
		_ = json.Unmarshal(paramValues, &assignments)
	}
	missing := make([]string, 0)
	for _, param := range params {
		if !param.Required {
			continue
		}
		if assignment, exists := assignments[param.Name]; !exists || (assignment.Value == nil && assignment.Ref == "") {
			missing = append(missing, param.Name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "必填巡检参数未赋值: " + strings.Join(missing, "、")
}
