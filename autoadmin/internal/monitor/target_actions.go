package monitor

import (
	"autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"

	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/api/targetbatch"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/installhistory"

	"github.com/gin-gonic/gin"
)

var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.@-]+$`)

func (handler *Handler) UpdateTarget(context *gin.Context) {
	id := parseID(context.Param("id"))
	var input map[string]any
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	// 合并 PATCH 字段时持有行锁，避免并发请求用旧快照覆盖彼此未提交的字段。
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	queries := db.New(tx)
	patch := db.UpdateMonitorTargetPatchParams{}
	current, err := queries.GetMonitorTargetPatchForUpdate(context, id)
	if errors.Is(err, sql.ErrNoRows) {
		response.BusinessError(context, 404, "monitor target not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	patch.ExporterType = current.ExporterType
	patch.ScrapePort = current.ScrapePort
	patch.ManagedEnabled = current.ManagedEnabled
	patch.Labels = current.Labels
	patch.Remark = current.Remark
	hasWritableField := false
	for _, field := range []string{"exporter_type", "scrape_port", "managed_enabled", "labels", "remark"} {
		value, exists := input[field]
		if !exists {
			continue
		}
		hasWritableField = true
		if field == "scrape_port" {
			port := intValue(value)
			if port < 1 || port > 65535 {
				response.BusinessError(context, 400, "scrape_port must be between 1 and 65535", nil)
				return
			}
		}
		if field == "exporter_type" && !serviceNamePattern.MatchString(strings.TrimSpace(stringValue(value))) {
			response.BusinessError(context, 400, "invalid exporter_type", nil)
			return
		}
		if field == "labels" {
			encoded, err := json.Marshal(value)
			if err != nil {
				response.BusinessError(context, 400, "labels must be valid JSON", nil)
				return
			}
			value = string(encoded)
		}
		switch field {
		case "exporter_type":
			patch.ExporterType = stringValue(value)
		case "scrape_port":
			patch.ScrapePort = uint32(intValue(value))
		case "managed_enabled":
			patch.ManagedEnabled = boolValue(value)
		case "labels":
			patch.Labels = json.RawMessage(stringValue(value))
		case "remark":
			// 显式提交 null 表示清空该列（三态在这里收敛成 NULL）。
			patch.Remark = sql.NullString{String: stringValue(value), Valid: value != nil}
		}
	}
	if !hasWritableField {
		response.BusinessError(context, 400, "no writable fields", nil)
		return
	}
	patch.UpdateTime, patch.ID = time.Now().UTC(), id
	if err := queries.UpdateMonitorTargetPatch(context, patch); err != nil {
		response.Error(context, err)
		return
	}
	if err := tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	handler.GetTarget(context)
}

func (handler *Handler) BatchCreateTargets(context *gin.Context) {
	var input struct {
		HostIDs      []int64 `json:"host_ids"`
		ExporterType string  `json:"exporter_type"`
		ScrapePort   int64   `json:"scrape_port"`
		InstallNow   bool    `json:"install_now"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.HostIDs) == 0 {
		response.BusinessError(context, 400, "host_ids must be a non-empty array", nil)
		return
	}
	input.ExporterType = strings.TrimSpace(input.ExporterType)
	if !serviceNamePattern.MatchString(input.ExporterType) {
		response.BusinessError(context, 400, "invalid exporter_type", nil)
		return
	}
	queries := db.New(handler.db)
	defaultPort, err := queries.GetExporterPackageDefaultPort(context, input.ExporterType)
	if errors.Is(err, sql.ErrNoRows) {
		response.BusinessError(context, 400, "no enabled exporter package found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if input.ScrapePort == 0 {
		input.ScrapePort = int64(defaultPort)
	}
	if input.ScrapePort < 1 || input.ScrapePort > 65535 {
		response.BusinessError(context, 400, "scrape_port must be between 1 and 65535", nil)
		return
	}
	results := make([]response.BatchResult, 0, len(input.HostIDs))
	for _, hostID := range input.HostIDs {
		identity, identityErr := queries.GetHostTargetIdentity(context, hostID)
		name, ip := identity.InstanceName, identity.Ip
		item, targetID := targetbatch.Create(hostID, name, ip, identity.IsDeletedInCloud, identityErr, func() (int64, bool, error) {
			now := time.Now().UTC()
			return database.InsertIfAbsentResult(queries.CreateMonitorTargetIfAbsent(context, db.CreateMonitorTargetIfAbsentParams{
				CreateTime: now, UpdateTime: now, HostID: hostID, ExporterType: input.ExporterType, ScrapePort: uint32(input.ScrapePort),
			}))
		})
		if item.OK && input.InstallNow {
			row := targetInstallRow{ID: targetID, HostID: hostID, ManagedEnabled: true, ExporterType: input.ExporterType, HostName: name, HostIP: ip}
			jobID, err := handler.dispatchExporterJob(context, row)
			var guard guardFailure
			if errors.As(err, &guard) {
				err = apperror.New(apperror.CodeInvalidArgument, guard.message)
			}
			targetbatch.Install(&item, targetID, jobID, err)
		}
		results = append(results, item)
	}
	targetbatch.Report(context, results, nil)
}

func (handler *Handler) CancelTarget(context *gin.Context) {
	id := parseID(context.Param("id"))
	if err := installhistory.Cancel(context.Request.Context(), handler.db, installhistory.Exporter, id); err != nil {
		response.Error(context, err)
		return
	}
	handler.GetTarget(context)
}

func (handler *Handler) CheckTargetService(context *gin.Context) {
	handler.controlTargetService(context, "status")
}
func (handler *Handler) StartTargetService(context *gin.Context) {
	handler.controlTargetService(context, "start")
}
func (handler *Handler) StopTargetService(context *gin.Context) {
	handler.controlTargetService(context, "stop")
}

