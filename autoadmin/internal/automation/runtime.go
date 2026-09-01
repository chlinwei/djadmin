package automation

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

const controllerKeyPrefix = "go:v1:"

type inventoryInput struct {
	Name               string  `json:"name"`
	SelectedHostIDs    []int64 `json:"selected_host_ids"`
	Enabled            *bool   `json:"enabled"`
	UpdateOnLaunch     *bool   `json:"update_on_launch"`
	UpdateCacheTimeout *int    `json:"update_cache_timeout"`
	Remark             string  `json:"remark"`
}

type taskInput struct {
	Name                    string         `json:"name"`
	PlaybookTemplateID      *int64         `json:"playbook_template"`
	InventoryID             *int64         `json:"inventory"`
	EnvVars                 map[string]any `json:"env_vars"`
	DefaultLimit            string         `json:"default_limit"`
	Enabled                 *bool          `json:"enabled"`
	ExecutionTimeoutSeconds *int           `json:"execution_timeout_seconds"`
	RunAsUser               string         `json:"run_as_user"`
	RunAsGroup              string         `json:"run_as_group"`
	WorkDirectory           string         `json:"work_directory"`
	Remark                  string         `json:"remark"`
}

type hostSnapshot struct {
	HostID      int64  `json:"host_id"`
	HostName    string `json:"host_name"`
	HostIP      string `json:"host_ip"`
	GroupID     *int64 `json:"group_id"`
	GroupName   string `json:"group_name"`
	GroupPath   string `json:"group_path"`
	AgentID     string `json:"-"`
	AgentOnline bool   `json:"agent_online"`
}

func (handler *Handler) CreateInventory(context *gin.Context) { handler.saveInventory(context, 0) }
func (handler *Handler) UpdateInventory(context *gin.Context) {
	id, ok := automationID(context)
	if ok {
		handler.saveInventory(context, id)
	}
}
func (handler *Handler) GetInventory(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	item, err := handler.inventoryByID(context, id)
	if err != nil {
		automationResourceError(context, err)
		return
	}
	response.Success(context, item)
}
func (handler *Handler) DeleteInventory(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	result, err := handler.db.ExecContext(context, `DELETE FROM automation_inventory WHERE id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		automationResourceError(context, sql.ErrNoRows)
		return
	}
	response.Success(context, gin.H{"id": id})
}

func (handler *Handler) saveInventory(context *gin.Context, id int64) {
	var input inventoryInput
	if context.ShouldBindJSON(&input) != nil || strings.TrimSpace(input.Name) == "" {
		automationBadRequest(context, "name is required")
		return
	}
	if input.Enabled == nil {
		enabled := true
		input.Enabled = &enabled
	}
	if input.UpdateOnLaunch == nil {
		value := false
		input.UpdateOnLaunch = &value
	}
	if input.UpdateCacheTimeout == nil {
		value := 300
		input.UpdateCacheTimeout = &value
	}
	if *input.UpdateCacheTimeout < 0 {
		automationBadRequest(context, "update_cache_timeout must be non-negative")
		return
	}
	ids := uniquePositiveIDs(input.SelectedHostIDs)
	now := time.Now().UTC()
	var err error
	if id == 0 {
		result, createErr := handler.db.ExecContext(context, `INSERT INTO automation_inventory(create_time,update_time,remark,name,selected_host_ids,enabled,update_on_launch,update_cache_timeout,last_sync_status,last_sync_message,last_sync_host_count) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, now, now, nullString(input.Remark), strings.TrimSpace(input.Name), marshalJSON(ids), *input.Enabled, *input.UpdateOnLaunch, *input.UpdateCacheTimeout, "never", "", 0)
		err = createErr
		if err == nil {
			id, _ = result.LastInsertId()
		}
	} else {
		_, err = handler.db.ExecContext(context, `UPDATE automation_inventory SET update_time=?,remark=?,name=?,selected_host_ids=?,enabled=?,update_on_launch=?,update_cache_timeout=? WHERE id=?`, now, nullString(input.Remark), strings.TrimSpace(input.Name), marshalJSON(ids), *input.Enabled, *input.UpdateOnLaunch, *input.UpdateCacheTimeout, id)
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	item, err := handler.inventoryByID(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, item)
}

func (handler *Handler) PrecheckInventoryLimit(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	var input struct {
		Limit   string  `json:"limit"`
		HostIDs []int64 `json:"host_ids"`
	}
	if context.Request.ContentLength > 0 && context.ShouldBindJSON(&input) != nil {
		automationBadRequest(context, "request body is invalid")
		return
	}
	inventory, err := handler.inventoryByID(context, id)
	if err != nil {
		automationResourceError(context, err)
		return
	}
	if !boolValue(inventory["enabled"]) {
		response.Success(context, precheckResult(false, "inventory_disabled", "Inventory is disabled", nil, input.Limit))
		return
	}
	ids := uniquePositiveIDs(input.HostIDs)
	if context.Request.ContentLength == 0 || len(input.HostIDs) == 0 {
		ids = intSlice(inventory["selected_host_ids"])
	}
	hosts, err := handler.snapshotHosts(context, ids, strings.TrimSpace(input.Limit))
	if err != nil {
		response.Error(context, err)
		return
	}
	if len(hosts) == 0 {
		response.Success(context, precheckResult(false, "inventory_empty", fmt.Sprintf("Inventory [%s] currently has no matching hosts", stringValue(inventory["name"])), hosts, input.Limit))
		return
	}
	response.Success(context, precheckResult(true, "ok", fmt.Sprintf("Precheck passed; %d hosts matched", len(hosts)), hosts, input.Limit))
}

