package logcollect

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	generated "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 批量作业的 HTTP 入口（计划 LOG_COLLECTION_LIFECYCLE §8 Phase 2）。
//
// 与旧「批量下发 / 批量重新安装」的区别：那些接口在**请求内**逐台跑完才返回，1000 台时
// 必然超时；现在这两个动作只做三件事——写作业与明细、投队列消息、返回作业号，
// 之后由执行器在后台按有界并发推进，前端轮询作业进度。
//
// 批量启停仍是同步接口（单台只是一次 30 秒的 systemctl 调用），删除也仍是同步的批量删除。

// CreateLogBatchJob POST /monitor/log-targets/batch-jobs/
//
// body：{"action":"apply"|"install","ids":[...]}。
// ids 可省略：省略且 action=apply 时表示"全部待下发的主机"（一键应用 N 台待下发），
// 由服务端按配置态实时算（§2.4：筛选/统计场景允许一次全量渲染）。
func (handler *Handler) CreateLogBatchJob(context *gin.Context) {
	var input struct {
		Action string  `json:"action"`
		IDs    []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	action := strings.TrimSpace(input.Action)
	if action != LogBatchActionApply && action != LogBatchActionInstall {
		response.BusinessError(context, 400, "action must be apply|install", nil)
		return
	}
	targetIDs := input.IDs
	if len(targetIDs) == 0 {
		if action != LogBatchActionApply {
			response.BusinessError(context, 400, "ids must be a non-empty array", nil)
			return
		}
		pending, err := handler.pendingApplyTargetIDs(context)
		if err != nil {
			response.BusinessError(context, 400, err.Error(), nil)
			return
		}
		targetIDs = pending
	}
	result, err := handler.createLogBatchJob(context, action, targetIDs, requestedUsername(context))
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, result)
}

// GetLogBatchJob GET /monitor/log-targets/batch-jobs/:id/
//
// 作业详情 = 头表进度 + 每台主机的明细（失败原因逐台可见）。
func (handler *Handler) GetLogBatchJob(context *gin.Context) {
	id := parseID(context.Param("id"))
	queries := generated.New(handler.db)
	row, err := queries.GetLogBatchJob(context, id)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "log batch job not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := queries.ListLogBatchJobItems(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	detail := make([]map[string]any, 0, len(items))
	for _, item := range items {
		detail = append(detail, logBatchItemDTO(item))
	}
	result := logBatchJobDTO(row)
	result["items"] = detail
	response.Success(context, result)
}

