package inspection

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"
)

// groupBindingInput 是「巡检组 → 挂载点」绑定输入。
type groupBindingInput struct {
	Group            *int64          `json:"group_id"`
	MountType        string          `json:"mount_type"`
	ProjectID        *int64          `json:"project_id"`
	EnvironmentID    *int64          `json:"environment_id"`
	BusinessSystemID *int64          `json:"business_system_id"`
	ServiceID        *int64          `json:"service_id"`
	InstanceMode     string          `json:"instance_mode"`
	ParamValues      json.RawMessage `json:"param_values"`
}

type taskInput struct {
	Name           *string              `json:"name"`
	InspectionName *string              `json:"inspection_name"`
	Group          *int64               `json:"group"`
	Groups         *json.RawMessage     `json:"groups"`
	Bindings       *[]groupBindingInput `json:"bindings"`
	Concurrency    *int                 `json:"concurrency"`
	TimeoutSeconds *int                 `json:"timeout_seconds"`
	CronExpression *string              `json:"cron_expression"`
	Enabled        *bool                `json:"enabled"`
}

type taskState struct {
	Name, InspectionName, CronExpression string
	// Bindings 是任务绑定的「巡检组 → 挂载点」（单组模型至多一项）；
	// GroupIDs 冗余保存组 ID，inspection_task.group_id 保存该组 ID。
	GroupIDs                    []int64
	Bindings                    []groupBindingInput
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
	state := taskState{Concurrency: 20, TimeoutSeconds: 60, Enabled: true, GroupIDs: []int64{}}
	if id > 0 {
		var primaryGroup int64
		if err := handler.db.QueryRowContext(context, `SELECT name,inspection_name,group_id,concurrency,timeout_seconds,cron_expression,enabled FROM inspection_task WHERE id=?`, id).Scan(&state.Name, &state.InspectionName, &primaryGroup, &state.Concurrency, &state.TimeoutSeconds, &state.CronExpression, &state.Enabled); err != nil {
			response.BusinessError(context, 404, "巡检任务不存在", nil)
			return
		}
		// 现有绑定优先（带挂载点）；无绑定行的存量任务要求重新选择巡检组。
		if bindings, bindErr := db.New(handler.db).ListInspectionTaskBindings(context, id); bindErr == nil && len(bindings) > 0 {
			state.Bindings = make([]groupBindingInput, 0, len(bindings))
			for _, row := range bindings {
				state.Bindings = append(state.Bindings, groupBindingInput{
					Group:            &row.GroupID,
					MountType:        row.MountType,
					ProjectID:        nullableInt64Ptr(row.ProjectID),
					EnvironmentID:    nullableInt64Ptr(row.EnvironmentID),
					BusinessSystemID: nullableInt64Ptr(row.BusinessSystemID),
					ServiceID:        nullableInt64Ptr(row.ServiceID),
					InstanceMode:     row.InstanceMode.String,
					ParamValues:      row.ParamValues,
				})
			}
		}
	}
	mergeTaskInput(&state, input)
	message, validationErr := handler.validateTask(context, &state, id == 0 || input.Group != nil || input.Groups != nil, id)
	if validationErr != nil {
		response.Error(context, validationErr)
		return
	}
	if message != "" {
		response.BusinessError(context, 400, message, nil)
		return
	}
	nextRun := nextRunTime(state.CronExpression, state.Enabled)
	var err error
	if id == 0 {
		result, execErr := handler.db.ExecContext(context, `INSERT INTO inspection_task(name,inspection_name,group_id,concurrency,timeout_seconds,cron_expression,next_run_time,last_run_time,enabled,create_time,update_time) VALUES(?,?,?,?,?,?,?,NULL,?,NOW(),NOW())`, state.Name, state.InspectionName, state.GroupIDs[0], state.Concurrency, state.TimeoutSeconds, state.CronExpression, nextRun, state.Enabled)
		if execErr != nil {
			response.Error(context, execErr)
			return
		}
		id, err = result.LastInsertId()
	} else {
		_, err = handler.db.ExecContext(context, `UPDATE inspection_task SET name=?,inspection_name=?,group_id=?,concurrency=?,timeout_seconds=?,cron_expression=?,next_run_time=?,enabled=?,update_time=NOW() WHERE id=?`, state.Name, state.InspectionName, state.GroupIDs[0], state.Concurrency, state.TimeoutSeconds, state.CronExpression, nextRun, state.Enabled, id)
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if err = handler.saveTaskGroups(context, id, state.Bindings); err != nil {
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

// saveTaskGroups 全量重建任务↔巡检组绑定（含挂载点）。
func (handler *Handler) saveTaskGroups(context *gin.Context, taskID int64, bindings []groupBindingInput) error {
	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	if _, err = transaction.ExecContext(context, `DELETE FROM inspection_task_group WHERE task_id=?`, taskID); err != nil {
		return err
	}
	for _, binding := range bindings {
		if binding.Group == nil {
			continue
		}
		paramValues := binding.ParamValues
		if len(paramValues) == 0 {
			paramValues = json.RawMessage("{}")
		}
		if _, err = transaction.ExecContext(context, `INSERT INTO inspection_task_group(task_id,group_id,mount_type,project_id,environment_id,business_system_id,service_id,instance_mode,param_values) VALUES(?,?,?,?,?,?,?,?,?)`,
			taskID, *binding.Group, binding.MountType, nullableIDPtr(binding.ProjectID), nullableIDPtr(binding.EnvironmentID), nullableIDPtr(binding.BusinessSystemID), nullableIDPtr(binding.ServiceID), instanceModeValue(binding.InstanceMode), paramValues); err != nil {
			return err
		}
	}
	return transaction.Commit()
}

func instanceModeValue(mode string) any {
	if mode == "" {
		return nil
	}
	return mode
}

func nullableInt64Ptr(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
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
	// bindings（组+挂载点对象）优先；groups 兼容对象数组/ID 数组（须带显式挂载）；
	// 都没提交时保留原值。
	switch {
	case input.Bindings != nil:
		state.Bindings = make([]groupBindingInput, 0, len(*input.Bindings))
		for _, binding := range *input.Bindings {
			if binding.Group != nil && *binding.Group > 0 {
				state.Bindings = append(state.Bindings, binding)
			}
		}
		state.GroupIDs = bindingGroupIDs(state.Bindings)
	case input.Groups != nil:
		state.Bindings = parseGroupBindings([]byte(*input.Groups))
		state.GroupIDs = bindingGroupIDs(state.Bindings)
	case input.Group != nil && *input.Group > 0:
		state.Bindings = []groupBindingInput{{Group: input.Group, MountType: mountProject}}
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

func (handler *Handler) validateTask(context *gin.Context, state *taskState, validateGroupAvailability bool, excludeID int64) (string, error) {
	// GroupIDs 从 Bindings 派生（直接构造 taskState 的调用方只填 Bindings 也应通过）。
	if len(state.GroupIDs) == 0 {
		state.GroupIDs = bindingGroupIDs(state.Bindings)
	}
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
	// 单组模型：一个任务只绑一个巡检组（在逐绑定查询前拦截）。
	if len(state.Bindings) > 1 {
		return "一个巡检任务只绑定一个巡检组；通用基线和应用巡检请分别创建任务", nil
	}
	// 任务名称唯一性收窄为"同巡检组内唯一"（迁移 000010 删除全局唯一键）：
	// 列表/执行记录都展示所属组，歧义只发生在同组内；不同组允许复用通用名。
	if len(state.GroupIDs) == 1 {
		var count int
		if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM inspection_task WHERE name=? AND group_id=? AND id<>?`, state.Name, state.GroupIDs[0], excludeID).Scan(&count); err != nil {
			return "", err
		}
		if count > 0 {
			return "同一巡检组下任务名称已存在", nil
		}
	}
	if len(state.GroupIDs) == 0 {
		state.GroupIDs = bindingGroupIDs(state.Bindings)
	}
	if state.Name == "" || len(state.GroupIDs) == 0 {
		return "名称和巡检组不能为空", nil
	}
	// 逐绑定校验：组存在（启用/检查项在显式提交时校验）+ 挂载点规则。
	type groupMeta struct {
		enabled    bool
		checkCount int
	}
	metas := make(map[int64]groupMeta, len(state.Bindings))
	for index := range state.Bindings {
		binding := &state.Bindings[index]
		if binding.Group == nil || *binding.Group <= 0 {
			return "巡检组无效", nil
		}
		var meta groupMeta
		var category string
		err := handler.db.QueryRowContext(context, `SELECT g.enabled,g.category,(SELECT COUNT(*) FROM inspection_check c WHERE c.group_id=g.id AND c.enabled=TRUE) FROM inspection_group g WHERE g.id=?`, *binding.Group).Scan(&meta.enabled, &category, &meta.checkCount)
		if errors.Is(err, sql.ErrNoRows) {
			return "巡检组不存在", nil
		}
		if err != nil {
			return "", err
		}
		metas[*binding.Group] = meta
		if binding.MountType == "" {
			return "挂载范围无效：请通过页面选择巡检对象", nil
		}
		if _, message, validateErr := handler.validateMountBinding(context, binding, category); validateErr != nil {
			return "", validateErr
		} else if message != "" {
			return message, nil
		}
		if message := validateParamValues(binding.ParamValues, category); message != "" {
			return message, nil
		}
		if missing := handler.missingRequiredParams(context, *binding.Group, binding.ParamValues); missing != "" {
			return missing, nil
		}
	}
	// DRF only runs field-level group validation when PATCH explicitly submits the group.
	if validateGroupAvailability {
		for groupID, meta := range metas {
			if !meta.enabled {
				return fmt.Sprintf("巡检组已禁用（组 %d）", groupID), nil
			}
			if meta.checkCount == 0 {
				return fmt.Sprintf("巡检组没有启用的检查项（组 %d）", groupID), nil
			}
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

// parseGroupBindings 解析 groups 输入（对象数组，须带 mount_type）。
func parseGroupBindings(raw []byte) []groupBindingInput {
	bindings := make([]groupBindingInput, 0)
	var objects []json.RawMessage
	if json.Unmarshal(raw, &objects) != nil {
		return bindings
	}
	for _, item := range objects {
		var binding groupBindingInput
		if json.Unmarshal(item, &binding) == nil && binding.Group != nil && *binding.Group > 0 && binding.MountType != "" {
			bindings = append(bindings, binding)
		}
	}
	return bindings
}

func bindingGroupIDs(bindings []groupBindingInput) []int64 {
	ids := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Group != nil && *binding.Group > 0 {
			ids = append(ids, *binding.Group)
		}
	}
	return uniqueInt64s(ids)
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