func (handler *Handler) CreateTask(context *gin.Context) { handler.saveTask(context, 0) }
func (handler *Handler) UpdateTask(context *gin.Context) {
	id, ok := automationID(context)
	if ok {
		handler.saveTask(context, id)
	}
}
func (handler *Handler) GetTask(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	item, err := handler.taskByID(context, id)
	if err != nil {
		automationResourceError(context, err)
		return
	}
	response.Success(context, item)
}
func (handler *Handler) DeleteTask(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	result, err := handler.db.ExecContext(context, `DELETE FROM automation_task WHERE id=?`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		automationResourceError(context, sql.ErrNoRows)
		return
	}
	response.Success(context, gin.H{"id": id})
}

func (handler *Handler) saveTask(context *gin.Context, id int64) {
	var input taskInput
	if context.ShouldBindJSON(&input) != nil {
		automationBadRequest(context, "request body is invalid")
		return
	}
	if id > 0 && input.Enabled != nil && strings.TrimSpace(input.Name) == "" && input.PlaybookTemplateID == nil && strings.TrimSpace(input.RunAsUser) == "" {
		// The list switch sends only enabled; retaining the rest avoids treating it as a form submission.
		if _, err := handler.db.ExecContext(context, `UPDATE automation_task SET enabled=?,update_time=? WHERE id=?`, *input.Enabled, time.Now().UTC(), id); err != nil {
			response.Error(context, err)
			return
		}
		item, err := handler.taskByID(context, id)
		if err != nil {
			automationResourceError(context, err)
			return
		}
		response.Success(context, item)
		return
	}
	fieldErrors := gin.H{}
	if strings.TrimSpace(input.Name) == "" {
		fieldErrors["name"] = "任务名称不能为空"
	}
	if input.PlaybookTemplateID == nil || *input.PlaybookTemplateID < 1 {
		fieldErrors["playbook_template"] = "请选择 Playbook 模板"
	}
	if strings.TrimSpace(input.RunAsUser) == "" {
		fieldErrors["run_as_user"] = "请输入执行用户"
	}
	if len(fieldErrors) > 0 {
		response.BusinessError(context, 400, "任务表单校验失败", fieldErrors)
		return
	}
	if input.InventoryID != nil && *input.InventoryID < 1 {
		automationBadRequest(context, "inventory must be a positive ID when provided")
		return
	}
	if input.EnvVars == nil {
		input.EnvVars = map[string]any{}
	}
	if input.Enabled == nil {
		value := true
		input.Enabled = &value
	}
	if input.ExecutionTimeoutSeconds == nil {
		value := 600
		input.ExecutionTimeoutSeconds = &value
	}
	if *input.ExecutionTimeoutSeconds < 1 || *input.ExecutionTimeoutSeconds > 14400 {
		automationBadRequest(context, "execution_timeout_seconds must be between 1 and 14400")
		return
	}
	if strings.TrimSpace(input.WorkDirectory) == "" {
		input.WorkDirectory = "/tmp"
	}
	var templateExists int
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM automation_playbook_template WHERE id=?`, *input.PlaybookTemplateID).Scan(&templateExists); err != nil {
		response.Error(context, err)
		return
	}
	if templateExists == 0 {
		automationBadRequest(context, "playbook_template does not exist")
		return
	}
	if input.InventoryID != nil {
		var inventoryExists int
		if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM automation_inventory WHERE id=?`, *input.InventoryID).Scan(&inventoryExists); err != nil {
			response.Error(context, err)
			return
		}
		if inventoryExists == 0 {
			automationBadRequest(context, "inventory does not exist")
			return
		}
	}
	now := time.Now().UTC()
	var err error
	if id == 0 {
		result, createErr := handler.db.ExecContext(context, `INSERT INTO automation_task(create_time,update_time,remark,name,playbook_template_id,inventory_id,env_vars,default_limit,enabled,execution_timeout_seconds,run_as_user,run_as_group,work_directory) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`, now, now, nullString(input.Remark), strings.TrimSpace(input.Name), *input.PlaybookTemplateID, nullableID(input.InventoryID), marshalJSON(input.EnvVars), strings.TrimSpace(input.DefaultLimit), *input.Enabled, *input.ExecutionTimeoutSeconds, strings.TrimSpace(input.RunAsUser), strings.TrimSpace(input.RunAsGroup), strings.TrimSpace(input.WorkDirectory))
		err = createErr
		if err == nil {
			id, _ = result.LastInsertId()
		}
	} else {
		_, err = handler.db.ExecContext(context, `UPDATE automation_task SET update_time=?,remark=?,name=?,playbook_template_id=?,inventory_id=?,env_vars=?,default_limit=?,enabled=?,execution_timeout_seconds=?,run_as_user=?,run_as_group=?,work_directory=? WHERE id=?`, now, nullString(input.Remark), strings.TrimSpace(input.Name), *input.PlaybookTemplateID, nullableID(input.InventoryID), marshalJSON(input.EnvVars), strings.TrimSpace(input.DefaultLimit), *input.Enabled, *input.ExecutionTimeoutSeconds, strings.TrimSpace(input.RunAsUser), strings.TrimSpace(input.RunAsGroup), strings.TrimSpace(input.WorkDirectory), id)
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	item, err := handler.taskByID(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, item)
}

