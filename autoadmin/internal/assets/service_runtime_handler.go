package assets

import (
	"fmt"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListApplicationServices(context *gin.Context) {
	pageValue, err := page(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	businessSystemID, err := optionalIDQuery(context, "business_system")
	if err != nil {
		response.Error(context, err)
		return
	}
	items, count, err := handler.service.repository.ListApplicationServices(context.Request.Context(), context.Query("search"), pageValue, businessSystemID)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	response.Paginated(context, items, count, pageValue.Number, pageValue.Size)
}
func (handler *Handler) GetApplicationService(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	item, err := handler.service.repository.GetApplicationService(context.Request.Context(), id)
	respond(context, item, translate(err))
}
func (handler *Handler) GetApplicationServiceLogConfig(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	items, err := handler.service.repository.ListServiceLogSettings(context.Request.Context(), id)
	respond(context, items, translate(err))
}

// optionalIDQuery 读取可选的整数型 query 参数；未传返回 0，传了但不是合法整数返回 400。
func optionalIDQuery(context *gin.Context, key string) (int64, error) {
	raw := strings.TrimSpace(context.Query(key))
	if raw == "" {
		return 0, nil
	}
	value, parseErr := strconv.ParseInt(raw, 10, 64)
	if parseErr != nil {
		return 0, apperror.NewWithHTTP(apperror.CodeInvalidArgument, fmt.Sprintf("%s 参数无效: %s", key, raw), 0)
	}
	return value, nil
}

// applicationDeploymentFilterFromQuery 解析部署实例列表的过滤参数，
// 与 Django 版 DRF filter 字段名保持一致（application_service / application_service__business_system / application_service__environment）。
func applicationDeploymentFilterFromQuery(context *gin.Context) (ApplicationDeploymentFilter, error) {
	var filter ApplicationDeploymentFilter
	if raw := strings.TrimSpace(context.Query("application_service")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			return filter, apperror.NewWithHTTP(apperror.CodeInvalidArgument, fmt.Sprintf("application_service 参数无效: %s", raw), 0)
		}
		filter.ApplicationServiceID = value
	}
	if raw := strings.TrimSpace(context.Query("application_service__business_system")); raw != "" {
		value, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil {
			return filter, apperror.NewWithHTTP(apperror.CodeInvalidArgument, fmt.Sprintf("application_service__business_system 参数无效: %s", raw), 0)
		}
		filter.BusinessSystemID = value
	}
	if raw, ok := context.GetQuery("application_service__environment"); ok {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			// 显式传空表示“未配置环境”（Django 侧 null 语义），环境过滤交由前端按值比对
			filter.EnvironmentID = nil
		} else {
			value, parseErr := strconv.ParseInt(raw, 10, 64)
			if parseErr != nil {
				return filter, apperror.NewWithHTTP(apperror.CodeInvalidArgument, fmt.Sprintf("application_service__environment 参数无效: %s", raw), 0)
			}
			filter.EnvironmentID = &value
		}
	}
	return filter, nil
}

func (handler *Handler) ListApplicationDeployments(context *gin.Context) {
	pageValue, err := page(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	filter, err := applicationDeploymentFilterFromQuery(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, count, err := handler.service.repository.ListApplicationDeployments(context.Request.Context(), pageValue, filter)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	response.Paginated(context, items, count, pageValue.Number, pageValue.Size)
}

func (handler *Handler) GetApplicationDeployment(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	items, _, err := handler.service.repository.ListApplicationDeployments(context.Request.Context(), pagination.Page{Number: 1, Size: 100000}, ApplicationDeploymentFilter{})
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	for _, item := range items {
		if item.ID == id {
			respond(context, item, nil)
			return
		}
	}
	respond(context, nil, ErrNotFound)
}
func (handler *Handler) CreateApplicationService(context *gin.Context) {
	input, ok := bind[ApplicationServiceInput](context)
	if !ok {
		return
	}
	item, err := handler.service.SaveApplicationService(context, 0, input)
	respond(context, item, err)
}
func (handler *Handler) UpdateApplicationService(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	input, ok := bind[ApplicationServiceInput](context)
	if !ok {
		return
	}
	item, err := handler.service.SaveApplicationService(context, id, input)
	respond(context, item, err)
}
func (handler *Handler) DeleteApplicationService(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	respond(context, nil, handler.service.DeleteApplicationService(context.Request.Context(), id))
}
func (handler *Handler) CreateApplicationDeployment(context *gin.Context) {
	input, ok := bind[ApplicationDeploymentInput](context)
	if !ok {
		return
	}
	item, err := handler.service.SaveApplicationDeployment(context, 0, input)
	respond(context, item, err)
}
func (handler *Handler) UpdateApplicationDeployment(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	input, ok := bind[ApplicationDeploymentInput](context)
	if !ok {
		return
	}
	item, err := handler.service.SaveApplicationDeployment(context, id, input)
	respond(context, item, err)
}
func (handler *Handler) DeleteApplicationDeployment(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	respond(context, nil, handler.service.DeleteApplicationDeployment(context.Request.Context(), id))
}

func (handler *Handler) ControlApplicationDeployment(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	var request struct {
		Action string `json:"action"`
	}
	if err := context.ShouldBindJSON(&request); err != nil {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	action := strings.ToLower(strings.TrimSpace(request.Action))
	if action != "status" && action != "start" && action != "stop" {
		response.Error(context, apperror.ErrInvalidRequest)
		return
	}
	if deploymentGateway != nil {
		result, executeErr := handler.service.executeDeploymentControl(context.Request.Context(), nil, id, action)
		respond(context, result, executeErr)
		return
	}
	items, _, err := handler.service.repository.ListApplicationDeployments(context.Request.Context(), pagination.Page{Number: 1, Size: 100000}, ApplicationDeploymentFilter{})
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	for _, item := range items {
		if item.ID != id {
			continue
		}
		if action != "status" {
			respond(context, nil, ErrAgentUnavailable)
			return
		}
		respond(context, gin.H{"job_id": nil, "action": action, "status": "success", "output": item.RuntimeStatusOutput, "exit_code": 0, "runtime_status": item.RuntimeStatus, "last_status_check_time": item.LastStatusCheckTime}, nil)
		return
	}
	respond(context, nil, ErrNotFound)
}
