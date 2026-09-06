package inspection

import (
	"database/sql"
	"encoding/json"
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
	ID              int64                     `json:"id"`
	Name            string                    `json:"name"`
	Description     string                    `json:"description"`
	Enabled         bool                      `json:"enabled"`
	Category        string                    `json:"category"`
	Application     *int64                    `json:"application"`
	ApplicationName string                    `json:"application_name"`
	Params          json.RawMessage           `json:"params"`
	CreateTime      time.Time                 `json:"create_time"`
	UpdateTime      time.Time                 `json:"update_time"`
	Checks          []inspectionCheckResponse `json:"checks"`
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
		ID: group.ID, Name: group.Name, Description: group.Description,
		Enabled: group.Enabled, Category: group.Category, Application: nullableInt64(group.Application),
		ApplicationName: group.ApplicationName, Params: emptyJSON(group.Params),
		CreateTime: group.CreateTime, UpdateTime: group.UpdateTime, Checks: checks,
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
			ID: row.ID, Name: row.Name, Description: row.Description,
			Enabled: row.Enabled, Category: row.Category, Application: nullableInt64(row.Application),
			ApplicationName: row.ApplicationName, Params: emptyJSON(row.Params),
			CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Checks: checks,
		})
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

// ---- inspection_task ----

type inspectionTaskResponse struct {
	ID             int64           `json:"id"`
	Name           string          `json:"name"`
	InspectionName string          `json:"inspection_name"`
	Group          int64           `json:"group"`
	GroupName      string          `json:"group_name"`
	Groups         []taskGroupItem `json:"groups"`
	Concurrency    int64           `json:"concurrency"`
	TimeoutSeconds int64           `json:"timeout_seconds"`
	CronExpression string          `json:"cron_expression"`
	NextRunTime    *time.Time      `json:"next_run_time"`
	LastRunTime    *time.Time      `json:"last_run_time"`
	Enabled        bool            `json:"enabled"`
	CreateTime     time.Time       `json:"create_time"`
	UpdateTime     time.Time       `json:"update_time"`
	TargetName     string          `json:"target_name"`
}

func emptyJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("[]")
	}
	return raw
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	return &value.Int64
}

type taskGroupItem struct {
	ID               int64           `json:"id"`
	Name             string          `json:"name"`
	Category         string          `json:"category"`
	MountType        string          `json:"mount_type"`
	ProjectID        *int64          `json:"project_id"`
	EnvironmentID    *int64          `json:"environment_id"`
	BusinessSystemID *int64          `json:"business_system_id"`
	ServiceID        *int64          `json:"service_id"`
	InstanceMode     string          `json:"instance_mode"`
	Params           json.RawMessage `json:"params"`
	ParamValues      json.RawMessage `json:"param_values"`
}

func decodeTaskGroups(raw any) []taskGroupItem {
	var decoded []taskGroupItem
	switch value := raw.(type) {
	case []byte:
		if json.Unmarshal(value, &decoded) != nil {
			return []taskGroupItem{}
		}
	case json.RawMessage:
		if json.Unmarshal(value, &decoded) != nil {
			return []taskGroupItem{}
		}
	case string:
		if json.Unmarshal([]byte(value), &decoded) != nil {
			return []taskGroupItem{}
		}
	default:
		return []taskGroupItem{}
	}
	for index := range decoded {
		if decoded[index].MountType == "" {
			decoded[index].MountType = "legacy_static"
		}
	}
	return decoded
}

func inspectionTaskResponseFrom(row db.ListInspectionTasksTypedRow) inspectionTaskResponse {
	var nextRunTime, lastRunTime *time.Time
	if row.NextRunTime.Valid {
		nextRunTime = &row.NextRunTime.Time
	}
	if row.LastRunTime.Valid {
		lastRunTime = &row.LastRunTime.Time
	}
	return inspectionTaskResponse{
		ID: row.ID, Name: row.Name, InspectionName: row.InspectionName, Group: row.Group, GroupName: row.GroupName,
		Groups:      decodeTaskGroups(row.Groups),
		Concurrency: int64(row.Concurrency), TimeoutSeconds: int64(row.TimeoutSeconds),
		CronExpression: row.CronExpression, NextRunTime: nextRunTime, LastRunTime: lastRunTime, Enabled: row.Enabled,
		CreateTime: row.CreateTime, UpdateTime: row.UpdateTime,
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