func (handler *Handler) PrecheckTask(context *gin.Context) {
	task, input, ok := handler.taskRunInput(context)
	if !ok {
		return
	}
	hosts, status, message, err := handler.precheckTask(context, task, input.HostIDs, input.Limit)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, precheckResult(status == "ok", status, message, hosts, input.Limit))
}
func (handler *Handler) RunTaskNow(context *gin.Context) {
	task, input, ok := handler.taskRunInput(context)
	if !ok {
		return
	}
	hosts, status, message, err := handler.precheckTask(context, task, input.HostIDs, input.Limit)
	if err != nil {
		response.Error(context, err)
		return
	}
	if status != "ok" {
		automationBadRequest(context, message)
		return
	}
	extra := input.ExtraVars
	if extra == nil {
		extra = jsonObject(task["env_vars"])
	}
	jobID, err := handler.createAutomationJob(context, task, hosts, extra, input.Limit, "Job created and queued for execution")
	if err != nil {
		response.Error(context, err)
		return
	}
	if err := handler.runAutomationJob(context.Request.Context(), jobID); err != nil {
		response.Error(context, err)
		return
	}
	job, err := handler.jobByID(context, jobID)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, job)
}

func (handler *Handler) GetJob(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	item, err := handler.jobByID(context, id)
	if err != nil {
		automationResourceError(context, err)
		return
	}
	response.Success(context, item)
}
func (handler *Handler) JobLog(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	job, err := handler.jobByID(context, id)
	if err != nil {
		automationResourceError(context, err)
		return
	}
	rows, err := handler.db.QueryContext(context, `SELECT host_id_snapshot,host_ip_snapshot,status,agent_job_id,stdout,stderr,error_message FROM automation_execution_host_log WHERE job_id=? ORDER BY id`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	var log strings.Builder
	for rows.Next() {
		var hostID sql.NullInt64
		var hostIP, status, agentJobID, stdout, stderr, message string
		if err = rows.Scan(&hostID, &hostIP, &status, &agentJobID, &stdout, &stderr, &message); err != nil {
			response.Error(context, err)
			return
		}
		fmt.Fprintf(&log, "\n\n===== Agent Host #%v (%s) | status=%s | job=%s =====\n%s", nullableInt(hostID), hostIP, status, agentJobID, stdout)
		if stderr != "" {
			fmt.Fprintf(&log, "\n[stderr]\n%s", stderr)
		}
		if message != "" {
			fmt.Fprintf(&log, "\n[error]\n%s", message)
		}
	}
	if err = rows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"job_id": id, "status": job["status"], "job_output": log.String()})
}
func (handler *Handler) JobEvents(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	if _, err := handler.jobByID(context, id); err != nil {
		automationResourceError(context, err)
		return
	}
	response.Success(context, []gin.H{})
}
func (handler *Handler) JobStatusSummary(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	job, err := handler.jobByID(context, id)
	if err != nil {
		automationResourceError(context, err)
		return
	}
	snapshot := jsonObject(job["inventory_snapshot"])
	total := len(jsonArray(snapshot["hosts"]))
	counts := gin.H{"pending": 0, "running": 0, "success": 0, "failed": 0, "skipped": 0, "unreachable": 0}
	switch stringValue(job["status"]) {
	case "pending", "running":
		counts["pending"] = total
	case "success":
		counts["success"] = total
	case "failed":
		counts["failed"] = total
	case "cancelled":
		counts["skipped"] = total
	}
	response.Success(context, gin.H{"job_id": id, "job_status": job["status"], "total_hosts": total, "finished_hosts": total - counts["pending"].(int) - counts["running"].(int), "pending": counts["pending"], "running": counts["running"], "success": counts["success"], "failed": counts["failed"], "skipped": counts["skipped"], "unreachable": counts["unreachable"]})
}
func (handler *Handler) CancelJob(context *gin.Context) {
	id, ok := automationID(context)
	if !ok {
		return
	}
	now := time.Now().UTC()
	result, err := handler.db.ExecContext(context, `UPDATE automation_execution_job SET status='cancelled',start_time=COALESCE(start_time,?),end_time=?,duration_seconds=TIMESTAMPDIFF(MICROSECOND,COALESCE(start_time,?),?)/1000000,result_summary=?,update_time=? WHERE id=? AND status IN ('pending','running')`, now, now, now, now, marshalJSON(gin.H{"message": "Cancelled by user"}), now, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	changed, _ := result.RowsAffected()
	if changed == 0 {
		automationBadRequest(context, "Job is already finished")
		return
	}
	job, err := handler.jobByID(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, job)
}

func (handler *Handler) taskRunInput(context *gin.Context) (gin.H, struct {
	HostIDs   []int64        `json:"host_ids"`
	Limit     string         `json:"limit"`
	ExtraVars map[string]any `json:"extra_vars"`
}, bool) {
	var input struct {
		HostIDs   []int64        `json:"host_ids"`
		Limit     string         `json:"limit"`
		ExtraVars map[string]any `json:"extra_vars"`
	}
	id, ok := automationID(context)
	if !ok {
		return nil, input, false
	}
	if context.Request.ContentLength > 0 && context.ShouldBindJSON(&input) != nil {
		automationBadRequest(context, "request body is invalid")
		return nil, input, false
	}
	task, err := handler.taskByID(context, id)
	if err != nil {
		automationResourceError(context, err)
		return nil, input, false
	}
	if strings.TrimSpace(input.Limit) == "" {
		input.Limit = stringValue(task["default_limit"])
	}
	return task, input, true
}
func (handler *Handler) precheckTask(context *gin.Context, task gin.H, requested []int64, limit string) ([]hostSnapshot, string, string, error) {
	if !boolValue(task["enabled"]) {
		return nil, "task_disabled", "Task is disabled", nil
	}
	inventoryID, ok := jsonID(task["inventory_id"])
	if !ok {
		return nil, "inventory_missing", "Task has no Inventory", nil
	}
	inventory, err := handler.inventoryByID(context, inventoryID)
	if err != nil {
		return nil, "inventory_missing", "Inventory is missing", nil
	}
	if !boolValue(inventory["enabled"]) {
		return nil, "inventory_disabled", "Inventory is disabled", nil
	}
	ids := uniquePositiveIDs(requested)
	if len(ids) == 0 {
		ids = intSlice(inventory["selected_host_ids"])
	}
	hosts, err := handler.snapshotHosts(context, ids, limit)
	if err != nil {
		return nil, "", "", err
	}
	if len(hosts) == 0 {
		return hosts, "inventory_empty", fmt.Sprintf("Inventory [%s] currently has no matching hosts", stringValue(inventory["name"])), nil
	}
	offline := 0
	for _, host := range hosts {
		if !host.AgentOnline {
			offline++
		}
	}
	if offline > 0 {
		return hosts, "has_offline_hosts", fmt.Sprintf("%d target hosts have an offline Agent", offline), nil
	}
	return hosts, "ok", fmt.Sprintf("Precheck passed; %d hosts matched", len(hosts)), nil
}

func (handler *Handler) snapshotHosts(ctx context.Context, ids []int64, limit string) ([]hostSnapshot, error) {
	ids = uniquePositiveIDs(ids)
	if len(ids) == 0 {
		return []hostSnapshot{}, nil
	}
	marks := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, len(ids))
	for index, id := range ids {
		args[index] = id
	}
	rows, err := handler.db.QueryContext(ctx, `SELECT h.id,COALESCE(h.instance_name,''),h.ip,h.group_id,COALESCE(g.name,''),COALESCE(h.agent_id,''),h.agent_online FROM assets_host h LEFT JOIN assets_hostgroup g ON g.id=h.group_id WHERE h.id IN (`+marks+`) AND h.ip IS NOT NULL ORDER BY h.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hosts := []hostSnapshot{}
	for rows.Next() {
		var host hostSnapshot
		var groupID sql.NullInt64
		if err = rows.Scan(&host.HostID, &host.HostName, &host.HostIP, &groupID, &host.GroupName, &host.AgentID, &host.AgentOnline); err != nil {
			return nil, err
		}
		if groupID.Valid {
			value := groupID.Int64
			host.GroupID = &value
		}
		// Database online flags can lag after a stream closes; execution eligibility
		// must use the live Gateway session so the confirmation precheck is accurate.
		host.AgentOnline = handler.gateway != nil && handler.gateway.IsOnline(host.AgentID)
		hosts = append(hosts, host)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return applyAnsibleLimit(hosts, limit), nil
}
func applyAnsibleLimit(hosts []hostSnapshot, limit string) []hostSnapshot {
	include, exclude := []string{}, []string{}
	for _, token := range strings.FieldsFunc(strings.TrimSpace(limit), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
		if strings.HasPrefix(token, "!") && len(token) > 1 {
			exclude = append(exclude, token[1:])
		} else {
			include = append(include, token)
		}
	}
	if len(include) == 0 && len(exclude) == 0 {
		return hosts
	}
	matched := make([]hostSnapshot, 0, len(hosts))
	for _, host := range hosts {
		allowed := len(include) == 0
		for _, token := range include {
			allowed = allowed || matchLimit(host, token)
		}
		for _, token := range exclude {
			if matchLimit(host, token) {
				allowed = false
			}
		}
		if allowed {
			matched = append(matched, host)
		}
	}
	return matched
}
func matchLimit(host hostSnapshot, token string) bool {
	scope, pattern, hasScope := "", strings.ToLower(strings.TrimSpace(token)), false
	if before, after, found := strings.Cut(pattern, ":"); found {
		scope, pattern, hasScope = before, after, true
	}
	wildcard := func(value string) bool { ok, _ := filepath.Match(pattern, strings.ToLower(value)); return ok }
	switch scope {
	case "host", "hostname", "name":
		return wildcard(host.HostName)
	case "id", "host_id":
		return wildcard(strconv.FormatInt(host.HostID, 10))
	case "path", "group_path":
		return wildcard(host.GroupPath)
	}
	return !hasScope && (wildcard(strconv.FormatInt(host.HostID, 10)) || wildcard(host.HostIP))
}

func (handler *Handler) createAutomationJob(ctx context.Context, task gin.H, hosts []hostSnapshot, extra map[string]any, limit, message string) (int64, error) {
	now := time.Now().UTC()
	result, err := handler.db.ExecContext(ctx, "INSERT INTO automation_execution_job(create_time,update_time,remark,job_id,task_id,status,trigger_type,inventory_snapshot,task_name_snapshot,template_name_snapshot,template_content_snapshot,extra_vars,`limit`,result_summary,run_as_user_snapshot,run_as_group_snapshot,work_directory_snapshot,requested_user_id,requested_username) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)", now, now, nil, uuid.NewString(), task["id"], "pending", "manual", marshalJSON(gin.H{"selected_host_ids": hostIDs(hosts), "hosts": hosts}), stringValue(task["name"]), stringValue(task["template_name"]), stringValue(task["template_content"]), marshalJSON(extra), strings.TrimSpace(limit), marshalJSON(gin.H{"message": message}), stringValue(task["run_as_user"]), stringValue(task["run_as_group"]), stringValue(task["work_directory"]), nil, "")
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (handler *Handler) runAutomationJob(ctx context.Context, jobID int64) error {
	now := time.Now().UTC()
	claim, err := handler.db.ExecContext(ctx, `UPDATE automation_execution_job SET status='running',start_time=?,result_summary=?,update_time=? WHERE id=? AND status='pending'`, now, marshalJSON(gin.H{"message": "Job is running"}), now, jobID)
	if err != nil {
		return err
	}
	claimed, _ := claim.RowsAffected()
	if claimed == 0 {
		return nil
	}
	job, err := handler.jobByIDContext(ctx, jobID)
	if err != nil {
		return err
	}
	hosts := decodeHostSnapshots(jsonObject(job["inventory_snapshot"])["hosts"])
	if strings.TrimSpace(stringValue(job["template_content_snapshot"])) == "" || len(hosts) == 0 {
		return handler.finishJob(ctx, jobID, now, 1, 0, len(hosts), "Template snapshot is empty or target inventory is empty")
	}
	if err := handler.rehydrateExecutionAgents(ctx, hosts); err != nil {
		return handler.finishJob(ctx, jobID, now, 1, 0, len(hosts), err.Error())
	}
	privateKey, publicKey, err := handler.loadOrCreateControllerKey(ctx)
	if err != nil {
		return handler.finishJob(ctx, jobID, now, 1, 0, len(hosts), err.Error())
	}
	ready, failures := handler.syncControllerKey(ctx, hosts, publicKey)
	if len(ready) == 0 {
		handler.persistTargetFailures(ctx, jobID, failures)
		return handler.finishJob(ctx, jobID, now, 1, 0, len(hosts), "No target agent accepted the controller key")
	}
	output, stderr, code, runErr := executeLocalAnsible(ctx, privateKey, ready, stringValue(job["template_content_snapshot"]), jsonObject(job["extra_vars"]), stringValue(job["run_as_user_snapshot"]), intValue(job["execution_timeout_seconds"], 600))
	handler.persistTargetFailures(ctx, jobID, failures)
	handler.persistTargetResults(ctx, jobID, ready, code, output, stderr, runErr)
	successful := 0
	if code == 0 && runErr == nil {
		successful = len(ready)
	}
	failed := len(hosts) - successful
	message := "Playbook executed successfully by local ansible-playbook"
	if runErr != nil {
		message = runErr.Error()
	} else if code != 0 {
		message = strings.TrimSpace(stderr)
		if message == "" {
			message = "ansible-playbook failed"
		}
	}
	return handler.finishJob(ctx, jobID, now, code, successful, failed, message)
}

// Agent identities are deliberately excluded from the immutable job snapshot,
// so reconnecting Agents can be resolved from the current host record at run time.
func (handler *Handler) rehydrateExecutionAgents(ctx context.Context, hosts []hostSnapshot) error {
	ids := hostIDs(hosts)
	if len(ids) == 0 {
		return nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
	arguments := make([]any, len(ids))
	for index, id := range ids {
		arguments[index] = id
	}
	rows, err := handler.db.QueryContext(ctx, `SELECT id,COALESCE(agent_id,'') FROM assets_host WHERE id IN (`+placeholders+`)`, arguments...)
	if err != nil {
		return err
	}
	defer rows.Close()
	agents := make(map[int64]string, len(hosts))
	for rows.Next() {
		var hostID int64
		var agentID string
		if err = rows.Scan(&hostID, &agentID); err != nil {
			return err
		}
		agents[hostID] = strings.TrimSpace(agentID)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for index := range hosts {
		hosts[index].AgentID = agents[hosts[index].HostID]
	}
	return nil
}

func (handler *Handler) loadOrCreateControllerKey(ctx context.Context) (string, string, error) {
	key, err := controllerEncryptionKey()
	if err != nil {
		return "", "", err
	}
	transaction, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer transaction.Rollback()
	rows, err := transaction.QueryContext(ctx, `SELECT id,public_key,private_key FROM automation_controller_ssh_key FOR UPDATE`)
	if err != nil {
		return "", "", err
	}
	var ids []int64
	var publicKey, encryptedPrivateKey string
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id, &publicKey, &encryptedPrivateKey); err != nil {
			rows.Close()
			return "", "", err
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return "", "", err
	}
	rows.Close()
	if len(ids) == 1 && strings.HasPrefix(encryptedPrivateKey, controllerKeyPrefix) {
		privateKey, decryptErr := decryptControllerKey(key, encryptedPrivateKey)
		if decryptErr == nil {
			if err = transaction.Commit(); err != nil {
				return "", "", err
			}
			return privateKey, publicKey, nil
		}
	}
	private, public, err := generateControllerKey()
	if err != nil {
		return "", "", err
	}
	encrypted, err := encryptControllerKey(key, private)
	if err != nil {
		return "", "", err
	}
	if len(ids) > 0 {
		if _, err = transaction.ExecContext(ctx, `DELETE FROM automation_controller_ssh_key`); err != nil {
			return "", "", err
		}
	}
	now := time.Now().UTC()
	if _, err = transaction.ExecContext(ctx, `INSERT INTO automation_controller_ssh_key(create_time,update_time,remark,public_key,private_key) VALUES(?,?,?,?,?)`, now, now, nil, public, encrypted); err != nil {
		return "", "", err
	}
	if err = transaction.Commit(); err != nil {
		return "", "", err
	}
	return private, public, nil
}
func controllerEncryptionKey() ([]byte, error) {
	configured := strings.TrimSpace(os.Getenv("ASSETS_CREDENTIAL_ENCRYPTION_KEY"))
	if configured != "" {
		key, err := base64.URLEncoding.DecodeString(configured)
		if err != nil || len(key) != 32 {
			return nil, errors.New("ASSETS_CREDENTIAL_ENCRYPTION_KEY must be a URL-safe base64 32-byte key")
		}
		return key, nil
	}
	secret := strings.TrimSpace(os.Getenv("DJANGO_SECRET_KEY"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("JWT_SECRET"))
	}
	if secret == "" {
		return nil, errors.New("ASSETS_CREDENTIAL_ENCRYPTION_KEY or DJANGO_SECRET_KEY is required for controller key encryption")
	}
	digest := sha256.Sum256([]byte(secret))
	return digest[:], nil
}
func generateControllerKey() (string, string, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privateBlock, err := ssh.MarshalPrivateKey(private, "")
	if err != nil {
		return "", "", err
	}
	publicKey, err := ssh.NewPublicKey(public)
	if err != nil {
		return "", "", err
	}
	return string(pem.EncodeToMemory(privateBlock)), strings.TrimSpace(string(ssh.MarshalAuthorizedKey(publicKey))) + " djadmin-automation", nil
}
func encryptControllerKey(key []byte, private string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	box, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, box.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return controllerKeyPrefix + base64.RawURLEncoding.EncodeToString(append(nonce, box.Seal(nil, nonce, []byte(private), nil)...)), nil
}
func decryptControllerKey(key []byte, encrypted string) (string, error) {
	encoded := strings.TrimPrefix(encrypted, controllerKeyPrefix)
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	box, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < box.NonceSize() {
		return "", errors.New("controller key ciphertext is invalid")
	}
	plain, err := box.Open(nil, payload[:box.NonceSize()], payload[box.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
func (handler *Handler) syncControllerKey(ctx context.Context, hosts []hostSnapshot, publicKey string) ([]hostSnapshot, map[int64]string) {
	ready := make([]hostSnapshot, 0, len(hosts))
	failures := map[int64]string{}
	for _, host := range hosts {
		if handler.gateway == nil {
			failures[host.HostID] = "automation agent gateway is unavailable"
			continue
		}
		if strings.TrimSpace(host.AgentID) == "" {
			failures[host.HostID] = "host has no usable agent identity"
			continue
		}
		params, _ := json.Marshal(gin.H{"public_key": publicKey})
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := handler.gateway.Execute(requestCtx, host.AgentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("sync-automation-ssh-key-%d-%d", host.HostID, time.Now().UnixNano()), Type: "custom", Action: "sync_automation_ssh_key", ParamsJson: string(params), TimeoutSeconds: 30})
		cancel()
		if err != nil || result.GetStatus() != "success" {
			if err != nil {
				failures[host.HostID] = err.Error()
			} else {
				failures[host.HostID] = result.GetErrorMessage()
			}
			continue
		}
		ready = append(ready, host)
	}
	return ready, failures
}
func executeLocalAnsible(ctx context.Context, privateKey string, hosts []hostSnapshot, content string, extra map[string]any, runAsUser string, timeoutSeconds int) (string, string, int, error) {
	directory, err := os.MkdirTemp("", "autoadmin-ansible-")
	if err != nil {
		return "", "", -1, err
	}
	defer os.RemoveAll(directory)
	keyPath, playbookPath, inventoryPath := filepath.Join(directory, "controller_key"), filepath.Join(directory, "playbook.yml"), filepath.Join(directory, "inventory.ini")
	if err = os.WriteFile(keyPath, []byte(privateKey), 0600); err != nil {
		return "", "", -1, err
	}
	if err = os.WriteFile(playbookPath, []byte(strings.TrimSpace(content)+"\n"), 0600); err != nil {
		return "", "", -1, err
	}
	lines := []string{"[all]"}
	for _, host := range hosts {
		lines = append(lines, fmt.Sprintf("host_%d ansible_host=%s ansible_user=root ansible_port=22", host.HostID, host.HostIP))
	}
	lines = append(lines, "", "[all:vars]", "ansible_ssh_private_key_file="+keyPath, "ansible_ssh_common_args='-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="+filepath.Join(directory, "known_hosts")+"'")
	if err = os.WriteFile(inventoryPath, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		return "", "", -1, err
	}
	if timeoutSeconds < 1 || timeoutSeconds > 14400 {
		timeoutSeconds = 600
	}
	commandCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, "ansible-playbook", "-i", inventoryPath, "--forks", strconv.Itoa(minimum(10, len(hosts))), playbookPath)
	if len(extra) > 0 {
		value, _ := json.Marshal(extra)
		command.Args = append(command.Args, "--extra-vars", string(value))
	}
	if user := strings.TrimSpace(runAsUser); user != "" && user != "root" {
		command.Args = append(command.Args, "--become", "--become-user", user)
	}
	command.Dir = directory
	var stdout, stderr strings.Builder
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	if commandCtx.Err() == context.DeadlineExceeded {
		return stdout.String(), stderr.String() + "\nPlaybook execution timed out.", 124, commandCtx.Err()
	}
	if err == nil {
		return stdout.String(), stderr.String(), 0, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return stdout.String(), stderr.String(), exitError.ExitCode(), err
	}
	return stdout.String(), stderr.String(), -1, err
}

func (handler *Handler) finishJob(ctx context.Context, id int64, start time.Time, code, succeeded, failed int, message string) error {
	now := time.Now().UTC()
	status := "success"
	if failed > 0 || code != 0 {
		status = "failed"
	}
	_, err := handler.db.ExecContext(ctx, `UPDATE automation_execution_job SET status=?,end_time=?,duration_seconds=TIMESTAMPDIFF(MICROSECOND,?,?)/1000000,result_summary=?,update_time=? WHERE id=? AND status<>'cancelled'`, status, now, start, now, marshalJSON(gin.H{"message": message, "total": succeeded + failed, "success": succeeded, "failed": failed, "rc": code, "execution_mode": "local_ansible"}), now, id)
	return err
}
func (handler *Handler) persistTargetFailures(ctx context.Context, jobID int64, failures map[int64]string) {
	for hostID, message := range failures {
		_, _ = handler.db.ExecContext(ctx, `INSERT INTO automation_execution_host_log(create_time,update_time,remark,job_id,host_id,host_id_snapshot,host_name_snapshot,host_ip_snapshot,agent_job_id,status,exit_code,stdout,stderr,error_message,result_data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, time.Now().UTC(), time.Now().UTC(), nil, jobID, hostID, hostID, "", "", "", "failed", nil, "", "", message, "{}")
	}
}
func (handler *Handler) persistTargetResults(ctx context.Context, jobID int64, hosts []hostSnapshot, code int, stdout, stderr string, runErr error) {
	status := "success"
	if code != 0 || runErr != nil {
		status = "failed"
	}
	for _, host := range hosts {
		message := ""
		if runErr != nil {
			message = runErr.Error()
		}
		_, _ = handler.db.ExecContext(ctx, `INSERT INTO automation_execution_host_log(create_time,update_time,remark,job_id,host_id,host_id_snapshot,host_name_snapshot,host_ip_snapshot,agent_job_id,status,exit_code,stdout,stderr,error_message,result_data) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, time.Now().UTC(), time.Now().UTC(), nil, jobID, host.HostID, host.HostID, host.HostName, host.HostIP, "", status, code, stdout, stderr, message, "{}")
	}
}
func (handler *Handler) inventoryByID(ctx context.Context, id int64) (gin.H, error) {
	rows, err := handler.db.QueryContext(ctx, `SELECT * FROM automation_inventory WHERE id=?`, id)
	if err != nil {
		return nil, err
	}
	items, err := scanAutomationRows(rows)
	if err != nil || len(items) == 0 {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	handler.decorateInventoryContext(ctx, items[0])
	return items[0], nil
}
func (handler *Handler) decorateInventoryContext(ctx context.Context, item gin.H) {
	hostIDs := intSlice(item["selected_host_ids"])
	if len(hostIDs) == 0 {
		item["scope_summary"], item["health_status"], item["resolved_host_count"] = gin.H{"label": "0 groups / 0 hosts", "group_count": 0, "host_count": 0, "is_empty_scope": true}, gin.H{"status": "empty", "label": "Empty", "message": "Inventory has no usable hosts"}, 0
		return
	}
	marks := strings.TrimRight(strings.Repeat("?,", len(hostIDs)), ",")
	args := make([]any, len(hostIDs))
	for index, hostID := range hostIDs {
		args[index] = hostID
	}
	var existing, resolved, groups int
	if err := handler.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(ip IS NOT NULL),0),COUNT(DISTINCT group_id) FROM assets_host WHERE id IN (`+marks+`)`, args...).Scan(&existing, &resolved, &groups); err != nil {
		return
	}
	item["scope_summary"], item["resolved_host_count"] = gin.H{"label": fmt.Sprintf("%d groups / %d hosts", groups, resolved), "group_count": groups, "host_count": resolved, "is_empty_scope": false}, resolved
	if existing < len(hostIDs) {
		item["health_status"] = gin.H{"status": "invalid", "label": "Invalid", "message": "Inventory contains deleted hosts"}
	} else if resolved == 0 {
		item["health_status"] = gin.H{"status": "empty", "label": "Empty", "message": "Inventory has no usable hosts"}
	} else {
		item["health_status"] = gin.H{"status": "healthy", "label": "Healthy", "message": fmt.Sprintf("%d executable hosts", resolved)}
	}
}
func (handler *Handler) taskByID(ctx context.Context, id int64) (gin.H, error) {
	rows, err := handler.db.QueryContext(ctx, `SELECT t.*,p.name AS template_name,p.content AS template_content,COALESCE(i.name,'') AS inventory_name FROM automation_task t JOIN automation_playbook_template p ON p.id=t.playbook_template_id LEFT JOIN automation_inventory i ON i.id=t.inventory_id WHERE t.id=?`, id)
	if err != nil {
		return nil, err
	}
	items, err := scanAutomationRows(rows)
	if err != nil || len(items) == 0 {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	item := items[0]
	item["playbook_template"] = item["playbook_template_id"]
	item["inventory"] = item["inventory_id"]
	return item, nil
}
func (handler *Handler) jobByID(context *gin.Context, id int64) (gin.H, error) {
	return handler.jobByIDContext(context, id)
}
func (handler *Handler) jobByIDContext(ctx context.Context, id int64) (gin.H, error) {
	rows, err := handler.db.QueryContext(ctx, `SELECT j.*,COALESCE(t.execution_timeout_seconds,600) AS execution_timeout_seconds FROM automation_execution_job j LEFT JOIN automation_task t ON t.id=j.task_id WHERE j.id=?`, id)
	if err != nil {
		return nil, err
	}
	items, err := scanAutomationRows(rows)
	if err != nil || len(items) == 0 {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	item := items[0]
	item["job_id"], item["template_name"], item["task_name"] = item["id"], item["template_name_snapshot"], item["task_name_snapshot"]
	return item, nil
}
func automationID(context *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(context.Param("id"), 10, 64)
	if err != nil || id < 1 {
		automationBadRequest(context, "invalid id")
		return 0, false
	}
	return id, true
}
func automationBadRequest(context *gin.Context, message string) {
	response.BusinessError(context, 400, message, nil)
}
func automationResourceError(context *gin.Context, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		response.BusinessError(context, 404, "not found", nil)
		return
	}
	response.Error(context, err)
}
func uniquePositiveIDs(values []int64) []int64 {
	result, seen := make([]int64, 0, len(values)), map[int64]bool{}
	for _, value := range values {
		if value > 0 && !seen[value] {
			result, seen[value] = append(result, value), true
		}
	}
	return result
}
func hostIDs(hosts []hostSnapshot) []int64 {
	result := make([]int64, 0, len(hosts))
	for _, host := range hosts {
		result = append(result, host.HostID)
	}
	return result
}
func decodeHostSnapshots(value any) []hostSnapshot {
	var hosts []hostSnapshot
	_ = json.Unmarshal(marshalJSON(value), &hosts)
	return hosts
}
func precheckResult(ok bool, status, message string, hosts []hostSnapshot, limit string) gin.H {
	preview := make([]gin.H, 0, len(hosts))
	for _, host := range hosts {
		preview = append(preview, gin.H{"host_id": host.HostID, "host_name": host.HostName, "host_ip": host.HostIP, "group_name": host.GroupName, "group_path": host.GroupPath, "agent_online": host.AgentOnline})
	}
	return gin.H{"ok": ok, "status": status, "message": message, "resolved_host_count": len(hosts), "effective_limit": strings.TrimSpace(limit), "matched_hosts_preview": preview, "matched_hosts_preview_total": len(hosts)}
}
func jsonArray(value any) []any { raw, _ := value.([]any); return raw }
func intValue(value any, fallback int) int {
	switch number := value.(type) {
	case int64:
		return int(number)
	case float64:
		return int(number)
	case int:
		return number
	}
	return fallback
}
func boolValue(value any) bool {
	switch value := value.(type) {
	case bool:
		return value
	case int64:
		return value != 0
	case int:
		return value != 0
	case float64:
		return value != 0
	case []byte:
		return string(value) == "1" || strings.EqualFold(string(value), "true")
	case string:
		return value == "1" || strings.EqualFold(value, "true")
	default:
		return false
	}
}
func minimum(first, second int) int {
	if first < second {
		return first
	}
	return second
}
