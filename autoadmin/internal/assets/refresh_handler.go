package assets

import (
	"context"
	"log/slog"
	"sync"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

type deploymentRuntimeOutcome struct {
	ID                  int64   `json:"id"`
	RuntimeStatus       string  `json:"runtime_status"`
	RuntimeStatusOutput string  `json:"runtime_status_output"`
	LastStatusCheckTime *string `json:"last_status_check_time"`
	Error               string  `json:"error,omitempty"`
}

// Each visible scope sends explicit deployment IDs, so shared instances are checked once
// without expanding a single-instance observation to its sibling deployments.
func (handler *Handler) RefreshApplicationDeploymentRuntimeStatus(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	ids, err := requiredIDListQuery(c, "ids")
	if err != nil {
		response.Error(c, ErrInvalid)
		return
	}
	results := collectDeploymentRuntimeStatuses(c.Request.Context(), ids, handler.refreshOneDeploymentRuntimeStatus)
	summary := map[string]int{"running": 0, "stopped": 0, "error": 0, "unknown": 0}
	for _, result := range results {
		if _, ok := summary[result.RuntimeStatus]; ok {
			summary[result.RuntimeStatus]++
		} else {
			summary["unknown"]++
		}
	}
	response.Success(c, gin.H{"results": results, "summary": summary, "total": len(results)})
}

func collectDeploymentRuntimeStatuses(ctx context.Context, ids []int64, check func(context.Context, int64) deploymentRuntimeOutcome) []deploymentRuntimeOutcome {
	seen := make(map[int64]bool, len(ids))
	targets := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			targets = append(targets, id)
		}
	}
	results := make([]deploymentRuntimeOutcome, len(targets))
	// Bound actual Agent work, including scopes containing many services.
	semaphore := make(chan struct{}, 8)
	var waitGroup sync.WaitGroup
	for index, id := range targets {
		select {
		case semaphore <- struct{}{}:
		case <-ctx.Done():
			results[index] = deploymentRuntimeFailure(id, ctx.Err())
			continue
		}
		waitGroup.Add(1)
		go func(index int, id int64) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()
			results[index] = check(ctx, id)
		}(index, id)
	}
	waitGroup.Wait()
	return results
}

func deploymentRuntimeFailure(id int64, err error) deploymentRuntimeOutcome {
	slog.Error("deployment runtime observation failed", "deployment_id", id, "error", err)
	return deploymentRuntimeOutcome{ID: id, RuntimeStatus: "error", RuntimeStatusOutput: err.Error(), Error: err.Error()}
}

func (handler *Handler) refreshOneDeploymentRuntimeStatus(ctx context.Context, id int64) deploymentRuntimeOutcome {
	item, err := handler.service.repository.GetApplicationDeployment(ctx, id)
	if err != nil {
		return deploymentRuntimeFailure(id, translate(err))
	}
	var checkErr error
	if item.Enabled {
		_, checkErr = handler.service.executeDeploymentControl(ctx, handler.gateway, id, "status")
		item, err = handler.service.repository.GetApplicationDeployment(ctx, id)
		if err != nil {
			return deploymentRuntimeFailure(id, err)
		}
	}
	result := deploymentRuntimeOutcome{
		ID: id, RuntimeStatus: item.RuntimeStatus, RuntimeStatusOutput: item.RuntimeStatusOutput,
		LastStatusCheckTime: item.LastStatusCheckTime,
	}
	if checkErr != nil {
		// A failed check must never leave the old "running" snapshot looking fresh,
		// including when writing the failure snapshot itself failed.
		result.RuntimeStatus = "error"
		result.Error = checkErr.Error()
		if item.RuntimeStatus != "error" {
			result.RuntimeStatusOutput = checkErr.Error()
		}
		slog.Error("deployment runtime check failed", "deployment_id", id, "error", checkErr)
	}
	return result
}

// RefreshHostInfo dispatches a synchronous get_host_info job to the host's agent, persists the
// result, then returns {result, host} — mirroring Django's HostViewSet.refresh_info.
func (handler *Handler) RefreshHostInfo(context *gin.Context) {
	context.Header("Cache-Control", "no-store")
	id, ok := resourceID(context)
	if !ok {
		return
	}
	ctx := context.Request.Context()
	item, err := handler.service.GetHost(ctx, id)
	if err != nil {
		respond(context, item, err)
		return
	}
	handler.applyAgentPresence(&item)
	result := handler.refreshHostAgentInfo(ctx, item)

	item, err = handler.service.GetHost(ctx, id)
	if err != nil {
		respond(context, item, err)
		return
	}
	handler.applyAgentPresence(&item)
	detail, detailErr := handler.getHostDetail(ctx, item)
	if detailErr != nil {
		respond(context, detail, detailErr)
		return
	}
	response.Success(context, gin.H{"result": result, "host": detail})
}

