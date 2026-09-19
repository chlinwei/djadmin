package logcollect

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 服务维度的下发（架构文档 §8「服务级下发」）。
//
// 为什么"服务级下发"只能是入口与聚合、底层仍是主机级：agent 侧 `apply_filebeat_config` 的语义是
// **"本次交付的这套文件就是该主机 inputs.d 的全量，没交付的 .yml 一律删除"**
// （dj_agent/internal/executor/builtin_actions.go）。所以"只把某个服务的片段推下去"会把同一主机上
// 其他服务的配置删掉——下发的最小完整单位只能是主机。服务维度做的是：解析出承载该服务的主机、
// 逐个全量重下发、并把各主机的配置态聚合给用户看。
//
// 幂等且廉价：每台主机是否需要真下发由渲染指纹决定（一致则跳过 agent 调用，见 applyLogTargetConfigRow），
// 所以"把服务的主机都下发一遍"不必先筛待下发。

// serviceApplyTarget 一台承载该服务的主机及其采集目标（未纳管时 TargetID 为 0）。
type serviceApplyTarget struct {
	HostID            int64  `json:"host_id"`
	HostIP            string `json:"host_ip"`
	HostInstanceName  string `json:"host_instance_name"`
	TargetID          int64  `json:"target_id"`
	ConfigFingerprint string `json:"-"`
	// ConfigState 仅对已纳管主机有效（未纳管主机的配置态不适用）。
	ConfigState string `json:"config_state,omitempty"`
	// Managed 是否已纳管日志采集：false = 没有可下发的目标，下发给不了它。
	Managed bool `json:"managed"`
	// AgentOnline 该主机的 agent 会话是否在线（**实时**来自网关，不是库里的字段）。
	// 它是"为什么没日志"的第一层：agent 不在线，查状态/启停/下发都做不了。
	AgentOnline bool `json:"agent_online"`
	// RuntimeStatus Filebeat 进程态（running/stopped/error），**落库快照**：
	// 只有安装/启停/查状态动作会写它，展示方要负责先刷新（见 log_service_chain.go）。
	RuntimeStatus string `json:"runtime_status,omitempty"`
}

// groupServiceApplyTargets 把解析结果分成"已纳管（可下发）"与"未纳管（下发不了）"两组，各按主机 id 排序。
//
// 单独拎成纯函数是为了能直接测：这里的判据（`target_id IS NULL` = 未纳管）是"少下发几台却不吭声"
// 这类问题的唯一防线。
func groupServiceApplyTargets(rows []db.ListServiceLogApplyTargetsRow) (managed, unmanaged []serviceApplyTarget) {
	managed, unmanaged = []serviceApplyTarget{}, []serviceApplyTarget{}
	for _, row := range rows {
		target := serviceApplyTarget{
			HostID: row.HostID, HostIP: row.HostIp, HostInstanceName: row.HostInstanceName,
			Managed: row.TargetID.Valid, RuntimeStatus: row.RuntimeStatus,
		}
		if row.TargetID.Valid {
			target.TargetID = row.TargetID.Int64
			target.ConfigFingerprint = row.ConfigFingerprint
			managed = append(managed, target)
			continue
		}
		unmanaged = append(unmanaged, target)
	}
	sort.Slice(managed, func(i, j int) bool { return managed[i].HostID < managed[j].HostID })
	sort.Slice(unmanaged, func(i, j int) bool { return unmanaged[i].HostID < unmanaged[j].HostID })
	return managed, unmanaged
}

// buildServiceHostStates 读库 + 评估配置态，返回"承载该服务的主机"及其聚合。
//
// 抽出来是因为有两个消费方都要这份数据：日志中心的"本服务下发状态"与"采集链路"诊断。
// 各写一遍会让两处对"几台待下发"给出不同数字——这种不一致比不显示更糟。
func (handler *Handler) buildServiceHostStates(context context.Context, serviceID int64) ([]serviceApplyTarget, []serviceApplyTarget, serviceConfigStateSummary, error) {
	managed, unmanaged, err := handler.resolveServiceApplyTargets(context, serviceID)
	if err != nil {
		return nil, nil, serviceConfigStateSummary{}, err
	}
	// agent 在线是**实时**事实（网关会话），与库里的配置态无关，所以在这里统一补上。
	// 网关未接线（单测/无数据面部署）时一律 false，不谎报在线。
	for index := range managed {
		managed[index].AgentOnline = handler.agentOnline(managed[index].HostInstanceName)
	}
	for index := range unmanaged {
		unmanaged[index].AgentOnline = handler.agentOnline(unmanaged[index].HostInstanceName)
	}
	summary := serviceConfigStateSummary{Hosts: len(managed) + len(unmanaged), Managed: len(managed), Unmanaged: len(unmanaged)}
	if len(managed) == 0 {
		return managed, unmanaged, summary, nil
	}
	refs := make([]LogConfigTargetRef, 0, len(managed))
	for _, target := range managed {
		refs = append(refs, LogConfigTargetRef{HostID: target.HostID, AppliedFingerprint: target.ConfigFingerprint})
	}
	states, err := handler.EvaluateLogConfigStates(context, refs)
	if err != nil {
		return nil, nil, summary, err
	}
	for index := range managed {
		state, ok := states[managed[index].HostID]
		if !ok {
			// 评估函数对算不出期望配置的主机也会给条目；缺条目说明它被判成"不适用"，
			// 这里如实标 unknown，不谎报"已同步"。
			managed[index].ConfigState = LogConfigUnknown
			summary.Unknown++
			continue
		}
		managed[index].ConfigState = state.Status
		switch state.Status {
		case LogConfigSynced:
			summary.Synced++
		case LogConfigDrift:
			summary.Drift++
		case LogConfigNever:
			summary.Never++
		default:
			summary.Unknown++
		}
	}
	return managed, unmanaged, summary, nil
}

