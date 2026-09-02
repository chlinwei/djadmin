package automation

import (
	"database/sql"
	"encoding/json"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 这些 rowToMap 转换函数是 listRows/scanAutomationRows 通用 map 扫描的替代——保持对外返回的
// gin.H 键名和调用方（inventoryByID/taskByID/jobByID 等约 20 处内部业务逻辑）完全一致，
// 只是数据来源从"SELECT * 反射扫描"换成 sqlc 类型化查询，enabled/update_on_launch 等
// TINYINT(1) 列现在保证是真正的 Go bool，不用再靠 boolValue() 兜底猜类型。

func nullStringAny(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
func nullInt64Any(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}
func nullInt32Any(value sql.NullInt32) any {
	if !value.Valid {
		return nil
	}
	return value.Int32
}
func nullTimeAny(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
func nullFloatAny(value sql.NullFloat64) any {
	if !value.Valid {
		return nil
	}
	return value.Float64
}
func decodeJSONAny(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	var decoded any
	if json.Unmarshal(raw, &decoded) != nil {
		return nil
	}
	return decoded
}

func inventoryRowToMap(row db.AutomationInventory) gin.H {
	return gin.H{
		"id": row.ID, "create_time": row.CreateTime, "update_time": row.UpdateTime,
		"remark": nullStringAny(row.Remark), "name": row.Name,
		"selected_host_ids": decodeJSONAny(row.SelectedHostIds), "enabled": row.Enabled,
		"last_sync_host_count": int64(row.LastSyncHostCount), "last_sync_message": row.LastSyncMessage,
		"last_sync_status": row.LastSyncStatus, "last_sync_time": nullTimeAny(row.LastSyncTime),
		"update_cache_timeout": int64(row.UpdateCacheTimeout), "update_on_launch": row.UpdateOnLaunch,
	}
}

func taskRowToMapFromList(row db.ListTasksTypedRow) gin.H {
	return gin.H{
		"id": row.ID, "create_time": row.CreateTime, "update_time": row.UpdateTime,
		"remark": nullStringAny(row.Remark), "name": row.Name, "env_vars": decodeJSONAny(row.EnvVars),
		"enabled": row.Enabled, "inventory_id": nullInt64Any(row.InventoryID), "default_limit": row.DefaultLimit,
		"execution_timeout_seconds": int64(row.ExecutionTimeoutSeconds), "playbook_template_id": nullInt64Any(row.PlaybookTemplateID),
		"run_as_user": row.RunAsUser, "run_as_group": row.RunAsGroup, "work_directory": row.WorkDirectory,
		"raw_template_name": row.RawTemplateName, "inventory_name": row.InventoryName,
	}
}

func taskRowToMapFromGet(row db.GetTaskTypedRow) gin.H {
	return gin.H{
		"id": row.ID, "create_time": row.CreateTime, "update_time": row.UpdateTime,
		"remark": nullStringAny(row.Remark), "name": row.Name, "env_vars": decodeJSONAny(row.EnvVars),
		"enabled": row.Enabled, "inventory_id": nullInt64Any(row.InventoryID), "default_limit": row.DefaultLimit,
		"execution_timeout_seconds": int64(row.ExecutionTimeoutSeconds), "playbook_template_id": nullInt64Any(row.PlaybookTemplateID),
		"run_as_user": row.RunAsUser, "run_as_group": row.RunAsGroup, "work_directory": row.WorkDirectory,
		"template_name": row.TemplateName, "template_content": row.TemplateContent, "inventory_name": row.InventoryName,
		"playbook_template": nullInt64Any(row.PlaybookTemplateID), "inventory": nullInt64Any(row.InventoryID),
	}
}

func jobRowToMap(row db.AutomationExecutionJob) gin.H {
	return gin.H{
		"id": row.ID, "create_time": row.CreateTime, "update_time": row.UpdateTime, "remark": nullStringAny(row.Remark),
		"job_id": row.JobID, "status": row.Status, "trigger_type": row.TriggerType,
		"inventory_snapshot": decodeJSONAny(row.InventorySnapshot), "extra_vars": decodeJSONAny(row.ExtraVars),
		"result_summary": decodeJSONAny(row.ResultSummary), "requested_user_id": nullInt32Any(row.RequestedUserID),
		"requested_username": row.RequestedUsername, "start_time": nullTimeAny(row.StartTime), "end_time": nullTimeAny(row.EndTime),
		"duration_seconds": nullFloatAny(row.DurationSeconds), "task_id": nullInt64Any(row.TaskID),
		"template_content_snapshot": row.TemplateContentSnapshot, "task_name_snapshot": row.TaskNameSnapshot,
		"template_name_snapshot": row.TemplateNameSnapshot, "limit": row.Limit,
		"run_as_user_snapshot": row.RunAsUserSnapshot, "run_as_group_snapshot": row.RunAsGroupSnapshot,
		"work_directory_snapshot": row.WorkDirectorySnapshot,
	}
}

func jobRowToMapTyped(row db.GetJobTypedRow) gin.H {
	item := jobRowToMap(db.AutomationExecutionJob{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark, JobID: row.JobID,
		Status: row.Status, TriggerType: row.TriggerType, InventorySnapshot: row.InventorySnapshot, ExtraVars: row.ExtraVars,
		ResultSummary: row.ResultSummary, RequestedUserID: row.RequestedUserID, RequestedUsername: row.RequestedUsername,
		StartTime: row.StartTime, EndTime: row.EndTime, DurationSeconds: row.DurationSeconds, TaskID: row.TaskID,
		TemplateContentSnapshot: row.TemplateContentSnapshot, TaskNameSnapshot: row.TaskNameSnapshot,
		TemplateNameSnapshot: row.TemplateNameSnapshot, Limit: row.Limit, RunAsUserSnapshot: row.RunAsUserSnapshot,
		RunAsGroupSnapshot: row.RunAsGroupSnapshot, WorkDirectorySnapshot: row.WorkDirectorySnapshot,
	})
	item["execution_timeout_seconds"] = int64(row.ExecutionTimeoutSeconds)
	return item
}