// GetActiveLogBatchJob GET /monitor/log-targets/batch-jobs/active/?action=apply
//
// 页面上"还在跑的批量作业"：刷新页面/切换 tab 回来后仍能接着看进度。
// **没有进行中的作业时返回 data=null，而不是 404**：这是页面每次进入都会调的"查询可选状态"，
// 用 404 表达"没有"会让前端弹一个无意义的错误提示（拦截器把所有非 200 业务码都当失败提示）。
func (handler *Handler) GetActiveLogBatchJob(context *gin.Context) {
	action := strings.TrimSpace(context.Query("action"))
	if action != LogBatchActionApply && action != LogBatchActionInstall {
		response.BusinessError(context, 400, "action must be apply|install", nil)
		return
	}
	row, err := generated.New(handler.db).GetActiveLogBatchJobByAction(context, action)
	if err == sql.ErrNoRows {
		response.Success(context, nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, logBatchJobTaskDTO(row))
}

// PendingConfigSummary GET /monitor/log-targets/pending-summary/
//
// "N 台主机待下发"的全量口径。与列表的 config_state 筛选同一算法，但一次算完全部纳管目标
// （查询次数与主机数无关，渲染是纯函数），因此 500–1000 台也是一次批量渲染。
func (handler *Handler) PendingConfigSummary(context *gin.Context) {
	targets, err := generated.New(handler.db).ListManagedLogTargetConfigs(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	refs := make([]LogConfigTargetRef, 0, len(targets))
	for _, target := range targets {
		refs = append(refs, LogConfigTargetRef{HostID: target.HostID, AppliedFingerprint: target.ConfigFingerprint})
	}
	counts := map[string]int{LogConfigSynced: 0, LogConfigDrift: 0, LogConfigNever: 0, LogConfigUnknown: 0}
	states, stateErr := handler.EvaluateLogConfigStates(context, refs)
	for _, ref := range refs {
		state, ok := states[ref.HostID]
		if !ok {
			counts[LogConfigUnknown]++
			continue
		}
		counts[state.Status]++
	}
	result := gin.H{
		"total": len(refs), "synced": counts[LogConfigSynced], "drift": counts[LogConfigDrift],
		"never": counts[LogConfigNever], "unknown": counts[LogConfigUnknown],
		"pending": counts[LogConfigDrift] + counts[LogConfigNever],
	}
	// 算不出期望配置（如没有启用的默认集群）时不谎报"0 台待下发"：回传原因，让前端提示。
	if stateErr != nil {
		result["error"] = stateErr.Error()
	}
	response.Success(context, result)
}

// pendingApplyTargetIDs 算出"配置待下发（从未下发或已变更）"的全部目标 id。
// 算不出期望配置的主机（unknown）不纳入：无法判断差异，也就无法负责地下发。
func (handler *Handler) pendingApplyTargetIDs(context context.Context) ([]int64, error) {
	targets, err := generated.New(handler.db).ListManagedLogTargetConfigs(context)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("没有已纳管的日志采集目标")
	}
	refs := make([]LogConfigTargetRef, 0, len(targets))
	for _, target := range targets {
		refs = append(refs, LogConfigTargetRef{HostID: target.HostID, AppliedFingerprint: target.ConfigFingerprint})
	}
	states, err := handler.EvaluateLogConfigStates(context, refs)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(targets))
	for _, target := range targets {
		state, ok := states[target.HostID]
		if !ok {
			continue
		}
		if state.Status == LogConfigDrift || state.Status == LogConfigNever {
			ids = append(ids, target.ID)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("没有待下发的日志采集目标")
	}
	return ids, nil
}

// createLogBatchJob 建作业与明细并入队。任一目标状态不满足就整体失败（避免"部分入队"的中间态）。
func (handler *Handler) createLogBatchJob(context context.Context, action string, targetIDs []int64, username string) (gin.H, error) {
	if handler.batchRunner == nil {
		return nil, fmt.Errorf("批量作业执行器未装配（缺少队列发布者），无法下发")
	}
	queries := generated.New(handler.db)
	targets, err := queries.ListLogBatchTargets(context, targetIDs)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("没有匹配的日志采集目标")
	}
	now := time.Now().UTC()
	jobID, err := queries.CreateLogBatchJob(context, generated.CreateLogBatchJobParams{
		CreateTime: now, UpdateTime: now, Action: action, TotalCount: int32(len(targets)),
		Concurrency:       int32(handler.batchRunner.concurrency(action)),
		RequestedUsername: username,
	})
	if err != nil {
		return nil, err
	}
	for _, target := range targets {
		// 主机名落成快照：主机改名或被删后，进度页仍要如实展示当时的目标。
		if err = queries.CreateLogBatchJobItem(context, generated.CreateLogBatchJobItemParams{
			CreateTime: now, UpdateTime: now, BatchJobID: jobID,
			TargetID: target.ID, HostID: target.HostID,
			HostName: firstNonEmpty(target.InstanceName, target.Ip), HostIp: target.Ip,
		}); err != nil {
			return nil, err
		}
	}
	// 入队放在最后：作业行与明细都在了，消息才会被投出。
	if err = ScheduleLogBatch(context, handler.batchRunner.publisher, jobID); err != nil {
		_, _ = queries.FinishLogBatchJob(context, generated.FinishLogBatchJobParams{
			Status:     LogBatchStatusFailed,
			Message:    fmt.Sprintf("入队失败: %v", err),
			FinishedAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: jobID,
		})
		return nil, fmt.Errorf("批量作业入队失败: %w", err)
	}
	row, err := queries.GetLogBatchJob(context, jobID)
	if err != nil {
		return nil, err
	}
	return logBatchJobDTO(row), nil
}

// requestedUsername 取当前操作者名（作业留档用；未认证上下文返回空串）。
func requestedUsername(context *gin.Context) string {
	claims, ok := identity.ClaimsFromContext(context)
	if !ok {
		return ""
	}
	return claims.Username
}
