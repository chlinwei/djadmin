package inspection

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

type groupInput struct {
	Name        *string       `json:"name"`
	Scope       *string       `json:"scope"`
	Description *string       `json:"description"`
	Enabled     *bool         `json:"enabled"`
	Checks      *[]checkInput `json:"checks"`
}

type checkInput struct {
	Name              string         `json:"name"`
	Executor          string         `json:"executor"`
	ExecutionLocation string         `json:"execution_location"`
	Config            map[string]any `json:"config"`
	Severity          string         `json:"severity"`
	Enabled           *bool          `json:"enabled"`
	Order             int            `json:"order"`
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
		if input.Name == nil || strings.TrimSpace(*input.Name) == "" || input.Scope == nil {
			response.BusinessError(context, 400, "名称和执行范围不能为空", nil)
			return
		}
		description, enabled := "", true
		if input.Description != nil {
			description = *input.Description
		}
		if input.Enabled != nil {
			enabled = *input.Enabled
		}
		result, execErr := transaction.ExecContext(context, `INSERT INTO inspection_group(name,scope,description,enabled,create_time,update_time) VALUES(?,?,?,?,NOW(),NOW())`, strings.TrimSpace(*input.Name), *input.Scope, description, enabled)
		if execErr != nil {
			response.BusinessError(context, 400, "巡检组名称已存在", nil)
			return
		}
		id, err = result.LastInsertId()
	} else {
		var oldScope string
		var taskCount int
		err = transaction.QueryRowContext(context, `SELECT g.scope,(SELECT COUNT(*) FROM inspection_task t WHERE t.group_id=g.id) FROM inspection_group g WHERE g.id=? FOR UPDATE`, id).Scan(&oldScope, &taskCount)
		if err != nil {
			response.BusinessError(context, 404, "巡检组不存在", nil)
			return
		}
		if input.Scope != nil && taskCount > 0 && (*input.Scope == "per_host") != (oldScope == "per_host") {
			response.BusinessError(context, 400, "巡检组已被任务引用，不能在“逻辑服务”与“主机组”之间切换范围，请新建巡检组", nil)
			return
		}
		_, err = transaction.ExecContext(context, `UPDATE inspection_group SET name=COALESCE(?,name),scope=COALESCE(?,scope),description=COALESCE(?,description),enabled=COALESCE(?,enabled),update_time=NOW() WHERE id=?`, input.Name, input.Scope, input.Description, input.Enabled, id)
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
			location, severity, enabled := check.ExecutionLocation, check.Severity, true
			if location == "" {
				location = "agent"
			}
			if severity == "" {
				severity = "critical"
			}
			if check.Enabled != nil {
				enabled = *check.Enabled
			}
			_, err = transaction.ExecContext(context, `INSERT INTO inspection_check(group_id,name,executor,execution_location,config,severity,enabled,`+"`order`"+`,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,NOW(),NOW())`, id, check.Name, check.Executor, location, config, severity, enabled, check.Order)
			if err != nil {
				response.BusinessError(context, 400, "同一巡检组内检查项名称不能重复", nil)
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

func (handler *Handler) loadGroup(context *gin.Context, id int64) (gin.H, error) {
	rows, err := handler.db.QueryContext(context, `SELECT id,name,scope,description,enabled,create_time,update_time FROM inspection_group WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	items, err := scanRows(rows)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, sql.ErrNoRows
	}
	checkRows, err := handler.db.QueryContext(context, `SELECT id,name,executor,execution_location,config,severity,enabled,`+"`order`"+` FROM inspection_check WHERE group_id=? ORDER BY `+"`order`"+`,id`, id)
	if err != nil {
		return nil, err
	}
	items[0]["checks"], err = scanRows(checkRows)
	return items[0], err
}

func validateGroupInput(input groupInput) string {
	if input.Scope != nil && !map[string]bool{"per_deployment": true, "service_once": true, "per_host": true}[*input.Scope] {
		return "执行范围无效"
	}
	if input.Checks == nil {
		return ""
	}
	seen := make(map[string]bool)
	for _, check := range *input.Checks {
		if seen[check.Name] {
			return "同一巡检组内检查项名称不能重复"
		}
		seen[check.Name] = true
		if message := validateCheck(check); message != "" {
			return fmt.Sprintf("检查项 %s: %s", check.Name, message)
		}
		if input.Scope != nil && *input.Scope == "per_host" && containsApplicationVariable(check.Config) {
			return "主机组巡检不能使用应用上下文变量，请使用 ${HOST_IP} 或 ${HOST_NAME}"
		}
	}
	return ""
}

func validateCheck(check checkInput) string {
	if !map[string]bool{"shell": true, "schema_validate": true, "http": true, "tcp": true}[check.Executor] {
		return "执行器无效"
	}
	location := check.ExecutionLocation
	if location == "" {
		location = "agent"
	}
	if location == "controller" && check.Executor == "schema_validate" {
		return "djadmin 端不支持 Schema 校验，请改用 Agent 执行"
	}
	if check.Executor == "shell" && strings.TrimSpace(fmt.Sprint(check.Config["command"])) == "" {
		return "Shell 命令不能为空"
	}
	if check.Executor == "http" {
		url := fmt.Sprint(check.Config["url"])
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			return "HTTP URL 必须以 http:// 或 https:// 开头"
		}
	}
	return ""
}

func containsApplicationVariable(value any) bool {
	raw, _ := json.Marshal(value)
	for _, variable := range []string{"${APP_HOME}", "${RUN_USER}", "${INSTANCE_NAME}", "${APPLICATION_VERSION}", "${SERVICE_NAME}"} {
		if strings.Contains(string(raw), variable) {
			return true
		}
	}
	return false
}