// agentOnline 问网关这台主机的 agent 会话在不在。
func (handler *Handler) agentOnline(hostInstanceName string) bool {
	if handler.gateway == nil || strings.TrimSpace(hostInstanceName) == "" {
		return false
	}
	return handler.gateway.IsOnline(hostInstanceName)
}

// resolveServiceApplyTargets 读库并分组：服务承载在哪些主机上、其中哪些主机能下发。
func (handler *Handler) resolveServiceApplyTargets(context context.Context, serviceID int64) ([]serviceApplyTarget, []serviceApplyTarget, error) {
	rows, err := db.New(handler.db).ListServiceLogApplyTargets(context, serviceID)
	if err != nil {
		return nil, nil, err
	}
	managed, unmanaged := groupServiceApplyTargets(rows)
	return managed, unmanaged, nil
}

// serviceConfigStateSummary 服务级配置态聚合。
//
// 只能聚合、没有"服务级指纹"：config_fingerprint 是主机级的，而同一个服务在不同主机上因实例级
// runtime_variables 不同，渲染结果本就不同。造一个服务级指纹只会误导。
type serviceConfigStateSummary struct {
	Hosts     int `json:"hosts"`
	Managed   int `json:"managed"`
	Unmanaged int `json:"unmanaged"`
	Synced    int `json:"synced"`
	Drift     int `json:"drift"`
	Never     int `json:"never"`
	Unknown   int `json:"unknown"`
}

// GetServiceLogConfigState GET /monitor/log-targets/service-config-state/?application_service_id=
//
// 日志中心页的"本服务下发状态"：承载主机清单（含未纳管的）+ 配置态聚合。
func (handler *Handler) GetServiceLogConfigState(context *gin.Context) {
	serviceID := parseID(context.Query("application_service_id"))
	if serviceID < 1 {
		response.BusinessError(context, 400, "application_service_id is required", nil)
		return
	}
	managed, unmanaged, summary, err := handler.buildServiceHostStates(context, serviceID)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, gin.H{"summary": summary, "hosts": managed, "unmanaged_hosts": unmanaged})
}

// ApplyLogTargetsForService POST /monitor/log-targets/service-apply/
//
// body: {"application_service_id": <必填>}
//
// 语义：对**承载该服务的全部已纳管主机**逐个做一次全量重下发（一次批量作业，进度可查）。
// 不带 service 启用态过滤：停用服务后也要能下发一次来移除主机上的旧片段（见查询里的说明）。
func (handler *Handler) ApplyLogTargetsForService(context *gin.Context) {
	var input struct {
		ApplicationServiceID int64 `json:"application_service_id"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	if input.ApplicationServiceID < 1 {
		response.BusinessError(context, 400, "application_service_id is required", nil)
		return
	}
	managed, unmanaged, err := handler.resolveServiceApplyTargets(context, input.ApplicationServiceID)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	if len(managed) == 0 {
		response.BusinessError(context, 400, unmanagedMessage(unmanaged), nil)
		return
	}
	targetIDs := make([]int64, 0, len(managed))
	for _, target := range managed {
		targetIDs = append(targetIDs, target.TargetID)
	}
	result, err := handler.createLogBatchJob(context, LogBatchActionApply, targetIDs, requestedUsername(context))
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, gin.H{
		"job": result, "target_total": len(targetIDs),
		// 下发不了的主机要显式返回：否则用户以为"整个服务都下发了"，实际少了几台。
		"unmanaged_hosts": unmanaged, "message": unmanagedMessage(unmanaged),
	})
}

// unmanagedMessage 未纳管主机的提示语（没有则返回空串，前端据此不展示）。
func unmanagedMessage(unmanaged []serviceApplyTarget) string {
	if len(unmanaged) == 0 {
		return ""
	}
	names := make([]string, 0, len(unmanaged))
	for _, target := range unmanaged {
		names = append(names, firstNonEmpty(target.HostInstanceName, target.HostIP, fmt.Sprintf("host-%d", target.HostID)))
	}
	return fmt.Sprintf("有 %d 台承载主机还没有纳管日志采集，本次不会下发：%s",
		len(unmanaged), strings.Join(names, "、"))
}
