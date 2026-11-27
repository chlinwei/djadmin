package assets

import (
	"autoadmin/internal/api/batchcrud"
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/batchwrite"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) DeleteCredentials(context *gin.Context) {
	ids, ok := batchDeleteIDs(context)
	if !ok {
		return
	}
	ids = batchwrite.UniqueIDs(ids)
	err := handler.service.DeleteCredentials(context.Request.Context(), ids)
	response.Batch(context, batchcrud.AtomicResults(ids, err))
}

func (handler *Handler) HostGroupTree(context *gin.Context) {
	tree, err := handler.service.HostGroupTree(context.Request.Context())
	respond(context, tree, err)
}

func (handler *Handler) DeleteHosts(context *gin.Context) {
	ids, ok := batchDeleteIDs(context)
	if !ok {
		return
	}
	ids = batchwrite.UniqueIDs(ids)
	err := handler.service.DeleteHosts(context.Request.Context(), ids)
	deleted := 0
	if err == nil {
		deleted = len(ids)
	}
	response.BatchWith(context, batchcrud.AtomicResults(ids, err), gin.H{"deleted_count": deleted})
}
