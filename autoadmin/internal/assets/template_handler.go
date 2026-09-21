package assets

import (
	"database/sql"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)


func (handler *Handler) ListDeploymentTemplates(context *gin.Context) {
	pageValue, err := page(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	// 可选按应用过滤（应用定义页的"部署模板管理"弹窗传入）
	var applicationID sql.NullInt64
	if raw := strings.TrimSpace(context.Query("application")); raw != "" {
		parsed, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || parsed <= 0 {
			response.BusinessError(context, 400, "application 参数无效", nil)
			return
		}
		applicationID = sql.NullInt64{Int64: parsed, Valid: true}
	}
	items, count, err := handler.service.ListDeploymentTemplates(context.Request.Context(), applicationID, context.Query("search"), pageValue)
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

// ListTemplateServices 模板的承载服务清单（只读，一次全量）：
// GET /assets/application-deployment-templates/:id/services/
//
// 日志处理规则页「影响服务数」弹窗用：改一条解析规则前看它经哪些模板落到哪些服务。
// 展示列 = 项目 / 业务系统 / 环境 / 服务名，项目名经 business_system → project 反查
// （业务系统可不属于任何项目，为空串）。单模板关联服务量级小，不分页。
func (handler *Handler) ListTemplateServices(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	rows, err := handler.service.repository.ListApplicationServicesByTemplate(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	response.Success(context, gin.H{"count": len(rows), "results": rows})
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
