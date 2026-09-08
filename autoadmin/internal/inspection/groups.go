package inspection

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/opapolicy"

	"github.com/gin-gonic/gin"
)

// paramInput 是巡检组的检查参数声明（形参）：检查项里用 ${name} 引用，
// 任务绑定时赋值（固定值 / 引用内置变量 / 引用服务宏）。
type paramInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	Default     string `json:"default"`
}

type groupInput struct {
	Name        *string       `json:"name"`
	Description *string       `json:"description"`
	Enabled     *bool         `json:"enabled"`
	Category    *string       `json:"category"`
	Application *int64        `json:"application"`
	Params      *[]paramInput `json:"params"`
	Checks      *[]checkInput `json:"checks"`
}

type checkInput struct {
	Name     string         `json:"name"`
	Config   map[string]any `json:"config"`
	Severity string         `json:"severity"`
	Enabled  *bool          `json:"enabled"`
	Order    int            `json:"order"`
}

func (handler *Handler) GetGroup(context *gin.Context) {
	item, err := handler.loadGroup(context, parseID(context.Param("id")))
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "巡检组不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	response.Success(context, item)
}

func (handler *Handler) SaveGroup(context *gin.Context) {
	var input groupInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	id := parseID(context.Param("id"))
	if message := validateGroupInput(input); message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer transaction.Rollback()
	if id == 0 {
		if input.Name == nil || strings.TrimSpace(*input.Name) == "" {
			response.BusinessError(context, 400, "巡检组名称不能为空", nil)
			return
		}
		description, enabled, category := "", true, groupCategory(input.Category, "general")
		if input.Description != nil {
			description = *input.Description
		}
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		if message, valid := handler.validateGroupApplication(context, input.Application, category); !valid {
			response.BusinessError(context, 400, message, nil)
			return
		}
		result, execErr := transaction.ExecContext(context, `INSERT INTO inspection_group(name,description,enabled,category,application_id,params,create_time,update_time) VALUES(?,?,?,?,?,?,NOW(),NOW())`, strings.TrimSpace(*input.Name), description, enabled, category, nullableIDPtr(input.Application), jsonBytes(input.Params))
		if execErr != nil {
			response.BusinessError(context, 400, "巡检组名称已存在", nil)
			return
		}
		id, err = result.LastInsertId()
	} else {
		var currentCategory string
		if err = transaction.QueryRowContext(context, `SELECT category FROM inspection_group WHERE id=? FOR UPDATE`, id).Scan(&currentCategory); err != nil {
			response.BusinessError(context, 404, "巡检组不存在", nil)
			return
		}
		// 编辑：提交了分类或应用标签就按"编辑后的分类"校验必选（分类未提交则查原值）。
		if input.Category != nil || input.Application != nil {
			effectiveCategory := groupCategory(input.Category, currentCategory)
			if message, valid := handler.validateGroupApplication(context, input.Application, effectiveCategory); !valid {
				response.BusinessError(context, 400, message, nil)
				return
			}
		}
		var paramsJSON any
		if input.Params != nil {
			paramsJSON = jsonBytes(*input.Params)
		}
		_, err = transaction.ExecContext(context, `UPDATE inspection_group SET name=COALESCE(?,name),description=COALESCE(?,description),enabled=COALESCE(?,enabled),category=COALESCE(?,category),application_id=COALESCE(?,application_id),params=COALESCE(?,params),update_time=NOW() WHERE id=?`, input.Name, input.Description, input.Enabled, input.Category, nullableIDPtr(input.Application), paramsJSON, id)
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if input.Checks != nil {
		if _, err = transaction.ExecContext(context, `DELETE FROM inspection_check WHERE group_id=?`, id); err != nil {
			response.Error(context, err)
			return
		}
		for _, check := range *input.Checks {
			config, _ := json.Marshal(check.Config)
			severity, enabled := check.Severity, true
			if severity == "" {
				severity = "critical"
			}
			if check.Enabled != nil {
				enabled = *check.Enabled
			}
			// 唯一执行器 OPA、唯一执行位置 Agent 端：executor/execution_location 列已删除（迁移 000009）。
			_, err = transaction.ExecContext(context, `INSERT INTO inspection_check(group_id,name,config,severity,enabled,`+"`order`"+`,create_time,update_time) VALUES(?,?,?,?,?,?,NOW(),NOW())`, id, check.Name, config, severity, enabled, check.Order)
			if err != nil {
				// 名称重复在上面的 validateGroupInput 已拦截并带名字；能走到这里的失败是别的原因，
				// 必须透传真实错误，不能笼统归为名称重复。
				response.Error(context, err)
				return
			}
		}
	}
	if err = transaction.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	item, err := handler.loadGroup(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, item)
}

func (handler *Handler) DeleteGroup(context *gin.Context) {
	id := parseID(context.Param("id"))
	var count int
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM inspection_task WHERE group_id=?`, id).Scan(&count); err != nil {
		response.Error(context, err)
		return
	}
	if count > 0 {
		response.BusinessError(context, 400, "巡检组已被任务使用，不能删除", nil)
		return
	}
	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer transaction.Rollback()
	// Django performs CASCADE in the ORM; the physical MySQL foreign key is NO ACTION.
	if _, err = transaction.ExecContext(context, `DELETE FROM inspection_check WHERE group_id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	result, err := transaction.ExecContext(context, `DELETE FROM inspection_group WHERE id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		response.BusinessError(context, 404, "巡检组不存在", nil)
		return
	}
	if err = transaction.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, nil)
}

func validateGroupInput(input groupInput) string {
	if input.Category != nil && !map[string]bool{"general": true, "application": true}[*input.Category] {
		return "巡检组分类无效，仅支持通用（general）或应用类型（application）"
	}
	if message := validateParamDeclarations(input.Category, input.Params); message != "" {
		return message
	}
	if input.Checks == nil {
		return ""
	}
	// 应用上下文变量（${APP_HOME} 等）只有实例上下文能展开；实例上下文由
	// 应用类型组的挂载点（business/service）提供，通用组挂任何地方都是主机
	// 上下文——所以按分类校验，而不是已废弃的 scope。
	category := groupCategory(input.Category, "general")
	seen := make(map[string]bool)
	for _, check := range *input.Checks {
		if seen[check.Name] {
			return fmt.Sprintf("同一巡检组内检查项名称不能重复: %q", check.Name)
		}
		seen[check.Name] = true
		if message := validateCheck(check); message != "" {
			return fmt.Sprintf("检查项 %s: %s", check.Name, message)
		}
		if category == "general" && containsApplicationVariable(check.Config) {
			return fmt.Sprintf("通用巡检组不能使用应用上下文变量（%s），请使用 ${HOST_IP} 或 ${HOST_NAME}；如需检查应用实例请使用应用类型巡检组", strings.Join(applicationVariables, "、"))
		}
	}
	return ""
}

var applicationVariables = []string{"${APP_HOME}", "${RUN_USER}", "${INSTANCE_NAME}", "${APPLICATION_VERSION}", "${SERVICE_NAME}"}

// validateParamDeclarations 校验检查参数声明：名称唯一、不得与内置变量重名。
func validateParamDeclarations(category *string, params *[]paramInput) string {
	if params == nil {
		return ""
	}
	standard := map[string]bool{}
	for _, variable := range applicationVariables {
		standard[strings.Trim(variable, "${}")] = true
	}
	seen := make(map[string]bool)
	for _, param := range *params {
		name := strings.TrimSpace(param.Name)
		if name == "" {
			return "检查参数名称不能为空"
		}
		if standard[name] {
			return fmt.Sprintf("检查参数 %q 与内置变量重名，请换个名字", name)
		}
		if seen[name] {
			return fmt.Sprintf("检查参数名称不能重复: %q", name)
		}
		seen[name] = true
	}
	return ""
}

// 巡检唯一执行器：OPA（Rego 策略），固定在 Agent 端执行。
func validateCheck(check checkInput) string {
	return opapolicy.Validate(check.Config)
}

func containsApplicationVariable(value any) bool {
	raw, _ := json.Marshal(value)
	for _, variable := range applicationVariables {
		if strings.Contains(string(raw), variable) {
			return true
		}
	}
	return false
}

func groupCategory(input *string, fallback string) string {
	if input == nil || *input == "" {
		return fallback
	}
	return *input
}

// validateGroupApplication 校验应用类型组的应用标签：应用类型组必选一个真实存在的应用；
// 通用组不适用（忽略）。
func (handler *Handler) validateGroupApplication(context *gin.Context, application *int64, category string) (string, bool) {
	if category != "application" {
		return "", true
	}
	if application == nil || *application <= 0 {
		return "应用类型巡检组必须选择适用应用", false
	}
	var count int
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM assets_application WHERE id=?`, *application).Scan(&count); err != nil {
		return "校验应用标签失败", false
	}
	if count == 0 {
		return "应用标签指向的应用不存在", false
	}
	return "", true
}

func nullableIDPtr(value *int64) any {
	if value == nil || *value <= 0 {
		return nil
	}
	return *value
}
