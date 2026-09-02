package inspection

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListExecutions(context *gin.Context) {
	page, size := pagination(context)
	queries := db.New(handler.db)
	taskID, status, triggerType := optionalInt64Filter(context, "task"), optionalStringFilter(context, "status"), optionalStringFilter(context, "trigger_type")
	startTime, endTime := optionalTimeFilter(context, "start_time"), optionalTimeFilter(context, "end_time")
	count, err := queries.CountInspectionExecutions(context, db.CountInspectionExecutionsParams{
		TaskID: taskID, Status: status, TriggerType: triggerType, StartTime: startTime, EndTime: endTime,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListInspectionExecutions(context, db.ListInspectionExecutionsParams{
		TaskID: taskID, Status: status, TriggerType: triggerType, StartTime: startTime, EndTime: endTime,
		Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]inspectionExecutionListResponse, 0, len(rows))
	for _, row := range rows {
		var task *int64
		if row.Task.Valid {
			task = &row.Task.Int64
		}
		var startAt, endAt *time.Time
		if row.StartTime.Valid {
			startAt = &row.StartTime.Time
		}
		if row.EndTime.Valid {
			endAt = &row.EndTime.Time
		}
		items = append(items, inspectionExecutionListResponse{
			ID: row.ID, Task: task, TaskName: row.TaskName, TargetName: anyToString(row.TargetName),
			Status: row.Status, TriggerType: row.TriggerType, Summary: row.Summary, RequestedUsername: row.RequestedUsername,
			StartTime: startAt, EndTime: endAt, CreateTime: row.CreateTime,
		})
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

type inspectionExecutionListResponse struct {
	ID                int64           `json:"id"`
	Task              *int64          `json:"task"`
	TaskName          string          `json:"task_name"`
	TargetName        string          `json:"target_name"`
	Status            string          `json:"status"`
	TriggerType       string          `json:"trigger_type"`
	Summary           json.RawMessage `json:"summary"`
	RequestedUsername string          `json:"requested_username"`
	StartTime         *time.Time      `json:"start_time"`
	EndTime           *time.Time      `json:"end_time"`
	CreateTime        time.Time       `json:"create_time"`
}

func anyToString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case []byte:
		return string(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func optionalInt64Filter(context *gin.Context, name string) sql.NullInt64 {
	value := strings.TrimSpace(context.Query(name))
	parsed, err := strconv.ParseInt(value, 10, 64)
	if value == "" || err != nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: parsed, Valid: true}
}
func optionalStringFilter(context *gin.Context, name string) sql.NullString {
	value := strings.TrimSpace(context.Query(name))
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}
func optionalTimeFilter(context *gin.Context, name string) sql.NullTime {
	value := strings.TrimSpace(context.Query(name))
	if value == "" {
		return sql.NullTime{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, parseErr := time.Parse(layout, value); parseErr == nil {
			return sql.NullTime{Time: parsed, Valid: true}
		}
	}
	return sql.NullTime{}
}

type inspectionExecutionDetailResponse struct {
	ID                int64                               `json:"id"`
	Task              *int64                              `json:"task"`
	TaskName          string                              `json:"task_name"`
	Status            string                              `json:"status"`
	TriggerType       string                              `json:"trigger_type"`
	TaskSnapshot      json.RawMessage                     `json:"task_snapshot"`
	GroupSnapshot     json.RawMessage                     `json:"group_snapshot"`
	ServiceSnapshot   json.RawMessage                     `json:"service_snapshot"`
	TargetSnapshot    json.RawMessage                     `json:"target_snapshot"`
	Summary           json.RawMessage                     `json:"summary"`
	RequestedUsername string                              `json:"requested_username"`
	StartTime         *time.Time                          `json:"start_time"`
	EndTime           *time.Time                          `json:"end_time"`
	CreateTime        time.Time                           `json:"create_time"`
	Targets           []inspectionTargetExecutionResponse `json:"targets"`
}

func (handler *Handler) GetExecution(context *gin.Context) {
	id := parseID(context.Param("id"))
	queries := db.New(handler.db)
	execution, err := queries.GetInspectionExecutionTyped(context, id)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "巡检执行不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	targetRows, err := queries.ListInspectionTargetExecutions(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	targets := make([]inspectionTargetExecutionResponse, 0, len(targetRows))
	for _, row := range targetRows {
		targets = append(targets, inspectionTargetExecutionDTO(row))
	}
	for index := range targets {
		resultRows, resultErr := queries.ListInspectionResultsByTarget(context, targets[index].ID)
		if resultErr != nil {
			response.Error(context, resultErr)
			return
		}
		results := make([]inspectionResultResponse, 0, len(resultRows))
		for _, resultRow := range resultRows {
			results = append(results, inspectionResultResponse{
				ID: resultRow.ID, CheckKey: resultRow.CheckKey, CheckType: resultRow.CheckType, Name: resultRow.Name,
				Status: resultRow.Status, Severity: resultRow.Severity, ExpectedValue: resultRow.ExpectedValue,
				ActualValue: resultRow.ActualValue, Message: resultRow.Message,
			})
		}
		targets[index].Results = results
	}
	var task *int64
	if execution.Task.Valid {
		task = &execution.Task.Int64
	}
	var startAt, endAt *time.Time
	if execution.StartTime.Valid {
		startAt = &execution.StartTime.Time
	}
	if execution.EndTime.Valid {
		endAt = &execution.EndTime.Time
	}
	response.Success(context, inspectionExecutionDetailResponse{
		ID: execution.ID, Task: task, TaskName: execution.TaskName, Status: execution.Status, TriggerType: execution.TriggerType,
		TaskSnapshot: execution.TaskSnapshot, GroupSnapshot: execution.GroupSnapshot, ServiceSnapshot: execution.ServiceSnapshot,
		TargetSnapshot: execution.TargetSnapshot, Summary: execution.Summary, RequestedUsername: execution.RequestedUsername,
		StartTime: startAt, EndTime: endAt, CreateTime: execution.CreateTime, Targets: targets,
	})
}

type inspectionResultResponse struct {
	ID            int64           `json:"id"`
	CheckKey      string          `json:"check_key"`
	CheckType     string          `json:"check_type"`
	Name          string          `json:"name"`
	Status        string          `json:"status"`
	Severity      string          `json:"severity"`
	ExpectedValue json.RawMessage `json:"expected_value"`
	ActualValue   json.RawMessage `json:"actual_value"`
	Message       string          `json:"message"`
}

type inspectionTargetExecutionResponse struct {
	ID              int64                      `json:"id"`
	Deployment      *int64                     `json:"deployment"`
	Host            *int64                     `json:"host"`
	TargetName      string                     `json:"target_name"`
	HostIDSnapshot  *int32                     `json:"host_id_snapshot"`
	HostIPSnapshot  string                     `json:"host_ip_snapshot"`
	AgentIDSnapshot string                     `json:"agent_id_snapshot"`
	Status          string                     `json:"status"`
	Passed          *bool                      `json:"passed"`
	ErrorMessage    string                     `json:"error_message"`
	RawResult       json.RawMessage            `json:"raw_result"`
	StartTime       *time.Time                 `json:"start_time"`
	EndTime         *time.Time                 `json:"end_time"`
	Results         []inspectionResultResponse `json:"results"`
}

func inspectionTargetExecutionDTO(row db.ListInspectionTargetExecutionsRow) inspectionTargetExecutionResponse {
	result := inspectionTargetExecutionResponse{
		ID: row.ID, TargetName: row.TargetName, HostIPSnapshot: row.HostIpSnapshot,
		AgentIDSnapshot: row.AgentIDSnapshot, Status: row.Status, ErrorMessage: row.ErrorMessage,
		Passed: row.Passed, RawResult: row.RawResult, Results: make([]inspectionResultResponse, 0),
	}
	if row.Deployment.Valid {
		result.Deployment = &row.Deployment.Int64
	}
	if row.Host.Valid {
		result.Host = &row.Host.Int64
	}
	if row.HostIDSnapshot.Valid {
		result.HostIDSnapshot = &row.HostIDSnapshot.Int32
	}
	if row.StartTime.Valid {
		result.StartTime = &row.StartTime.Time
	}
	if row.EndTime.Valid {
		result.EndTime = &row.EndTime.Time
	}
	return result
}

func (handler *Handler) CancelExecution(context *gin.Context) {
	id := parseID(context.Param("id"))
	transaction, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer transaction.Rollback()
	var status string
	if err = transaction.QueryRowContext(context, `SELECT status FROM inspection_execution WHERE id=? FOR UPDATE`, id).Scan(&status); err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "巡检执行不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	if status != "pending" && status != "running" {
		response.BusinessError(context, 400, "只有等待中或执行中的巡检可以取消", nil)
		return
	}
	// The current Agent protocol has no cancel frame, so persist cancellation atomically and let in-flight responses be ignored by status checks.
	if _, err = transaction.ExecContext(context, `UPDATE inspection_execution SET status='canceled',end_time=NOW(),summary=JSON_SET(COALESCE(summary,JSON_OBJECT()),'$.canceled',TRUE),update_time=NOW() WHERE id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	if _, err = transaction.ExecContext(context, `UPDATE inspection_target_execution SET status='canceled',end_time=NOW(),update_time=NOW() WHERE execution_id=? AND status IN ('pending','running')`, id); err != nil {
		response.Error(context, err)
		return
	}
	if err = transaction.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": id, "status": "canceled"})
}

func nullableID(value any) any {
	if fmt.Sprint(value) == "<nil>" {
		return nil
	}
	return value
}
