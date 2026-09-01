package assets

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) QueryAgentJobs(context *gin.Context) {
	hostID, _ := strconv.ParseInt(context.Query("host_id"), 10, 64)
	action := strings.TrimSpace(context.Query("action"))
	where := ` WHERE (?=0 OR host_id=?) AND (?='' OR action=?)`
	if context.Query("group_by") == "action" {
		rows, err := handler.service.repository.pool.QueryContext(context, `SELECT action,COUNT(*) FROM assets_agent_job`+where+` GROUP BY action ORDER BY COUNT(*) DESC`, hostID, hostID, action, action)
		if err != nil {
			response.Error(context, err)
			return
		}
		defer rows.Close()
		items := []gin.H{}
		for rows.Next() {
			var name string
			var count int64
			if rows.Scan(&name, &count) == nil {
				items = append(items, gin.H{"action": name, "count": count})
			}
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
	var total int64
	if err := handler.service.repository.pool.QueryRowContext(context, `SELECT COUNT(*) FROM assets_agent_job`+where, hostID, hostID, action, action).Scan(&total); err != nil {
		response.Error(context, err)
		return
	}
	rows, err := handler.service.repository.pool.QueryContext(context, `SELECT job_id,agent_id,host_id,job_type,action,status,timeout_seconds,params,result_data,error_message,exit_code,stdout,stderr,create_time,picked_at,finished_at FROM assets_agent_job`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, hostID, hostID, action, action, size, (page-1)*size)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	items := []gin.H{}
	for rows.Next() {
		var jobID, agentID, jobType, act, status, errorMessage, stdout, stderr string
		var host sql.NullInt64
		var timeout, exit int64
		var paramsRaw, resultRaw []byte
		var created any
		var picked, finished sql.NullTime
		if err = rows.Scan(&jobID, &agentID, &host, &jobType, &act, &status, &timeout, &paramsRaw, &resultRaw, &errorMessage, &exit, &stdout, &stderr, &created, &picked, &finished); err != nil {
			response.Error(context, err)
			return
		}
		items = append(items, gin.H{"job_id": jobID, "agent_id": agentID, "host_id": nullIntValue(host), "type": jobType, "action": act, "status": status, "timeout_seconds": timeout, "params": agentJobJSON(paramsRaw), "result_data": agentJobJSON(resultRaw), "error_message": errorMessage, "exit_code": exit, "stdout": stdout, "stderr": stderr, "create_time": created, "picked_at": nullTimeValue(picked), "finished_at": nullTimeValue(finished)})
	}
	summary := gin.H{}
	statusRows, err := handler.service.repository.pool.QueryContext(context, `SELECT status,COUNT(*) FROM assets_agent_job`+where+` GROUP BY status`, hostID, hostID, action, action)
	if err == nil {
		defer statusRows.Close()
		for statusRows.Next() {
			var name string
			var count int64
			if statusRows.Scan(&name, &count) == nil {
				summary[name] = count
			}
		}
	}
	summary["total"] = total
	totalPages := (total + int64(size) - 1) / int64(size)
	response.Success(context, gin.H{"count": len(items), "pageNumber": page, "pageSize": size, "total": total, "totalPages": totalPages, "results": items, "summary": summary, "recent_failure_reasons": []any{}})
}
func agentJobJSON(raw []byte) any {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return gin.H{}
	}
	return value
}