// BatchRefreshHostInfo dispatches get_host_info to each requested host's agent with bounded
// concurrency (mirroring Django's ThreadPoolExecutor(max_workers=8)) and returns the freshest
// detail only for hosts that were actually updated, so the frontend can merge them in place.
func (handler *Handler) BatchRefreshHostInfo(context *gin.Context) {
	context.Header("Cache-Control", "no-store")
	ids, err := requiredIDListQuery(context, "ids")
	if err != nil {
		response.Error(context, ErrInvalid)
		return
	}
	ctx := context.Request.Context()

	results := make([]hostInfoOutcome, len(ids))
	const maxConcurrency = 8
	semaphore := make(chan struct{}, maxConcurrency)
	var waitGroup sync.WaitGroup
	for index, id := range ids {
		waitGroup.Add(1)
		semaphore <- struct{}{}
		go func(index int, id int64) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()
			results[index] = handler.refreshOneHostForBatch(ctx, id)
		}(index, id)
	}
	waitGroup.Wait()

	hosts := make([]HostDetail, 0)
	for _, result := range results {
		if !result.Updated {
			continue
		}
		item, err := handler.service.GetHost(ctx, result.HostID)
		if err != nil {
			continue
		}
		handler.applyAgentPresence(&item)
		detail, detailErr := handler.getHostDetail(ctx, item)
		if detailErr != nil {
			continue
		}
		hosts = append(hosts, detail)
	}
	response.Success(context, gin.H{"results": results, "hosts": hosts})
}

func (handler *Handler) refreshOneHostForBatch(ctx context.Context, id int64) hostInfoOutcome {
	item, err := handler.service.GetHost(ctx, id)
	if err != nil {
		return hostInfoOutcome{HostID: id, Error: err.Error()}
	}
	handler.applyAgentPresence(&item)
	return handler.refreshHostAgentInfo(ctx, item)
}

// RefreshApplicationServiceRuntimeStatus 同步检查逻辑服务名下所有启用部署实例的运行状态
// （最多 8 并发，与批量采集主机信息的限流口径一致），逐实例把结果写回 runtime_status，
// 并返回 {running, stopped, error, unknown} 汇总，供服务树刷新后的提示与状态列使用。
func (handler *Handler) RefreshApplicationServiceRuntimeStatus(context *gin.Context) {
	context.Header("Cache-Control", "no-store")
	id, ok := resourceID(context)
	if !ok {
		return
	}
	ctx := context.Request.Context()
	if _, err := handler.service.repository.GetApplicationService(ctx, id); err != nil {
		respond(context, nil, translate(err))
		return
	}
	deployments, _, err := handler.service.repository.ListApplicationDeployments(ctx, pagination.Page{Number: 1, Size: 100000, Offset: 0}, ApplicationDeploymentFilter{ApplicationServiceID: id})
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	targets := make([]int64, 0, len(deployments))
	for _, item := range deployments {
		if item.Enabled {
			targets = append(targets, item.ID)
		}
	}
	statuses := make([]string, len(targets))
	const maxConcurrency = 8
	semaphore := make(chan struct{}, maxConcurrency)
	var waitGroup sync.WaitGroup
	for index := range targets {
		waitGroup.Add(1)
		semaphore <- struct{}{}
		go func(index int) {
			defer waitGroup.Done()
			defer func() { <-semaphore }()
			statuses[index] = handler.service.checkDeploymentRuntimeStatus(ctx, targets[index])
		}(index)
	}
	waitGroup.Wait()

	summary := map[string]int{"running": 0, "stopped": 0, "error": 0, "unknown": 0}
	for _, status := range statuses {
		if _, ok := summary[status]; ok {
			summary[status]++
			continue
		}
		summary["unknown"]++
	}
	response.Success(context, gin.H{"summary": summary, "total": len(targets)})
}

func (handler *Handler) GetHostWebSSHActiveCount(context *gin.Context) {
	if _, ok := resourceID(context); !ok {
		return
	}
	respond(context, gin.H{"count": 0, "active_count": 0}, nil)
}

func (handler *Handler) GetHostWebSSHActiveSessions(context *gin.Context) {
	if _, ok := resourceID(context); !ok {
		return
	}
	respond(context, []any{}, nil)
}
