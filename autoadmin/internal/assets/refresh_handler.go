package assets

import (
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) RefreshHostInfo(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	item, err := handler.service.GetHost(context.Request.Context(), id)
	if err == nil {
		handler.applyAgentPresence(&item)
	}
	respond(context, item, err)
}

func (handler *Handler) BatchRefreshHostInfo(context *gin.Context) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if err := context.ShouldBindJSON(&input); err != nil || len(input.IDs) == 0 {
		response.Error(context, ErrInvalid)
		return
	}
	items := make([]Host, 0, len(input.IDs))
	for _, id := range input.IDs {
		item, err := handler.service.GetHost(context.Request.Context(), id)
		if err != nil {
			respond(context, nil, err)
			return
		}
		handler.applyAgentPresence(&item)
		items = append(items, item)
	}
	respond(context, gin.H{"hosts": items}, nil)
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
