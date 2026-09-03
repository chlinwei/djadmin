package inspection

import (
	"encoding/json"
	"fmt"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 这个文件是 groups.go/read_config.go/executions.go 里 scanRows 通用扫描的类型化替代——
// inspection_group/inspection_check/inspection_task 的 enabled 都是 TINYINT(1)，之前
// Scan 进 interface{} 时和 monitor_target 是同一类隐患；现在全部换成 sqlc 类型化查询，
// 从根上不用再靠列名猜。

type inspectionGroupResponse struct {
	ID          int64                     `json:"id"`
	Name        string                    `json:"name"`
	Scope       string                    `json:"scope"`
	Description string                    `json:"description"`
	Enabled     bool                      `json:"enabled"`
	CreateTime  time.Time                 `json:"create_time"`
	UpdateTime  time.Time                 `json:"update_time"`
	Checks      []inspectionCheckResponse `json:"checks"`
}

type inspectionCheckResponse struct {
	ID       int64          `json:"id"`
	Name     string         `json:"name"`
	Executor string         `json:"executor"`
	Config   map[string]any `json:"config"`
	Severity string         `json:"severity"`
	Enabled  bool           `json:"enabled"`
	Order    int64          `json:"order"`
}

func inspectionCheckResponseFrom(row db.ListInspectionChecksByGroupRow) inspectionCheckResponse {
	var config map[string]any
	_ = json.Unmarshal(row.Config, &config)
	if config == nil {
		config = map[string]any{}
	}
	return inspectionCheckResponse{
		ID: row.ID, Name: row.Name, Executor: row.Executor,
		Config: config, Severity: row.Severity, Enabled: row.Enabled, Order: int64(row.Order),
	}
}

func (handler *Handler) loadGroup(context *gin.Context, id int64) (inspectionGroupResponse, error) {
	queries := db.New(handler.db)
	group, err := queries.GetInspectionGroup(context, id)
	if err != nil {
		return inspectionGroupResponse{}, err
	}
	checkRows, err := queries.ListInspectionChecksByGroup(context, id)
	if err != nil {
		return inspectionGroupResponse{}, err
	}
	checks := make([]inspectionCheckResponse, 0, len(checkRows))
	for _, row := range checkRows {
		checks = append(checks, inspectionCheckResponseFrom(row))
	}
	return inspectionGroupResponse{
		ID: group.ID, Name: group.Name, Scope: group.Scope, Description: group.Description,
		Enabled: group.Enabled, CreateTime: group.CreateTime, UpdateTime: group.UpdateTime, Checks: checks,
	}, nil
}

func (handler *Handler) ListGroups(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	pattern := optionalSearchPattern(context)
	count, err := queries.CountInspectionGroups(context, db.CountInspectionGroupsParams{Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListInspectionGroups(context, db.ListInspectionGroupsParams{Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]inspectionGroupResponse, 0, len(rows))
	for _, row := range rows {
		checkRows, checkErr := queries.ListInspectionChecksByGroup(context, row.ID)
		if checkErr != nil {
			response.Error(context, checkErr)
			return
		}
		checks := make([]inspectionCheckResponse, 0, len(checkRows))
		for _, checkRow := range checkRows {
			checks = append(checks, inspectionCheckResponseFrom(checkRow))
		}
		items = append(items, inspectionGroupResponse{
			ID: row.ID, Name: row.Name, Scope: row.Scope, Description: row.Description,
			Enabled: row.Enabled, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Checks: checks,
		})
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

// ---- inspection_task ----

type inspectionTaskResponse struct {
	ID                 int64      `json:"id"`
	Name               string     `json:"name"`
	InspectionName     string     `json:"inspection_name"`
	Group              int64      `json:"group"`
	GroupName          string     `json:"group_name"`
	Scope              string     `json:"scope"`
	LogicalService     *int64     `json:"logical_service"`
	LogicalServiceName string     `json:"logical_service_name"`
	SelectedHostIDs    []int64    `json:"selected_host_ids"`
	Concurrency        int64      `json:"concurrency"`
	TimeoutSeconds     int64      `json:"timeout_seconds"`
	CronExpression     string     `json:"cron_expression"`
	NextRunTime        *time.Time `json:"next_run_time"`
	LastRunTime        *time.Time `json:"last_run_time"`
	Enabled            bool       `json:"enabled"`
	CreateTime         time.Time  `json:"create_time"`
	UpdateTime         time.Time  `json:"update_time"`
	TargetType         string     `json:"target_type"`
	TargetName         string     `json:"target_name"`
}

func decodeInt64Array(raw json.RawMessage) []int64 {
	var decoded []int64
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	return decoded
}

func inspectionTaskResponseFrom(row db.ListInspectionTasksTypedRow) inspectionTaskResponse {
	var logicalService *int64
	if row.LogicalService.Valid {
		logicalService = &row.LogicalService.Int64
	}
	var nextRunTime, lastRunTime *time.Time
	if row.NextRunTime.Valid {
		nextRunTime = &row.NextRunTime.Time
	}
	if row.LastRunTime.Valid {
		lastRunTime = &row.LastRunTime.Time
	}
	hostIDs := decodeInt64Array(row.SelectedHostIds)
	targetType, targetName := "logical_service", row.LogicalServiceName
	if row.Scope == "per_host" {
		targetType = "host_group"
		if len(hostIDs) == 0 {
			targetName = "未选择范围"
		} else {
			targetName = fmt.Sprintf("%d 台主机", len(hostIDs))
		}
	}
	return inspectionTaskResponse{
		ID: row.ID, Name: row.Name, InspectionName: row.InspectionName, Group: row.Group, GroupName: row.GroupName,
		Scope: row.Scope, LogicalService: logicalService, LogicalServiceName: row.LogicalServiceName,
		SelectedHostIDs: hostIDs, Concurrency: int64(row.Concurrency), TimeoutSeconds: int64(row.TimeoutSeconds),
		CronExpression: row.CronExpression, NextRunTime: nextRunTime, LastRunTime: lastRunTime, Enabled: row.Enabled,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, TargetType: targetType, TargetName: targetName,
	}
}

func (handler *Handler) ListTasks(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	pattern := optionalSearchPattern(context)
	count, err := queries.CountInspectionTasks(context, db.CountInspectionTasksParams{Pattern: pattern})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListInspectionTasksTyped(context, db.ListInspectionTasksTypedParams{Pattern: pattern, Limit: int32(size), Offset: int32((page - 1) * size)})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]inspectionTaskResponse, 0, len(rows))
	for _, row := range rows {
		items = append(items, inspectionTaskResponseFrom(row))
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}
