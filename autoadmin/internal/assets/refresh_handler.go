package assets

import (
	"context"
	"sync"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

// RefreshHostInfo dispatches a synchronous get_host_info job to the host's agent, persists the
// result, then returns {result, host} — mirroring Django's HostViewSet.refresh_info.
func (handler *Handler) RefreshHostInfo(context *gin.Context) {
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
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if err := context.ShouldBindJSON(&input); err != nil || len(input.IDs) == 0 {
		response.Error(context, ErrInvalid)
		return
	}
	ctx := context.Request.Context()

	results := make([]hostInfoOutcome, len(input.IDs))
	const maxConcurrency = 8
	semaphore := make(chan struct{}, maxConcurrency)
	var waitGroup sync.WaitGroup
	for index, id := range input.IDs {
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
