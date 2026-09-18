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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/automation/ansiblecmd"
	db "autoadmin/internal/platform/database/generated"

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
	Remark             *string `json:"remark"`
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
	HostID    int64  `json:"host_id"`
	HostName  string `json:"host_name"`
	HostIP    string `json:"host_ip"`
	GroupID   *int64 `json:"group_id"`
	GroupName string `json:"group_name"`
	GroupPath string `json:"group_path"`
	// InstanceName 是主机 instance_name（= assets_host.instance_name），同时是
	// gateway 会话的路由 key；仅内部使用，不参与 inventory 对外序列化。
	InstanceName string `json:"-"`
	AgentOnline  bool   `json:"agent_online"`
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

// inventoryExisting 是 inventory 部分更新时从库中读出的现值快照。
type inventoryExisting struct {
	Name               string
	Remark             sql.NullString
	SelectedHostIDs    sql.NullString
	Enabled            bool
	UpdateOnLaunch     bool
	UpdateCacheTimeout sql.NullInt64
}

// resolveInventoryUpdate 实现 PATCH 部分更新合并：未提供的字段保留原值。
// 合并后 name 仍为空（只可能显式传空名）时返回错误。
func resolveInventoryUpdate(existing inventoryExisting, input inventoryInput) (inventoryInput, error) {
	if strings.TrimSpace(input.Name) == "" {
		input.Name = existing.Name
	}
	if input.Remark == nil {
		value := existing.Remark.String
		input.Remark = &value
	}
	if input.SelectedHostIDs == nil {
		input.SelectedHostIDs = decodeJSONInt64Array(existing.SelectedHostIDs.String)
	}
	if input.Enabled == nil {
		input.Enabled = &existing.Enabled
	}
	if input.UpdateOnLaunch == nil {
		input.UpdateOnLaunch = &existing.UpdateOnLaunch
	}
	if input.UpdateCacheTimeout == nil {
		if !existing.UpdateCacheTimeout.Valid {
			input.UpdateCacheTimeout = new(int)
			*input.UpdateCacheTimeout = 300
		} else {
			value := int(existing.UpdateCacheTimeout.Int64)
			input.UpdateCacheTimeout = &value
		}
	}
	if strings.TrimSpace(input.Name) == "" {
		return input, errors.New("name is required")
	}
	return input, nil
}

