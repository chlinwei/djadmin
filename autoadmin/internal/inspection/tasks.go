package inspection

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

type taskInput struct {
	Name            *string  `json:"name"`
	InspectionName  *string  `json:"inspection_name"`
	Group           *int64   `json:"group"`
	Groups          *[]int64 `json:"groups"`
	LogicalService  *int64   `json:"logical_service"`
	SelectedHostIDs *[]int64 `json:"selected_host_ids"`
	Concurrency     *int     `json:"concurrency"`
	TimeoutSeconds  *int     `json:"timeout_seconds"`
	CronExpression  *string  `json:"cron_expression"`
	Enabled         *bool    `json:"enabled"`
}

type taskState struct {
	Name, InspectionName, CronExpression string
	// GroupIDs 是任务绑定的全部巡检组（SOP 语义：1 个通用组 + N 个应用组）；
	// inspection_task.group_id 冗余保存第一个组以兼容旧查询。
	GroupIDs                    []int64
	LogicalServiceID            *int64
	SelectedHostIDs             []int64
	Concurrency, TimeoutSeconds int
	Enabled                     bool
}

func (handler *Handler) GetTask(context *gin.Context) {
	item, err := handler.loadTask(context, parseID(context.Param("id")))
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "巡检任务不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	response.Success(context, item)
}
func (handler *Handler) SaveTask(context *gin.Context) {
	var input taskInput
	if context.ShouldBindJSON(&input) != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	id := parseID(context.Param("id"))
	state := taskState{Concurrency: 20, TimeoutSeconds: 60, Enabled: true, SelectedHostIDs: []int64{}, GroupIDs: []int64{}}
	if id > 0 {
		var selectedHostIDs []byte
		var primaryGroup int64
		if err := handler.db.QueryRowContext(context, `SELECT name,inspection_name,group_id,logical_service_id,selected_host_ids,concurrency,timeout_seconds,cron_expression,enabled FROM inspection_task WHERE id=?`, id).Scan(&state.Name, &state.InspectionName, &primaryGroup, &state.LogicalServiceID, &selectedHostIDs, &state.Concurrency, &state.TimeoutSeconds, &state.CronExpression, &state.Enabled); err != nil {
			response.BusinessError(context, 404, "巡检任务不存在", nil)
			return
		}
		if json.Unmarshal(selectedHostIDs, &state.SelectedHostIDs) != nil {
			response.BusinessError(context, 400, "巡检任务的主机范围数据无效", nil)
			return
		}
		// 现有多组关系优先；存量单组任务回落到 group_id 列。
		if groupIDs, groupErr := handler.loadTaskGroupIDs(context, id); groupErr == nil && len(groupIDs) > 0 {
			state.GroupIDs = groupIDs
		} else if primaryGroup > 0 {
			state.GroupIDs = []int64{primaryGroup}
		}
	}
	mergeTaskInput(&state, input)
	message, validationErr := handler.validateTask(context, &state, id == 0 || input.Group != nil || input.Groups != nil)
	if validationErr != nil {
		response.Error(context, validationErr)
		return
	}
	if message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	hostIDs, _ := json.Marshal(state.SelectedHostIDs)
	nextRun := nextRunTime(state.CronExpression, state.Enabled)
	var err error
	if id == 0 {
		result, execErr := handler.db.ExecContext(context, `INSERT INTO inspection_task(name,inspection_name,group_id,logical_service_id,selected_host_ids,concurrency,timeout_seconds,cron_expression,next_run_time,last_run_time,enabled,create_time,update_time) VALUES(?,?,?,?,?,?,?,?,?,NULL,?,NOW(),NOW())`, state.Name, state.InspectionName, state.GroupIDs[0], state.LogicalServiceID, hostIDs, state.Concurrency, state.TimeoutSeconds, state.CronExpression, nextRun, state.Enabled)
		if execErr != nil {
			response.BusinessError(context, 400, "巡检任务名称已存在", nil)
			return
		}
		id, err = result.LastInsertId()
	} else {
		_, err = handler.db.ExecContext(context, `UPDATE inspection_task SET name=?,inspection_name=?,group_id=?,logical_service_id=?,selected_host_ids=?,concurrency=?,timeout_seconds=?,cron_expression=?,next_run_time=?,enabled=?,update_time=NOW() WHERE id=?`, state.Name, state.InspectionName, state.GroupIDs[0], state.LogicalServiceID, hostIDs, state.Concurrency, state.TimeoutSeconds, state.CronExpression, nextRun, state.Enabled, id)
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if err = handler.saveTaskGroups(context, id, state.GroupIDs); err != nil {
		response.Error(context, err)
		return
	}
	item, err := handler.loadTask(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, item)
}

// loadTaskGroupIDs 读取任务已绑定的巡检组 ID（按保存顺序）。
func (handler *Handler) loadTaskGroupIDs(context *gin.Context, taskID int64) ([]int64, error) {
	return db.New(handler.db).ListInspectionTaskGroupIDs(context, taskID)
}

// saveTaskGroups 全量重建任务↔巡检组关联。
func (handler *Handler) saveTaskGroups(context *gin.Context, taskID int64, groupIDs []int64) error {
	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(context, `DELETE FROM inspection_task_group WHERE task_id=?`, taskID); err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		if _, err = transaction.ExecContext(context, `INSERT INTO inspection_task_group(task_id,group_id) VALUES(?,?)`, taskID, groupID); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func (handler *Handler) DeleteTask(context *gin.Context) {
	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer transaction.Rollback()
	id := parseID(context.Param("id"))
	// Django applies SET_NULL in the ORM while the physical MySQL foreign key remains NO ACTION.
	if _, err = transaction.ExecContext(context, `UPDATE inspection_execution SET task_id=NULL,update_time=NOW() WHERE task_id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	if _, err = transaction.ExecContext(context, `DELETE FROM inspection_task_group WHERE task_id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	result, err := transaction.ExecContext(context, `DELETE FROM inspection_task WHERE id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		response.BusinessError(context, 404, "巡检任务不存在", nil)
		return
	}
	if err = transaction.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, nil)
}

func (handler *Handler) loadTask(context *gin.Context, id int64) (inspectionTaskResponse, error) {
	row, err := db.New(handler.db).GetInspectionTask(context, id)
	if err != nil {
		return inspectionTaskResponse{}, err
	}
	return inspectionTaskResponseFrom(db.ListInspectionTasksTypedRow(row)), nil
}

func mergeTaskInput(state *taskState, input taskInput) {
	if input.Name != nil {
		state.Name = strings.TrimSpace(*input.Name)
	}
	if input.InspectionName != nil {
		state.InspectionName = strings.TrimSpace(*input.InspectionName)
	}
	// groups 优先；group 为兼容字段（单组），两者都没提交时保留原值。
	if input.Groups != nil {
		state.GroupIDs = uniquePositiveInt64s(*input.Groups)
	} else if input.Group != nil && *input.Group > 0 {
		state.GroupIDs = uniquePositiveInt64s([]int64{*input.Group})
	}
	if input.LogicalService != nil {
		state.LogicalServiceID = input.LogicalService
	}
	if input.SelectedHostIDs != nil {
		state.SelectedHostIDs = *input.SelectedHostIDs
	}
	if input.Concurrency != nil {
		state.Concurrency = *input.Concurrency
	}
	if input.TimeoutSeconds != nil {
		state.TimeoutSeconds = *input.TimeoutSeconds
	}
	if input.CronExpression != nil {
		state.CronExpression = strings.TrimSpace(*input.CronExpression)
	}
	if input.Enabled != nil {
		state.Enabled = *input.Enabled
	}
}

func (handler *Handler) validateTask(context *gin.Context, state *taskState, validateGroupAvailability bool) (string, error) {
	if state.Name == "" || len(state.GroupIDs) == 0 {
		return "名称和巡检组不能为空", nil
	}
	if state.Concurrency < 1 || state.Concurrency > 100 {
		return "并发数必须在 1 到 100 之间", nil
	}
	if state.TimeoutSeconds < 5 || state.TimeoutSeconds > 3600 {
		return "单目标超时必须在 5 到 3600 秒之间", nil
	}
	if state.CronExpression != "" {
		if state.InspectionName == "" {
			return "配置定时计划时必须填写巡检名称", nil
		}
		if _, err := cron.ParseStandard(state.CronExpression); err != nil {
			return "cron 表达式无效，必须为 5 段：分 时 日 月 周", nil
		}
	}
	// 多组绑定要求所有组 scope 一致：目标解析按 scope 类别走（per_host / 逻辑服务），
	// 混合 scope 的组无法在一次执行里确定目标。
	scopes := make([]string, 0, len(state.GroupIDs))
	enabledGroups := make([]struct {
		id         int64
		scope      string
		enabled    bool
		checkCount int
	}, 0, len(state.GroupIDs))
	for _, groupID := range state.GroupIDs {
		var scope string
		var enabled bool
		var checkCount int
		err := handler.db.QueryRowContext(context, `SELECT g.scope,g.enabled,(SELECT COUNT(*) FROM inspection_check c WHERE c.group_id=g.id AND c.enabled=TRUE) FROM inspection_group g WHERE g.id=?`, groupID).Scan(&scope, &enabled, &checkCount)
		if errors.Is(err, sql.ErrNoRows) {
			return "巡检组不存在", nil
		}
		if err != nil {
			return "", err
		}
		scopes = append(scopes, scope)
		enabledGroups = append(enabledGroups, struct {
			id         int64
			scope      string
			enabled    bool
			checkCount int
		}{groupID, scope, enabled, checkCount})
	}
	if len(uniqueStrings(scopes)) > 1 {
		return "一次巡检组合的巡检组必须具有相同的执行范围", nil
	}
	scope := scopes[0]
	// DRF only runs field-level group validation when PATCH explicitly submits the group.
	if validateGroupAvailability {
		for _, group := range enabledGroups {
			if !group.enabled {
				return "存在已禁用的巡检组", nil
			}
			if group.checkCount == 0 {
				return "巡检组没有启用的检查项", nil
			}
		}
	}
	if scope == "per_host" {
		state.LogicalServiceID = nil
		state.SelectedHostIDs = uniqueInt64s(state.SelectedHostIDs)
		if len(state.SelectedHostIDs) == 0 {
			return "请勾选主机", nil
		}
		var count int
		placeholders, arguments := make([]string, len(state.SelectedHostIDs)), make([]any, len(state.SelectedHostIDs))
		for index, hostID := range state.SelectedHostIDs {
			placeholders[index], arguments[index] = "?", hostID
		}
		if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM assets_host WHERE is_deleted_in_cloud=FALSE AND id IN (`+strings.Join(placeholders, ",")+`)`, arguments...).Scan(&count); err != nil {
			return "", err
		}
		if count != len(state.SelectedHostIDs) {
			return "勾选主机包含不存在或已删除的主机", nil
		}
	} else {
		state.SelectedHostIDs = []int64{}
		if state.LogicalServiceID == nil {
			return "请选择逻辑服务", nil
		}
		var count int
		if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM assets_application_service WHERE id=?`, *state.LogicalServiceID).Scan(&count); err != nil {
			return "", err
		}
		if count == 0 {
			return "逻辑服务不存在", nil
		}
	}
	return "", nil
}

func uniqueInt64s(values []int64) []int64 {
	result := make([]int64, 0, len(values))
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func uniquePositiveInt64s(values []int64) []int64 {
	filtered := make([]int64, 0, len(values))
	for _, value := range values {
		if value > 0 {
			filtered = append(filtered, value)
		}
	}
	return uniqueInt64s(filtered)
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func nextRunTime(expression string, enabled bool) any {
	if expression == "" || !enabled {
		return nil
	}
	schedule, err := cron.ParseStandard(expression)
	if err != nil {
		return nil
	}
	return schedule.Next(time.Now().UTC())
}
