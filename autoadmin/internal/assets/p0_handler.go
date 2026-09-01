package assets

import (
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) DeleteCredentials(context *gin.Context) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	respond(context, nil, handler.service.DeleteCredentials(context.Request.Context(), input.IDs))
}

func (handler *Handler) HostGroupTree(context *gin.Context) {
	tree, err := handler.service.HostGroupTree(context.Request.Context())
	respond(context, tree, err)
}

func (handler *Handler) DeleteHosts(context *gin.Context) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	if err := handler.service.DeleteHosts(context.Request.Context(), input.IDs); err != nil {
		respond(context, nil, err)
		return
	}
	response.Success(context, gin.H{"deleted_count": len(input.IDs)})
}
