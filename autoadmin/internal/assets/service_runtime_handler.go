package assets

import (
	"fmt"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	"autoadmin/internal/shared/apperror"

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
	items, err := handler.service.repository.ListServiceTemplateLogs(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	collection, err := handler.service.repository.GetApplicationServiceLogCollection(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	// 服务编码单独取：logs 为空（模板没有日志定义）时也要能给出来，否则页面查不到存量流。
	serviceCode, err := handler.service.repository.queries.GetApplicationServiceCode(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	// 前端契约：{logs: [...], log_collection_enabled}（模板日志定义 + 服务级覆盖三态 + 服务级总开关）。
	// 总开关必须一起给：逐条开关在总开关关闭时并不生效，只回 logs 会让页面把"未采集"误显示成
	// "逐条都关着"（用户会去逐条打开，却没有效果）。
	respond(context, ServiceLogConfig{Logs: items, LogCollectionEnabled: collection, ServiceCode: serviceCode}, nil)
}

// SetApplicationServiceLogCollection 服务级日志采集总开关：
// POST /assets/application-services/:id/log-collection/  body {"enabled": true|false}
//
// 关掉后该服务下所有日志都不再采集（逐条开关不生效），重新打开即恢复原有逐条配置。
// 与逐条覆盖值同一个思路：粒度最小的写操作有自己的接口，不必伪造一份完整服务表单。
func (handler *Handler) SetApplicationServiceLogCollection(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	if context.ShouldBindJSON(&input) != nil || input.Enabled == nil {
		response.Error(context, ErrInvalid)
		return
	}
	enabled, err := handler.service.SetServiceLogCollection(context.Request.Context(), id, *input.Enabled)
	if err != nil {
		respond(context, nil, err)
		return
	}
	respond(context, gin.H{"log_collection_enabled": enabled}, nil)
}

// VerifyApplicationServiceLogFormat 日志格式认证：POST /assets/application-services/:id/log-config/verify/
//
// body: {"log_definition_id":<必填>,"source":"instance|sample_log|waiver","deployment_id":<实例 id，source=instance 时必填>}
//
// 通过才写库（记当前指纹 + 依据 + 操作人，见 log_format_verify.go）；不通过返回 200 +
// passed=false + missing_fields，前端据此提示缺哪几个必备字段——"不通过"是正常的业务结果，
// 不是接口错误，所以不用错误码表达。
func (handler *Handler) VerifyApplicationServiceLogFormat(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	var input struct {
		LogDefinitionID int64  `json:"log_definition_id"`
		Source          string `json:"source"`
		DeploymentID    int64  `json:"deployment_id"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.Error(context, ErrInvalid)
		return
	}
	if input.LogDefinitionID < 1 {
		response.Error(context, ErrInvalid)
		return
	}
	source := strings.TrimSpace(input.Source)
	if source == "" {
		source = LogFormatSourceInstance
	}
	// 操作人取自登录态：豁免认证要留痕"是谁确认的"，不接受前端自报身份。
	actor := ""
	if claims, ok := identity.ClaimsFromContext(context); ok {
		actor = claims.Username
	}
	result, err := handler.service.VerifyServiceLogFormat(context.Request.Context(), id, input.LogDefinitionID, input.DeploymentID, source, actor)
	respond(context, result, err)
}

// SaveApplicationServiceLogSetting 按行保存一条 (服务 × 日志定义) 的日志覆盖值：
// POST /assets/application-services/:id/log-config/settings/
//
// body: {"log_definition_id":<必填>,"collection_enabled":true|false|null,"retention_tier":<档位 id>|null}
//
// 两个值都是"覆盖值"语义，null = 不覆盖（采集默认采、档位继承服务默认）。这是**按行**写入，
// 不会影响该服务其他日志的覆盖值——整表替换那条路（SaveApplicationService 的 log_settings）
// 的边界见 docs/architecture/LOG_COLLECTION_ARCHITECTURE.md §9.5。
func (handler *Handler) SaveApplicationServiceLogSetting(context *gin.Context) {
	id, ok := resourceID(context)
	if !ok {
		return
	}
	var input struct {
		LogDefinitionID   int64  `json:"log_definition_id"`
		CollectionEnabled *bool  `json:"collection_enabled"`
		RetentionTier     *int64 `json:"retention_tier"`
	}
	if context.ShouldBindJSON(&input) != nil {
		response.Error(context, ErrInvalid)
		return
	}
	// 回读整行（覆盖值 + 认证状态都由后端算），前端据此就地更新那一行。
	item, err := handler.service.SaveServiceLogOverride(context.Request.Context(), id, ServiceLogOverrideInput{
		LogDefinition: input.LogDefinitionID, CollectionEnabled: input.CollectionEnabled, RetentionTier: input.RetentionTier,
	})
	respond(context, item, err)
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
	// 按 id 直查。历史写法是把全部实例（Size: 100000）拉回来在内存里找目标行——
	// 每请求一次全表，且找不到时的"不存在"结论依赖列表查询的可见性（列表 INNER JOIN 主机）。
	item, err := handler.service.repository.GetApplicationDeployment(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	respond(context, item, nil)
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
	// 没有 agent 网关时的兜底：只能读回库里的既有状态，不能真的执行启停。
	item, err := handler.service.repository.GetApplicationDeployment(context.Request.Context(), id)
	if err != nil {
		respond(context, nil, translate(err))
		return
	}
	if action != "status" {
		respond(context, nil, ErrAgentUnavailable)
		return
	}
	respond(context, gin.H{"job_id": nil, "action": action, "status": "success", "output": item.RuntimeStatusOutput, "exit_code": 0, "runtime_status": item.RuntimeStatus, "last_status_check_time": item.LastStatusCheckTime}, nil)
}
