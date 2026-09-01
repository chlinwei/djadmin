package assets

import (
	"context"
	"sync"

	"autoadmin/internal/api/response"

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

func (handler *Handler) RefreshApplicationServiceRuntimeStatus(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	item, err := handler.service.repository.GetApplicationService(context.Request.Context(), id)
	respond(context, item, translate(err))
}

func (handler *Handler) GetHostAgentRuntimeStatus(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	item, err := handler.service.GetHost(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, err)
		return
	}
	handler.applyAgentPresence(&item)
	respond(context, gin.H{"agent_id": item.AgentID, "online": item.AgentOnline, "last_seen": item.AgentOnlineTime, "collect_status": item.CollectStatus, "collect_message": item.CollectMessage}, nil)
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
