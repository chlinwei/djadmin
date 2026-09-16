package assets

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) QueryAgentJobs(context *gin.Context) {
	hostID, _ := strconv.ParseInt(context.Query("host_id"), 10, 64)
	action := strings.TrimSpace(context.Query("action"))
	// 过滤条件：host_id=0 / action='' 表示不过滤 → 传 NULL 走 `narg(...) IS NULL` 分支。
	filter := db.ListAgentJobActionCountsParams{Action: sql.NullString{String: action, Valid: action != ""}}
	if hostID != 0 {
		filter.HostID = sql.NullInt64{Int64: hostID, Valid: true}
	}
	jobQueries := db.New(handler.service.repository.pool)
	if context.Query("group_by") == "action" {
		rows, err := jobQueries.ListAgentJobActionCounts(context, db.ListAgentJobActionCountsParams{
			HostID: filter.HostID, Action: filter.Action,
		})
		if err != nil {
			response.Error(context, err)
			return
		}
		items := make([]gin.H, 0, len(rows))
		for _, row := range rows {
			items = append(items, gin.H{"action": row.Action, "count": row.Total})
		}
		response.Success(context, gin.H{"count": len(items), "results": items})
		return
	}
	page, _ := strconv.Atoi(context.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(context.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	total, err := jobQueries.CountAgentJobs(context, db.CountAgentJobsParams{HostID: filter.HostID, Action: filter.Action})
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := jobQueries.ListAgentJobs(context, db.ListAgentJobsParams{
		HostID: filter.HostID, Action: filter.Action, Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		items = append(items, gin.H{"job_id": row.JobID, "instance_name": row.InstanceName, "host_id": nullIntValue(row.HostID), "type": row.JobType, "action": row.Action, "status": row.Status, "timeout_seconds": row.TimeoutSeconds, "params": agentJobJSON(row.Params), "result_data": agentJobJSON(row.ResultData), "error_message": row.ErrorMessage, "exit_code": row.ExitCode, "stdout": row.Stdout, "stderr": row.Stderr, "create_time": row.CreateTime, "picked_at": nullTimeValue(row.PickedAt), "finished_at": nullTimeValue(row.FinishedAt)})
	}
	summary := gin.H{}
	if statusRows, statusErr := jobQueries.ListAgentJobStatusCounts(context, db.ListAgentJobStatusCountsParams{
		HostID: filter.HostID, Action: filter.Action,
	}); statusErr == nil {
		for _, row := range statusRows {
			summary[row.Status] = row.Total
		}
	}
	summary["total"] = total
	totalPages := (total + int64(size) - 1) / int64(size)
	response.Success(context, gin.H{"count": len(items), "pageNumber": page, "pageSize": size, "total": total, "totalPages": totalPages, "results": items, "summary": summary, "recent_failure_reasons": []any{}})
}
func agentJobJSON(raw json.RawMessage) any {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return gin.H{}
	}
	return value
}