func (handler *Handler) saveInventory(context *gin.Context, id int64) {
	var input inventoryInput
	if context.ShouldBindJSON(&input) != nil {
		automationBadRequest(context, "invalid request body")
		return
	}
	queries := db.New(handler.db)
	if id != 0 {
		// 部分更新（PATCH）语义：前端状态开关等场景只传部分字段，未提供的字段保留原值。
		row, scanErr := queries.GetInventoryTyped(context, id)
		if scanErr != nil {
			automationResourceError(context, scanErr)
			return
		}
		// 两列都是 NOT NULL，原实现（Scan 进 sql.Null*）恒为 Valid，这里保持一致：
		// 不能把"值为 0"当成"没有值"，否则 update_cache_timeout=0 会被默认值覆盖。
		existing := inventoryExisting{
			Name: row.Name, Remark: row.Remark,
			SelectedHostIDs:    sql.NullString{String: string(row.SelectedHostIds), Valid: true},
			Enabled:            row.Enabled,
			UpdateOnLaunch:     row.UpdateOnLaunch,
			UpdateCacheTimeout: sql.NullInt64{Int64: int64(row.UpdateCacheTimeout), Valid: true},
		}
		merged, mergeErr := resolveInventoryUpdate(existing, input)
		if mergeErr != nil {
			automationBadRequest(context, mergeErr.Error())
			return
		}
		input = merged
	}
	if strings.TrimSpace(input.Name) == "" {
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
		id, err = queries.CreateAutomationInventory(context, db.CreateAutomationInventoryParams{
			CreateTime: now, UpdateTime: now, Remark: nullString(derefString(input.Remark)),
			Name: strings.TrimSpace(input.Name), SelectedHostIds: marshalJSON(ids), Enabled: *input.Enabled,
			UpdateOnLaunch: *input.UpdateOnLaunch, UpdateCacheTimeout: uint32(*input.UpdateCacheTimeout),
		})
	} else {
		err = queries.UpdateAutomationInventory(context, db.UpdateAutomationInventoryParams{
			UpdateTime: now, Remark: nullString(derefString(input.Remark)), Name: strings.TrimSpace(input.Name),
			SelectedHostIds: marshalJSON(ids), Enabled: *input.Enabled, UpdateOnLaunch: *input.UpdateOnLaunch,
			UpdateCacheTimeout: uint32(*input.UpdateCacheTimeout), ID: id,
		})
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
func (handler *Handler) saveTask(context *gin.Context, id int64) {
	var input taskInput
	if context.ShouldBindJSON(&input) != nil {
		automationBadRequest(context, "request body is invalid")
		return
	}
	queries := db.New(handler.db)
	if id > 0 && input.Enabled != nil && strings.TrimSpace(input.Name) == "" && input.PlaybookTemplateID == nil && strings.TrimSpace(input.RunAsUser) == "" {
		// The list switch sends only enabled; retaining the rest avoids treating it as a form submission.
		if err := queries.SetAutomationTaskEnabled(context, db.SetAutomationTaskEnabledParams{
			Enabled: *input.Enabled, UpdateTime: time.Now().UTC(), ID: id,
		}); err != nil {
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
	// 存在性检查复用已有的取行查询：不存在即 ErrNoRows（与原来的 COUNT(*)=0 同义）。
	if _, err := queries.GetAutomationPlaybook(context, *input.PlaybookTemplateID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			automationBadRequest(context, "playbook_template does not exist")
			return
		}
		response.Error(context, err)
		return
	}
	if input.InventoryID != nil {
		if _, err := queries.GetInventoryTyped(context, *input.InventoryID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				automationBadRequest(context, "inventory does not exist")
				return
			}
			response.Error(context, err)
			return
		}
	}
	now := time.Now().UTC()
	var err error
	if id == 0 {
		id, err = queries.CreateAutomationTask(context, db.CreateAutomationTaskParams{
			CreateTime: now, UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name),
			PlaybookTemplateID: sql.NullInt64{Int64: *input.PlaybookTemplateID, Valid: true},
			InventoryID:        nullableID(input.InventoryID), EnvVars: marshalJSON(input.EnvVars),
			DefaultLimit: strings.TrimSpace(input.DefaultLimit), Enabled: *input.Enabled,
			ExecutionTimeoutSeconds: uint32(*input.ExecutionTimeoutSeconds), RunAsUser: strings.TrimSpace(input.RunAsUser),
			RunAsGroup: strings.TrimSpace(input.RunAsGroup), WorkDirectory: strings.TrimSpace(input.WorkDirectory),
		})
	} else {
		err = queries.UpdateAutomationTask(context, db.UpdateAutomationTaskParams{
			UpdateTime: now, Remark: nullString(input.Remark), Name: strings.TrimSpace(input.Name),
			PlaybookTemplateID: sql.NullInt64{Int64: *input.PlaybookTemplateID, Valid: true},
			InventoryID:        nullableID(input.InventoryID), EnvVars: marshalJSON(input.EnvVars),
			DefaultLimit: strings.TrimSpace(input.DefaultLimit), Enabled: *input.Enabled,
			ExecutionTimeoutSeconds: uint32(*input.ExecutionTimeoutSeconds), RunAsUser: strings.TrimSpace(input.RunAsUser),
			RunAsGroup: strings.TrimSpace(input.RunAsGroup), WorkDirectory: strings.TrimSpace(input.WorkDirectory), ID: id,
		})
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
	// 后台执行，**不在请求里等**。多主机作业要跑几分钟，请求一断（浏览器关闭、网关超时、
	// 服务重启）请求的 context 就会被取消：不但会把 ansible 杀掉，还会让收尾写入一起失败，
	// 作业永久停在 running（2026-09-18 作业 #831 即此）。与监控域安装派发同一做法：
	// 脱离请求的 context + 独立 goroutine，前端按作业 id 看状态与日志。
	handler.dispatchJobAsync(jobID)
	job, err := handler.jobByID(context, jobID)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, job)
}

// jobDispatchTimeout 是后台执行的兜底上限。单个作业真正的时限是任务上的
// execution_timeout_seconds（最大 4 小时，见 executeLocalAnsible），这里给足余量，
// 只用于防止 goroutine 因意外永久挂住。
const jobDispatchTimeout = 6 * time.Hour

// dispatchJobAsync 脱离请求 context 在后台执行作业（对应 RunTaskNow 的"立即执行"）。
// 不用请求 context 是刻意的：请求结束/取消不该中断已经派发的作业，更不该让收尾写不进去。
func (handler *Handler) dispatchJobAsync(jobID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), jobDispatchTimeout)
		defer cancel()
		_ = handler.RunJobByID(ctx, jobID)
	}()
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
	rows, err := db.New(handler.db).ListAutomationJobHostLogs(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	var log strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&log, "\n\n===== Agent Host #%v (%s) | status=%s | job=%s =====\n%s", nullableInt32(row.HostIDSnapshot), row.HostIpSnapshot, row.Status, row.AgentJobID, row.Stdout)
		if row.Stderr != "" {
			fmt.Fprintf(&log, "\n[stderr]\n%s", row.Stderr)
		}
		if row.ErrorMessage != "" {
			fmt.Fprintf(&log, "\n[error]\n%s", row.ErrorMessage)
		}
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
	// duration 在应用层算：TIMESTAMPDIFF 是 MySQL 方言函数，PG 侧没有对应写法
	// （分叉清单 §4.2）。start_time 为 NULL 时原语义等价于"现在开始"，即时长 0。
	queries := db.New(handler.db)
	now := time.Now().UTC()
	startTime, startErr := queries.GetAutomationJobStartTime(context, id)
	if startErr != nil && !errors.Is(startErr, sql.ErrNoRows) {
		response.Error(context, startErr)
		return
	}
	effectiveStart := now
	if startTime.Valid {
		effectiveStart = startTime.Time
	}
	changed, err := queries.CancelAutomationJob(context, db.CancelAutomationJobParams{
		StartTime:       sql.NullTime{Time: now, Valid: true},
		EndTime:         sql.NullTime{Time: now, Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: now.Sub(effectiveStart).Seconds(), Valid: true},
		ResultSummary:   marshalJSON(gin.H{"message": "Cancelled by user"}),
		UpdateTime:      now,
		ID:              id,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	if changed == 0 {
		automationBadRequest(context, "Job is already finished")
		return
	}
	// Agent 安装/更新作业另有一套 assets_agent_job 状态；取消 automation 作业时一并收尾，
	// 否则再次安装会被 rejectActiveAgentJobs 的"已有 Agent 任务执行中"一直拦住。
	if cancelErr := queries.CancelAgentJobsByExecution(context, db.CancelAgentJobsByExecutionParams{
		FinishedAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, JobID: id,
	}); cancelErr != nil {
		response.Error(context, cancelErr)
		return
	}
	if cancelErr := queries.CancelAgentJobHostLogsByExecution(context, db.CancelAgentJobHostLogsByExecutionParams{
		UpdateTime: now, JobID: id,
	}); cancelErr != nil {
		response.Error(context, cancelErr)
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
	rows, err := db.New(handler.db).ListAutomationInventoryHosts(ctx, ids)
	if err != nil {
		return nil, err
	}
	hosts := []hostSnapshot{}
	for _, row := range rows {
		// HostName 即 h.instance_name，同时作为 gateway 会话 key（见 hostSnapshot）。
		host := hostSnapshot{HostID: row.ID, HostName: row.InstanceName, HostIP: row.Ip,
			GroupName: row.GroupName, AgentOnline: row.AgentOnline}
		if row.GroupID.Valid {
			value := row.GroupID.Int64
			host.GroupID = &value
		}
		host.InstanceName = host.HostName
		// Database online flags can lag after a stream closes; execution eligibility
		// must use the live Gateway session so the confirmation precheck is accurate.
		host.AgentOnline = handler.gateway != nil && handler.gateway.IsOnline(host.InstanceName)
		hosts = append(hosts, host)
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
	// requested_user_id 保持 NULL：这条派发路径（RunTaskNow / Filebeat 安装）
	// 迁移前就没记发起人。
	return db.New(handler.db).CreateAutomationJob(ctx, db.CreateAutomationJobParams{
		CreateTime:              now,
		UpdateTime:              now,
		JobID:                   uuid.NewString(),
		TaskID:                  nullableTaskID(task["id"]),
		InventorySnapshot:       marshalJSON(gin.H{"selected_host_ids": hostIDs(hosts), "hosts": hosts}),
		TaskNameSnapshot:        stringValue(task["name"]),
		TemplateNameSnapshot:    stringValue(task["template_name"]),
		TemplateContentSnapshot: stringValue(task["template_content"]),
		ExtraVars:               marshalJSON(extra),
		JobLimit:                strings.TrimSpace(limit),
		ResultSummary:           marshalJSON(gin.H{"message": message}),
		RunAsUserSnapshot:       stringValue(task["run_as_user"]),
		RunAsGroupSnapshot:      stringValue(task["run_as_group"]),
		WorkDirectorySnapshot:   stringValue(task["work_directory"]),
		RequestedUsername:       "",
	})
}

// nullableTaskID 把 gin.H 里的 id 转成 sqlc 的 sql.NullInt64 参数（缺值即 NULL）。
func nullableTaskID(value any) sql.NullInt64 {
	if id, ok := jsonID(value); ok {
		return sql.NullInt64{Int64: id, Valid: true}
	}
	return sql.NullInt64{}
}

func (handler *Handler) runAutomationJob(ctx context.Context, jobID int64) error {
	now := time.Now().UTC()
	claimed, err := db.New(handler.db).ClaimAutomationJob(ctx, db.ClaimAutomationJobParams{
		StartTime:     sql.NullTime{Time: now, Valid: true},
		ResultSummary: marshalJSON(gin.H{"message": "Job is running"}),
		UpdateTime:    now,
		ID:            jobID,
	})
	if err != nil {
		return err
	}
	if claimed == 0 {
		return nil
	}
	// 收尾写入一律用**不可取消**的 context：运行中进程收到关闭信号时 ctx 会被取消，
	// 若沿用同一个 ctx，下面这些更新会因 "context canceled" 失败，作业就永久停在 running。
	// 2026-09-18 作业 #831 即此情形（命令已返回、临时目录已被 defer 清掉，但收尾没写进去）。
	persistCtx := persistenceContext(ctx)
	job, err := handler.jobByIDContext(ctx, jobID)
	if err != nil {
		return err
	}
	hosts := decodeHostSnapshots(jsonObject(job["inventory_snapshot"])["hosts"])
	if strings.TrimSpace(stringValue(job["template_content_snapshot"])) == "" || len(hosts) == 0 {
		return handler.finishJob(persistCtx, jobID, now, 1, 0, len(hosts), "Template snapshot is empty or target inventory is empty")
	}
	if err := handler.rehydrateExecutionAgents(ctx, hosts); err != nil {
		return handler.finishJob(persistCtx, jobID, now, 1, 0, len(hosts), err.Error())
	}
	privateKey, publicKey, err := handler.loadOrCreateControllerKey(ctx)
	if err != nil {
		return handler.finishJob(persistCtx, jobID, now, 1, 0, len(hosts), err.Error())
	}
	ready, failures := handler.syncControllerKey(ctx, hosts, publicKey)
	if len(ready) == 0 {
		handler.persistTargetFailures(persistCtx, jobID, failures)
		return handler.finishJob(persistCtx, jobID, now, 1, 0, len(hosts), "No target agent accepted the controller key")
	}
	output, stderr, code, runErr := executeLocalAnsible(ctx, privateKey, ready, stringValue(job["template_content_snapshot"]), jsonObject(job["extra_vars"]), stringValue(job["run_as_user_snapshot"]), intValue(job["execution_timeout_seconds"], 600), handler.jobLogSink(persistCtx, jobID))
	handler.persistTargetFailures(persistCtx, jobID, failures)
	handler.persistTargetResults(persistCtx, jobID, ready, code, output, stderr, runErr)
	successful := 0
	if code == 0 && runErr == nil {
		successful = len(ready)
	}
	failed := len(hosts) - successful
	message := ansibleResultMessage(code, output, stderr, runErr)
	finishErr := handler.finishJob(persistCtx, jobID, now, code, successful, failed, message)
	// 实时输出的块到此使命结束：终态后日志视图改读按主机的结果行，留着只会让同一份
	// ansible 输出重复展示。清理失败不影响作业结论（对账也会兜底清）。
	_ = db.New(handler.db).DeleteAutomationJobLogChunks(persistCtx, jobID)
	return finishErr
}

// Agent identities are deliberately excluded from the immutable job snapshot,
// so reconnecting Agents can be resolved from the current host record at run time.
func (handler *Handler) rehydrateExecutionAgents(ctx context.Context, hosts []hostSnapshot) error {
	ids := hostIDs(hosts)
	if len(ids) == 0 {
		return nil
	}
	rows, err := db.New(handler.db).ListAutomationHostAgentIdentities(ctx, ids)
	if err != nil {
		return err
	}
	agents := make(map[int64]string, len(hosts))
	for _, row := range rows {
		// instance_name 即 gateway 会话 key，回填到 hostSnapshot.
		agents[row.ID] = strings.TrimSpace(row.InstanceName)
	}
	for index := range hosts {
		hosts[index].InstanceName = agents[hosts[index].HostID]
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
	queries := db.New(transaction)
	controllerKeys, err := queries.ListAutomationControllerKeysForUpdate(ctx)
	if err != nil {
		return "", "", err
	}
	var ids []int64
	var publicKey, encryptedPrivateKey string
	for _, row := range controllerKeys {
		ids = append(ids, row.ID)
		publicKey, encryptedPrivateKey = row.PublicKey, row.PrivateKey
	}
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
		if err = queries.DeleteAutomationControllerKeys(ctx); err != nil {
			return "", "", err
		}
	}
	now := time.Now().UTC()
	if err = queries.CreateAutomationControllerKey(ctx, db.CreateAutomationControllerKeyParams{
		CreateTime: now, UpdateTime: now, PublicKey: public, PrivateKey: encrypted,
	}); err != nil {
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
		if strings.TrimSpace(host.InstanceName) == "" {
			failures[host.HostID] = "host has no usable agent identity"
			continue
		}
		params, _ := json.Marshal(gin.H{"public_key": publicKey})
		requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		result, err := handler.gateway.Execute(requestCtx, host.InstanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("sync-automation-ssh-key-%d-%d", host.HostID, time.Now().UnixNano()), Type: "custom", Action: "sync_automation_ssh_key", ParamsJson: string(params), TimeoutSeconds: 30})
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

// executeLocalAnsible 本地执行一趟 ansible。sink 非空时，运行期间的输出会按块交给它
// （用于实时落库，见 liveLogStreamer），作业结束后仍返回完整 stdout/stderr。
func executeLocalAnsible(ctx context.Context, privateKey string, hosts []hostSnapshot, content string, extra map[string]any, runAsUser string, timeoutSeconds int, sink func(string)) (string, string, int, error) {
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
	// 别名用 <主机名>(<IP>)：ansible 会把它原样打印在任务输出与 PLAY RECAP 里，
	// 换成可读标签后，日志里一眼能看出是哪台机器（此前是 host_<id>，无法对应到服务器）。
	usedLabels := map[string]bool{}
	for _, host := range hosts {
		label := inventoryHostLabel(host, usedLabels)
		lines = append(lines, fmt.Sprintf("%s ansible_host=%s ansible_user=root ansible_port=22", label, host.HostIP))
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
	command, err := ansiblecmd.CommandContext(commandCtx, "-i", inventoryPath, "--forks", strconv.Itoa(minimum(10, len(hosts))), playbookPath)
	if err != nil {
		return "", "", -1, err
	}
	isolateProcessGroup(command)
	if len(extra) > 0 {
		value, _ := json.Marshal(extra)
		command.Args = append(command.Args, "--extra-vars", string(value))
	}
	if user := strings.TrimSpace(runAsUser); user != "" && user != "root" {
		command.Args = append(command.Args, "--become", "--become-user", user)
	}
	command.Dir = directory
	var stdout, stderr strings.Builder
	// 用管道而不是直接把 strings.Builder 交给 Cmd：这样能在输出的同时按块推给 sink，
	// 「查看日志」在作业跑完之前就能看到进度，而不是一直"等待新输出"。
	stream := newLiveLogStreamer(sink)
	stdoutPipe, pipeErr := command.StdoutPipe()
	if pipeErr != nil {
		return "", "", -1, pipeErr
	}
	stderrPipe, pipeErr := command.StderrPipe()
	if pipeErr != nil {
		return "", "", -1, pipeErr
	}
	var pumps sync.WaitGroup
	pumps.Add(2)
	go func() { defer pumps.Done(); _, _ = io.Copy(io.MultiWriter(&stdout, stream), stdoutPipe) }()
	go func() { defer pumps.Done(); _, _ = io.Copy(io.MultiWriter(&stderr, stream), stderrPipe) }()
	err = runInProcessGroup(command)
	// 进程（及其进程组）已经收干净，管道随之 EOF，两个 pump 必然结束。
	pumps.Wait()
	stream.Flush()
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

// isolateProcessGroup 把 ansible 放到独立进程组，并在取消/超时时按**整组**杀。
//
// 为什么必须这样：exec.CommandContext 默认只对 controller 进程发 SIGKILL，而 ansible 的
// `--forks N` 会 fork 出多个工作进程（还有它们拉起的 ssh）。只杀 controller 会把工作进程
// 遗弃成孤儿（PPID=1），它们又都卡在"输出管道已无人读取"上永不退出：
// 2026-09-18 现场作业 #831 就留下 8 个这样的孤儿进程，占着到生产主机的 ssh 连接。
//
// WaitDelay 是第二道保险：进程被杀后若还有子进程持有 stdout/stderr 管道，Wait 会一直等 I/O
// 结束；给一个兜底期限保证 Run() 一定会返回，作业能被正常收尾。
func isolateProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return nil
		}
		// 负号 = 整个进程组（组 id 即组长 pid）。
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		return nil
	}
	command.WaitDelay = 10 * time.Second
}

// runInProcessGroup 运行命令，并保证**返回前整组进程都已回收**（配合 isolateProcessGroup 使用）。
//
// 单靠 Cancel 按组杀不够：实测 Wait 返回后进程组仍然存在（ansible fork 出来的工作进程还在，
// 它们持有 ssh 连接并卡在"输出管道无人读取"上）。因此返回前无条件再按组补一刀——
// 正常结束、超时、被杀三种路径都覆盖，且对已经消失的组只是忽略 ESRCH。
func runInProcessGroup(command *exec.Cmd) error {
	if err := command.Start(); err != nil {
		return err
	}
	defer func() {
		if command.Process != nil {
			_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		}
	}()
	err := command.Wait()
	// 命令已退出、但仍有子进程（如挂住的 ssh）持有输出管道时，WaitDelay 到期会返回
	// ErrWaitDelay 覆盖掉"退出码 0"。对作业而言 ansible 跑完就是成功，不能因为一个
	// 残留子进程把作业判失败——这里按真实退出码判定（残留子进程由上面的 defer 清掉）。
	if errors.Is(err, exec.ErrWaitDelay) && command.ProcessState != nil && command.ProcessState.Success() {
		return nil
	}
	return err
}

// persistenceContext 返回"必须落库"的写入用 context：作业一旦进入执行，收尾状态就必须写进去，
// 进程关闭、请求取消都不该阻止它。cancel 只用来中断执行本身，不用来中断结果落库。
func persistenceContext(ctx context.Context) context.Context {
	return context.WithoutCancel(ctx)
}

// liveLogChunkBytes / liveLogChunkInterval 控制实时输出落库的频度：
// 攒够字节数或到间隔就刷一次，避免每个 read 都写库。
const (
	liveLogChunkBytes    = 4096
	liveLogChunkInterval = 1500 * time.Millisecond
)

// liveLogStreamer 是 io.Writer：把运行中的 ansible 输出按块交给 sink 落库。
// 两个 pump goroutine（stdout/stderr）会并发写，因此内部用互斥保护。
// sink 为 nil 时退化为只计数、不落库。
type liveLogStreamer struct {
	sink   func(string)
	mu     sync.Mutex
	buffer strings.Builder
	last   time.Time
}

func newLiveLogStreamer(sink func(string)) *liveLogStreamer {
	return &liveLogStreamer{sink: sink, last: time.Now()}
}

func (streamer *liveLogStreamer) Write(p []byte) (int, error) {
	streamer.mu.Lock()
	defer streamer.mu.Unlock()
	streamer.buffer.Write(p)
	if streamer.sink != nil && (streamer.buffer.Len() >= liveLogChunkBytes || time.Since(streamer.last) >= liveLogChunkInterval) {
		streamer.flushLocked()
	}
	return len(p), nil
}

// Flush 把剩余缓冲写出去（执行结束时调用，保证最后一段不丢）。
func (streamer *liveLogStreamer) Flush() {
	streamer.mu.Lock()
	defer streamer.mu.Unlock()
	streamer.flushLocked()
}

func (streamer *liveLogStreamer) flushLocked() {
	if streamer.buffer.Len() == 0 {
		return
	}
	chunk := streamer.buffer.String()
	streamer.buffer.Reset()
	streamer.last = time.Now()
	if streamer.sink != nil {
		streamer.sink(chunk)
	}
}

// jobLogSink 返回把输出块写进 automation_execution_job_log 的回调（见 AUTOMATION_JOB_EXECUTION.md）。
// 写失败只丢弃该块：实时输出是"尽力而为"的展示，不该影响作业本身。
func (handler *Handler) jobLogSink(ctx context.Context, jobID int64) func(string) {
	return func(chunk string) {
		now := time.Now().UTC()
		_ = db.New(handler.db).InsertAutomationJobLogChunk(ctx, db.InsertAutomationJobLogChunkParams{
			CreateTime: now, UpdateTime: now, JobID: jobID, Content: chunk,
		})
	}
}

// inventoryHostLabel 生成 inventory 里的主机别名：<主机名>(<IP>)。
//
// ansible 的 task 输出与 PLAY RECAP 都用这个别名标识主机，所以它是给人看的：
// 早先用 host_<id>，日志里看不出是哪台服务器。别名必须唯一——同名同 IP 的两条主机记录
// 会被 ansible 并成一台（任务只跑一次），因此撞名时加 #<id> 后缀区分。
func inventoryHostLabel(host hostSnapshot, used map[string]bool) string {
	name := sanitizeInventoryLabel(host.HostName)
	// 名字为空、或含非 ASCII 字符（如中文名，规整后只剩一串下划线加数字）时回落 host-<id>，
	// 免得出现 "__-01(10.0.0.7)" 这种比 ID 更看不懂的别名。ASCII 名（含空格/斜杠）正常规整。
	if !isPureASCII(host.HostName) {
		name = ""
	}
	if name == "" {
		name = fmt.Sprintf("host-%d", host.HostID)
	}
	label := fmt.Sprintf("%s(%s)", name, host.HostIP)
	if used[label] {
		label = fmt.Sprintf("%s#%d", label, host.HostID)
	}
	used[label] = true
	return label
}

// sanitizeInventoryLabel 只保留 INI 主机名安全的字符：空格会把别名拆成 inventory 变量，
// 其余特殊字符在 pattern/分组语法里有语义，统一替换成下划线（中文名等也会被规整掉）。
func sanitizeInventoryLabel(value string) string {
	var builder strings.Builder
	for _, char := range strings.TrimSpace(value) {
		switch {
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			builder.WriteRune(char)
		case char == '.', char == '_', char == '-':
			builder.WriteRune(char)
		default:
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

// isPureASCII 判断名字是否全为 ASCII：非 ASCII（中文等）无法规整成可读别名。
func isPureASCII(value string) bool {
	for _, char := range value {
		if char > 127 {
			return false
		}
	}
	return true
}

func (handler *Handler) finishJob(ctx context.Context, id int64, start time.Time, code, succeeded, failed int, message string) error {
	now := time.Now().UTC()
	status := "success"
	if failed > 0 || code != 0 {
		status = "failed"
	}
	// duration_seconds 在应用层算（TIMESTAMPDIFF 是方言函数，见 CancelJob 的说明）。
	_, err := db.New(handler.db).FinishAutomationJob(ctx, db.FinishAutomationJobParams{
		Status:          status,
		EndTime:         sql.NullTime{Time: now, Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: now.Sub(start).Seconds(), Valid: true},
		ResultSummary: marshalJSON(gin.H{"message": message, "total": succeeded + failed, "success": succeeded,
			"failed": failed, "rc": code, "execution_mode": "local_ansible"}),
		UpdateTime: now,
		ID:         id,
	})
	return err
}
func (handler *Handler) persistTargetFailures(ctx context.Context, jobID int64, failures map[int64]string) {
	queries := db.New(handler.db)
	now := time.Now().UTC()
	for hostID, message := range failures {
		_ = queries.CreateAutomationJobHostLog(ctx, db.CreateAutomationJobHostLogParams{
			CreateTime: now, UpdateTime: now, JobID: jobID,
			HostID:         sql.NullInt64{Int64: hostID, Valid: true},
			HostIDSnapshot: sql.NullInt32{Int32: int32(hostID), Valid: true},
			Status:         "failed", ErrorMessage: message,
		})
	}
}

// ansibleResultMessage 生成作业摘要文案：失败时优先返回 stderr（真正的 ansible 报错），
// 其次是 stdout，最后才回退到 runErr。ansible 退出码非 0 时 runErr 就是 *exec.ExitError，
// 直接取 Error() 只能得到 "exit status 2" 这种无信息量文案，不要用它覆盖 stderr。
func ansibleResultMessage(code int, stdout, stderr string, runErr error) string {
	if code == 0 && runErr == nil {
		return "Playbook executed successfully by local ansible-playbook"
	}
	if message := strings.TrimSpace(stderr); message != "" {
		return message
	}
	if message := strings.TrimSpace(stdout); message != "" {
		return message
	}
	if runErr != nil {
		return runErr.Error()
	}
	return "ansible-playbook failed"
}

func (handler *Handler) persistTargetResults(ctx context.Context, jobID int64, hosts []hostSnapshot, code int, stdout, stderr string, runErr error) {
	status := "success"
	if code != 0 || runErr != nil {
		status = "failed"
	}
	queries := db.New(handler.db)
	now := time.Now().UTC()
	for _, host := range hosts {
		message := ""
		if status == "failed" {
			message = ansibleResultMessage(code, stdout, stderr, runErr)
		}
		_ = queries.CreateAutomationJobHostLog(ctx, db.CreateAutomationJobHostLogParams{
			CreateTime: now, UpdateTime: now, JobID: jobID,
			HostID:           sql.NullInt64{Int64: host.HostID, Valid: true},
			HostIDSnapshot:   sql.NullInt32{Int32: int32(host.HostID), Valid: true},
			HostNameSnapshot: host.HostName, HostIpSnapshot: host.HostIP, Status: status,
			ExitCode: sql.NullInt32{Int32: int32(code), Valid: true},
			Stdout:   stdout, Stderr: stderr, ErrorMessage: message,
		})
	}
}
func (handler *Handler) inventoryByID(ctx context.Context, id int64) (gin.H, error) {
	row, err := db.New(handler.db).GetInventoryTyped(ctx, id)
	if err != nil {
		return nil, err
	}
	item := inventoryRowToMap(row)
	handler.decorateInventoryContext(ctx, item)
	return item, nil
}
func (handler *Handler) decorateInventoryContext(ctx context.Context, item gin.H) {
	hostIDs := intSlice(item["selected_host_ids"])
	if len(hostIDs) == 0 {
		item["scope_summary"], item["health_status"], item["resolved_host_count"] = gin.H{"label": "0 groups / 0 hosts", "group_count": 0, "host_count": 0, "is_empty_scope": true}, gin.H{"status": "empty", "label": "Empty", "message": "Inventory has no usable hosts"}, 0
		return
	}
	counts, err := db.New(handler.db).CountAutomationInventoryHosts(ctx, hostIDs)
	if err != nil {
		return
	}
	existing, resolved, groups := int(counts.Existing), int(counts.Resolved), int(counts.GroupCount)
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
	row, err := db.New(handler.db).GetTaskTyped(ctx, id)
	if err != nil {
		return nil, err
	}
	return taskRowToMapFromGet(row), nil
}
func (handler *Handler) jobByID(context *gin.Context, id int64) (gin.H, error) {
	return handler.jobByIDContext(context, id)
}
func (handler *Handler) jobByIDContext(ctx context.Context, id int64) (gin.H, error) {
	row, err := db.New(handler.db).GetJobTyped(ctx, id)
	if err != nil {
		return nil, err
	}
	item := jobRowToMapTyped(row)
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

// RunJobByID 执行一个已创建（pending）的自动化任务，供其它模块（如 Filebeat 安装派发）
// 复用离线 playbook 执行链路；调用方负责先按 createAutomationJob 的表结构写入任务行。
func (handler *Handler) RunJobByID(ctx context.Context, jobID int64) error {
	return handler.runAutomationJob(ctx, jobID)
}
