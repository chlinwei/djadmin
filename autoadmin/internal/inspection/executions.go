package inspection

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) ListExecutions(context *gin.Context) {
	page, size := pagination(context)
	clauses, arguments := make([]string, 0), make([]any, 0)
	for _, filter := range []struct {
		query  string
		column string
	}{{"task", "e.task_id"}, {"status", "e.status"}, {"trigger_type", "e.trigger_type"}} {
		if value := strings.TrimSpace(context.Query(filter.query)); value != "" {
			clauses = append(clauses, filter.column+"=?")
			arguments = append(arguments, value)
		}
	}
	if value := strings.TrimSpace(context.Query("start_time")); value != "" {
		clauses = append(clauses, "e.create_time>=?")
		arguments = append(arguments, value)
	}
	if value := strings.TrimSpace(context.Query("end_time")); value != "" {
		clauses = append(clauses, "e.create_time<=?")
		arguments = append(arguments, value)
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM inspection_execution e`+where, arguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT e.id,e.task_id AS task,COALESCE(t.name,'') AS task_name,COALESCE(JSON_UNQUOTE(JSON_EXTRACT(e.service_snapshot,'$.name')),'') AS target_name,e.status,e.trigger_type,e.summary,e.requested_username,e.start_time,e.end_time,e.create_time FROM inspection_execution e LEFT JOIN inspection_task t ON t.id=e.task_id`+where+` ORDER BY e.id DESC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) GetExecution(context *gin.Context) {
	id := parseID(context.Param("id"))
	rows, err := handler.db.QueryContext(context, `SELECT e.id,e.task_id AS task,COALESCE(t.name,'') AS task_name,e.status,e.trigger_type,e.task_snapshot,e.group_snapshot,e.service_snapshot,e.target_snapshot,e.summary,e.requested_username,e.start_time,e.end_time,e.create_time FROM inspection_execution e LEFT JOIN inspection_task t ON t.id=e.task_id WHERE e.id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	if len(items) == 0 {
		response.BusinessError(context, 404, "巡检执行不存在", nil)
		return
	}
	execution := items[0]
	targetRows, err := db.New(handler.db).ListInspectionTargetExecutions(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	targets := make([]inspectionTargetExecutionResponse, 0, len(targetRows))
	for _, row := range targetRows {
		targets = append(targets, inspectionTargetExecutionDTO(row))
	}
	for index := range targets {
		resultRows, resultErr := handler.db.QueryContext(context, `SELECT id,check_key,check_type,name,status,severity,expected_value,actual_value,message FROM inspection_result WHERE target_id=? ORDER BY id`, targets[index].ID)
		if resultErr != nil {
			response.Error(context, resultErr)
			return
		}
		targets[index].Results, err = scanRows(resultRows)
		if err != nil {
			response.Error(context, err)
			return
		}
	}
	execution["targets"] = targets
	response.Success(context, execution)
}

type inspectionTargetExecutionResponse struct {
	ID              int64           `json:"id"`
	Deployment      *int64          `json:"deployment"`
	Host            *int64          `json:"host"`
	TargetName      string          `json:"target_name"`
	HostIDSnapshot  *int32          `json:"host_id_snapshot"`
	HostIPSnapshot  string          `json:"host_ip_snapshot"`
	AgentIDSnapshot string          `json:"agent_id_snapshot"`
	Status          string          `json:"status"`
	Passed          *bool           `json:"passed"`
	ErrorMessage    string          `json:"error_message"`
	RawResult       json.RawMessage `json:"raw_result"`
	StartTime       *time.Time      `json:"start_time"`
	EndTime         *time.Time      `json:"end_time"`
	Results         []gin.H         `json:"results"`
}

func inspectionTargetExecutionDTO(row db.ListInspectionTargetExecutionsRow) inspectionTargetExecutionResponse {
	result := inspectionTargetExecutionResponse{
		ID: row.ID, TargetName: row.TargetName, HostIPSnapshot: row.HostIpSnapshot,
		AgentIDSnapshot: row.AgentIDSnapshot, Status: row.Status, ErrorMessage: row.ErrorMessage,
		Passed: row.Passed, RawResult: row.RawResult, Results: make([]gin.H, 0),
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