func (handler *Handler) controlTargetService(context *gin.Context, action string) {
	id := parseID(context.Param("id"))
	result, err := handler.dispatchTargetServiceControl(context, id, action)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, result)
}

// dispatchTargetServiceControl 是 controlTargetService 与批量接口共用的下发核心，
// 避免批量版本和单台版本的 systemctl 命令拼接逻辑各写一份、后续改一处漏一处。
func (handler *Handler) dispatchTargetServiceControl(context *gin.Context, id int64, action string) (gin.H, error) {
	contextRow, err := db.New(handler.db).GetTargetServiceContext(context, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperror.New(apperror.CodeNotFound, "monitor target not found")
	}
	if err != nil {
		return nil, apperror.WithCause(apperror.ErrInternal, err)
	}
	instanceName, exporterType := contextRow.InstanceName, contextRow.ExporterType
	if handler.gateway == nil || !handler.gateway.IsOnline(instanceName) {
		return nil, apperror.New(apperror.CodeInvalidArgument, "host agent is offline")
	}
	if !serviceNamePattern.MatchString(exporterType) {
		return nil, apperror.New(apperror.CodeInvalidArgument, "invalid exporter service name")
	}
	// agent 内置了 exporter 生命周期动作（直接 systemctl，agent 以 root 运行无需 sudo），
	// 之前误下发 agent 不认识的 "run_shell" 动作，agent 报 unsupported action 却被当成功上报。
	builtinActions := map[string]string{"start": "start_exporter", "stop": "stop_exporter", "status": "check_exporter_status"}
	builtinAction, ok := builtinActions[action]
	if !ok {
		return nil, fmt.Errorf("unsupported service action %q", action)
	}
	params, _ := json.Marshal(gin.H{"service_name": exporterType + ".service"})
	result, err := handler.gateway.Execute(context, instanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("monitor-service-%d", time.Now().UnixNano()), Type: "custom", Action: builtinAction, ParamsJson: string(params), TimeoutSeconds: 30})
	if err != nil {
		return nil, targetbatch.ServiceDispatchError(err)
	}
	return serviceControlResult(result, action, exporterType)
}

// serviceControlResult 判定 agent 的执行结果：Agent 在线且动作已下发时 Execute 的 err 为 nil，
// 但 systemctl 本身可能失败（服务不存在、权限不足等），必须检查状态/退出码，否则会把失败当成功上报。
// 注意 Status 为空也是失败：executor 层直接报错（典型是 agent 版本过旧、不认识动作名）时
// 返回的是零值 JobResult，Status=""、ExitCode=0，只判 "failed" 会被漏过。
func serviceControlResult(result *pb.AutomationExecuteResponse, action, exporterType string) (gin.H, error) {
	return targetbatch.ServiceResult(result, action, exporterType)
}

// targetHostLabel 取一个 monitor_target 对应的主机展示名，批量接口的结果列表要按主机报告成功/失败。
func targetHostLabel(context *gin.Context, pool *sql.DB, id int64) string {
	row, err := db.New(pool).GetTargetHostAddress(context, id)
	if err != nil {
		return fmt.Sprintf("target-%d", id)
	}
	if row.InstanceName.Valid && row.InstanceName.String != "" {
		return row.InstanceName.String
	}
	if row.Ip.Valid && row.Ip.String != "" {
		return row.Ip.String
	}
	return fmt.Sprintf("target-%d", id)
}

func batchTargetIDs(context *gin.Context) ([]int64, bool) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.BusinessError(context, 400, "ids must be a non-empty array", nil)
		return nil, false
	}
	return input.IDs, true
}

func (handler *Handler) BatchDeleteTargets(context *gin.Context) {
	ids, ok := batchTargetIDs(context)
	if !ok {
		return
	}
	results := make([]response.BatchResult, 0, len(ids))
	for _, id := range ids {
		label := targetHostLabel(context, handler.db, id)
		queries := db.New(handler.db)
		affected, err := queries.DeleteMonitorTarget(context, id)
		if err == nil && affected == 0 {
			// 删除条件在写入时判断；补查只用于解释拒绝原因，不据此重试删除。
			state, stateErr := queries.GetMonitorTargetState(context, id)
			switch {
			case stateErr != nil:
				err = stateErr
			case state.ManagedEnabled:
				err = apperror.New(apperror.CodeInvalidArgument, "disable the monitor target before deleting it")
			case state.InstallStatus == "pending":
				err = apperror.New(apperror.CodeInvalidArgument, "wait for the uninstall task to finish before deleting")
			default:
				err = apperror.New(apperror.CodeInvalidArgument, "monitor target state changed; refresh and retry")
			}
		}
		result := response.BatchItem(id, err)
		result.Host = label
		results = append(results, result)
	}
	targetbatch.Report(context, results, nil)
}

func (handler *Handler) BatchStartTargetService(context *gin.Context) {
	handler.batchControlTargetService(context, "start")
}
func (handler *Handler) BatchStopTargetService(context *gin.Context) {
	handler.batchControlTargetService(context, "stop")
}

func (handler *Handler) batchControlTargetService(context *gin.Context, action string) {
	ids, ok := batchTargetIDs(context)
	if !ok {
		return
	}
	results := make([]response.BatchResult, 0, len(ids))
	for _, id := range ids {
		label := targetHostLabel(context, handler.db, id)
		detail, err := handler.dispatchTargetServiceControl(context, id, action)
		item := response.BatchItem(id, err)
		item.Host = label
		if detail != nil {
			item.Detail = detail
		}
		results = append(results, item)
	}
	targetbatch.Report(context, results, nil)
}
