package assets

import (
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListDeploymentTemplates(context *gin.Context) {
	pageValue, err := page(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, count, err := handler.service.ListDeploymentTemplates(context.Request.Context(), context.Query("search"), pageValue)
	if err != nil {
		respond(context, nil, err)
		return
	}
	response.Paginated(context, items, count, pageValue.Number, pageValue.Size)
}

func (handler *Handler) GetDeploymentTemplate(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	item, err := handler.service.GetDeploymentTemplate(context.Request.Context(), id)
	respond(context, item, err)
}

func (handler *Handler) CreateDeploymentTemplate(context *gin.Context) {
	input, ok := bind[DeploymentTemplateInput](context)
	if !ok {
		return
	}
	item, err := handler.service.SaveDeploymentTemplate(context.Request.Context(), 0, input)
	respond(context, item, err)
}

func (handler *Handler) UpdateDeploymentTemplate(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	input, ok := bind[DeploymentTemplateInput](context)
	if !ok {
		return
	}
	item, err := handler.service.SaveDeploymentTemplate(context.Request.Context(), id, input)
	respond(context, item, err)
}

func (handler *Handler) DeleteDeploymentTemplate(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	respond(context, nil, handler.service.DeleteDeploymentTemplate(context.Request.Context(), id))
}
