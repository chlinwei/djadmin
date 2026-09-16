package monitor

import (
	generated "autoadmin/internal/platform/database/generated"

	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Fluent Bit 纳管目标（monitor_log_collection_target）的运维操作。
// 之前前端调用的 /monitor/log-targets/* 接口后端完全没有实现，点击全部 404；
// 这里补齐：安装/卸载（离线 playbook，与 exporter 安装同链路）、启停/查状态（agent 通用命令）、
// 下发配置（agent 内置 configure_fluent_bit_opensearch 动作）、取消/删除/批量操作。

const fluentBitServiceName = "fluent-bit"

type logTargetRow struct {
	ID             int64
	HostID         int64
	ManagedEnabled bool
	InstallStatus  string
	HostName       string
	HostIP         string
	OSType         string
	OSIDLike       string
	OSVersionID    string
}

func loadLogTarget(ctx context.Context, pool generated.DBTX, id int64) (logTargetRow, error) {
	row, err := generated.New(pool).GetLogTargetForAction(ctx, id)
	if err != nil {
		return logTargetRow{}, err
	}
	return logTargetRow{
		ID: row.ID, HostID: row.HostID, ManagedEnabled: row.ManagedEnabled,
		InstallStatus: row.InstallStatus, HostName: row.InstanceName, HostIP: row.Ip,
		OSType: row.OsType, OSIDLike: row.OsIDLike, OSVersionID: row.OsVersionID,
	}, nil
}

func (row logTargetRow) label() string {
	if row.HostName != "" {
		return row.HostName
	}
	if row.HostIP != "" {
		return row.HostIP
	}
	return fmt.Sprintf("log-target-%d", row.ID)
}

// logTargetPending 取最近一条安装历史，判断是否有任务在执行中（无历史时 pending=false）。
func logTargetPending(ctx context.Context, pool generated.DBTX, id int64) (bool, generated.GetLatestLogTargetInstallHistoryRow, error) {
	history, err := generated.New(pool).GetLatestLogTargetInstallHistory(ctx, sql.NullInt64{Int64: id, Valid: true})
	if err == sql.ErrNoRows {
		return false, generated.GetLatestLogTargetInstallHistoryRow{}, nil
	}
	if err != nil {
		return false, generated.GetLatestLogTargetInstallHistoryRow{}, err
	}
	return history.Status == "pending" || history.Status == "running", history, nil
}

// ---- 安装/卸载（离线 playbook） ----

type fluentBitPackage struct {
	ID          int64
	Family      string
	Major       string
	Format      string
	File        string
	SHA256      string
	PlaybookID  sql.NullInt64
	PlaybookDir string // install 或 uninstall
}

// pickFluentBitPackage 按主机的包格式/发行版/主版本挑一个最合适的 Fluent Bit 软件包。
// 安装与卸载要读不同的 playbook 列，原来是运行时拼列名——现在按角色分派到两条显式语句。
func (handler *Handler) pickFluentBitPackage(ctx context.Context, row logTargetRow, uninstall bool) (*fluentBitPackage, error) {
	queries := generated.New(handler.db)
	candidates := make([]*fluentBitPackage, 0)
	if uninstall {
		rows, err := queries.ListUninstallableFluentBitPackages(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range rows {
			candidates = append(candidates, &fluentBitPackage{
				ID: item.ID, Family: item.PlatformFamily, Major: item.PlatformMajor, Format: item.PackageFormat,
				File: item.File, SHA256: item.Sha256, PlaybookID: item.UninstallPlaybookTemplateID,
			})
		}
	} else {
		rows, err := queries.ListInstallableFluentBitPackages(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range rows {
			candidates = append(candidates, &fluentBitPackage{
				ID: item.ID, Family: item.PlatformFamily, Major: item.PlatformMajor, Format: item.PackageFormat,
				File: item.File, SHA256: item.Sha256, PlaybookID: item.InstallPlaybookTemplateID,
			})
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("没有可用的 Fluent Bit 软件包（需要在软件仓库维护 package_type=fluent_bit 且配置安装 playbook 的启用包）")
	}
	format := hostPackageFormat(row)
	osLine := strings.ToLower(row.OSType + " " + row.OSIDLike)
	best, bestScore := (*fluentBitPackage)(nil), -1
	for _, item := range candidates {
		if item.Format != format {
			continue
		}
		score := 1
		if packageFamilyMatches(item.Family, osLine) {
			score += 2
		}
		if item.Major != "" && strings.HasPrefix(osMajor(row), item.Major) {
			score += 4
		}
		if score > bestScore {
			best, bestScore = item, score
		}
	}
	if best == nil {
		return nil, fmt.Errorf("主机系统 %s/%s 没有匹配的 %s 格式 Fluent Bit 软件包", row.OSType, row.OSIDLike, format)
	}
	return best, nil
}

// hostPackageFormat 依据 agent 采集的系统信息决定离线包格式：debian 系用 deb，其余默认 rpm。
func hostPackageFormat(row logTargetRow) string {
	osLine := strings.ToLower(row.OSType + " " + row.OSIDLike)
	if strings.Contains(osLine, "debian") || strings.Contains(osLine, "ubuntu") {
		return "deb"
	}
	return "rpm"
}

func packageFamilyMatches(family, osLine string) bool {
	family = strings.ToLower(strings.TrimSpace(family))
	if family == "" || family == "any" {
		return true
	}
	if strings.Contains(osLine, family) {
		return true
	}
	if family == "rhel" && (strings.Contains(osLine, "red hat") || strings.Contains(osLine, "centos") || strings.Contains(osLine, "rocky") || strings.Contains(osLine, "almalinux") || strings.Contains(osLine, "fedora")) {
		return true
	}
	return false
}

// osMajor 取 os_version_id 的主版本段（如 "9.4"→"9"、"22.04"→"22"），用于匹配 rhel7/rhel9 这类包目录。
func osMajor(row logTargetRow) string {
	version := strings.TrimSpace(row.OSVersionID)
	if index := strings.Index(version, "."); index > 0 {
		version = version[:index]
	}
	return version
}

func playbookContent(context *gin.Context, pool *sql.DB, playbookID int64) (string, error) {
	// 复用 automation 域已有的取模板查询（内容是唯一需要的列）。
	playbook, err := generated.New(pool).GetAutomationPlaybook(context, playbookID)
	if err != nil {
		return "", err
	}
	return playbook.Content, nil
}

// fluentBitMainConfig 安装时一次性写入的主配置，与 Django render_main_config() 契约一致：
// 开启热重载与本地 HTTP 状态接口（§8.1/§8.5），并挂载 inputs.d/outputs.d 片段目录。
func fluentBitMainConfig() string {
	return "[SERVICE]\n" +
		"    Flush                  5\n" +
		"    Log_Level              info\n" +
		"    Hot_Reload             On\n" +
		"    HTTP_Server            On\n" +
		"    HTTP_Listen            127.0.0.1\n" +
		"    HTTP_Port              2020\n" +
		"    Parsers_File           /etc/fluent-bit/parsers.d/djadmin-multiline.conf\n" +
		"    storage.path           /var/lib/fluent-bit/storage/\n" +
		"\n" +
		"@INCLUDE inputs.d/*.conf\n" +
		"@INCLUDE outputs.d/*.conf\n"
}

func (handler *Handler) dispatchLogTargetInstall(ginContext *gin.Context, row logTargetRow) (gin.H, error) {
	if handler.gateway == nil || !handler.gateway.IsOnline(row.HostName) {
		return nil, fmt.Errorf("host agent is offline")
	}
	pending, _, err := logTargetPending(ginContext, handler.db, row.ID)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, fmt.Errorf("已有安装/卸载任务在执行中，请等待完成或先取消")
	}
	action, desiredStatus := "install", "success"
	if !row.ManagedEnabled {
		action, desiredStatus = "uninstall", "uninstalled"
	}
	item, err := handler.pickFluentBitPackage(ginContext, row, !row.ManagedEnabled)
	if err != nil {
		return nil, err
	}
	content, err := playbookContent(ginContext, handler.db, item.PlaybookID.Int64)
	if err != nil {
		return nil, fmt.Errorf("Fluent Bit %s playbook 不存在: %w", action, err)
	}
	extra := gin.H{"service_name": fluentBitServiceName, "package_format": item.Format, "fluent_bit_main_config": fluentBitMainConfig()}
	packageDirectory := ""
	if action == "install" {
		if strings.TrimSpace(item.File) == "" || strings.TrimSpace(item.SHA256) == "" {
			return nil, fmt.Errorf("选中的 Fluent Bit 软件包缺少离线文件或校验和，请先在软件仓库上传")
		}
		packageDirectory = filepath.Join(handler.packageRoot, filepath.Dir(filepath.FromSlash(item.File)))
		extra["package_file_name"] = filepath.Base(item.File)
		extra["package_sha256"] = item.SHA256
		extra["package_local_directory"] = packageDirectory
	}

	now := time.Now().UTC()
	inventory := gin.H{"selected_host_ids": []int64{row.HostID}, "hosts": []gin.H{{
		"host_id": row.HostID, "host_name": row.HostName, "host_ip": row.HostIP,
		"group_id": nil, "group_name": "", "group_path": "", "agent_online": true,
	}}}
	inventoryJSON, _ := json.Marshal(inventory)
	extraJSON, _ := json.Marshal(extra)
	queries := generated.New(handler.db)
	// 作业行的列集与常量与 exporter 安装一致，直接复用 CreateMonitorTargetJob。
	jobID, err := queries.CreateMonitorTargetJob(ginContext, generated.CreateMonitorTargetJobParams{
		CreateTime: now, UpdateTime: now, JobID: uuid.NewString(),
		InventorySnapshot: inventoryJSON, ExtraVars: extraJSON,
		ResultSummary:        json.RawMessage(`{"message":"Fluent Bit install/uninstall job queued"}`),
		TaskNameSnapshot:     fmt.Sprintf("Fluent Bit %s", action),
		TemplateNameSnapshot: "fluent-bit", TemplateContentSnapshot: content,
		RunAsUserSnapshot: "", RunAsGroupSnapshot: "", WorkDirectorySnapshot: "",
		RequestedUserID: sql.NullInt32{}, RequestedUsername: "system",
	})
	if err != nil {
		return nil, err
	}
	historyID, err := queries.CreateLogTargetInstallHistory(ginContext, generated.CreateLogTargetInstallHistoryParams{
		CreateTime: now, UpdateTime: now, Action: action,
		HostIDSnapshot:       sql.NullInt32{Int32: int32(row.HostID), Valid: true},
		HostNameSnapshot:     row.HostName,
		HostIpSnapshot:       row.HostIP,
		ExporterTypeSnapshot: "fluent_bit",
		SummaryMessage:       "",
		RequestedUserIDSnapshot: sql.NullInt32{}, RequestedUsernameSnapshot: "system",
		HostID:                sql.NullInt64{Int64: row.HostID, Valid: true},
		LogCollectionTargetID: sql.NullInt64{Int64: row.ID, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	if err = queries.MarkLogTargetInstallPending(ginContext, generated.MarkLogTargetInstallPendingParams{
		UpdateTime: now, ID: row.ID,
	}); err != nil {
		return nil, err
	}
	// playbook 可能执行数分钟，异步跑，前端通过列表刷新和安装历史查看进度。
	go func() {
		runContext, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_ = handler.jobs.RunJobByID(runContext, jobID)
		// 任务结束后把 install_status 落成终态，供列表直接展示；安装历史本身由 automation 侧结果快照追溯。
		background := context.Background()
		asyncQueries := generated.New(handler.db)
		finalStatus := "failed"
		var summary, jobStatus string
		if job, scanErr := asyncQueries.GetJobResultSummary(background, jobID); scanErr == nil {
			jobStatus = job.Status
			summary = jobResultMessage(job.ResultSummary)
			if jobStatus == "success" {
				finalStatus = desiredStatus
			}
		}
		if summary == "" {
			summary = "Fluent Bit 任务执行失败"
		}
		message := summary
		if finalStatus == desiredStatus {
			message = ""
		}
		// 时长由应用层算：历史的 create_time 就是上面的 now
		// （原实现用 TIMESTAMPDIFF(MICROSECOND,create_time,?)/1000000）。
		finishedAt := time.Now().UTC()
		succeeded := 0
		if finalStatus == "success" {
			succeeded = 1
		}
		_, _ = asyncQueries.FinishLogTargetInstallState(background, generated.FinishLogTargetInstallStateParams{
			InstallStatus: finalStatus, InstallMessage: message, InstallSucceeded: succeeded,
			UpdateTime: finishedAt, ID: row.ID,
		})
		_, _ = asyncQueries.FinishLogTargetInstallHistory(background, generated.FinishLogTargetInstallHistoryParams{
			Status: finalStatus, SummaryMessage: message,
			EndTime:         sql.NullTime{Time: finishedAt, Valid: true},
			DurationSeconds: sql.NullFloat64{Float64: finishedAt.Sub(now).Seconds(), Valid: true},
			UpdateTime:      finishedAt, ID: historyID,
		})
	}()
	_ = desiredStatus
	return gin.H{"id": row.ID, "action": action, "history_id": historyID, "job_id": jobID}, nil
}

// jobResultMessage 取作业结果摘要里的 message 字段。
// 原实现用 JSON_UNQUOTE(JSON_EXTRACT(result_summary,'$.message'))，是 MySQL 方言函数；
// 改成取回 json 列在应用层解析（与 target 域 target_install.go 同一手法）。
func jobResultMessage(raw json.RawMessage) string {
	var summary struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &summary)
	return strings.TrimSpace(summary.Message)
}

func (handler *Handler) RetryLogTarget(context *gin.Context) {
	id := parseID(context.Param("id"))
	row, err := loadLogTarget(context, handler.db, id)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "log collection target not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	result, err := handler.dispatchLogTargetInstall(context, row)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, result)
}

// ---- 启停/查状态（agent 通用 systemctl 命令） ----

func (handler *Handler) StartLogTargetService(context *gin.Context) {
	handler.controlLogTargetService(context, "start")
}
func (handler *Handler) StopLogTargetService(context *gin.Context) {
	handler.controlLogTargetService(context, "stop")
}
func (handler *Handler) CheckLogTargetService(context *gin.Context) {
	handler.controlLogTargetService(context, "status")
}

func (handler *Handler) controlLogTargetService(context *gin.Context, action string) {
	id := parseID(context.Param("id"))
	result, err := handler.dispatchLogTargetServiceControl(context, id, action)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, result)
}

func (handler *Handler) dispatchLogTargetServiceControl(context *gin.Context, id int64, action string) (gin.H, error) {
	instanceName, err := generated.New(handler.db).GetLogTargetHostName(context, id)
	if err != nil {
		return nil, fmt.Errorf("log collection target not found")
	}
	if handler.gateway == nil || !handler.gateway.IsOnline(instanceName) {
		return nil, fmt.Errorf("host agent is offline")
	}
	// agent 通用命令通道：agent 以 root 运行，直接 systemctl，无需 sudo。
	params, _ := json.Marshal(gin.H{"command": "systemctl", "args": []string{action, fluentBitServiceName + ".service"}})
	result, err := handler.gateway.Execute(context, instanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("fluentbit-service-%d", time.Now().UnixNano()), Type: "command", Action: "fluent_bit_service_control", ParamsJson: string(params), TimeoutSeconds: 30})
	if err != nil {
		return nil, err
	}
	detail, err := serviceControlResult(result, action, fluentBitServiceName)
	if err != nil {
		return nil, err
	}
	handler.persistLogTargetRuntimeStatus(context, id, action, result.ExitCode)
	return detail, nil
}

// persistLogTargetRuntimeStatus 把启停/查状态的真实结果落库，列表里的 Fluent Bit 状态列
// 读的就是这个字段；之前没人更新它，服务停了界面仍显示旧的"running"。
// systemctl status 退出码语义与 exporter 一致：0=运行中，3=已停止，其余=异常。
func (handler *Handler) persistLogTargetRuntimeStatus(context *gin.Context, id int64, action string, exitCode int32) {
	runtimeStatus := "error"
	switch {
	case action == "stop" && exitCode == 0:
		runtimeStatus = "stopped"
	case action == "start" && exitCode == 0:
		runtimeStatus = "running"
	case action == "status":
		switch exitCode {
		case 0:
			runtimeStatus = "running"
		case 3:
			runtimeStatus = "stopped"
		}
	}
	_ = generated.New(handler.db).SetLogTargetRuntimeStatus(context, generated.SetLogTargetRuntimeStatusParams{
		RuntimeStatus: runtimeStatus, UpdateTime: time.Now().UTC(), ID: id,
	})
}

// ---- 下发配置（agent 内置 configure_fluent_bit_opensearch） ----

func (handler *Handler) ApplyLogTargetConfig(context *gin.Context) {
	id := parseID(context.Param("id"))
	cluster, err := generated.New(handler.db).GetLogTargetDefaultCluster(context, id)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 400, "没有已启用的默认 OpenSearch 集群，请先在日志存储里配置", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	instanceName := cluster.InstanceName
	if handler.gateway == nil || !handler.gateway.IsOnline(instanceName) {
		response.BusinessError(context, 400, "host agent is offline", nil)
		return
	}
	host, port, err := firstOpenSearchEndpoint(cluster.Hosts)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	password, err := handler.secrets.Decrypt(cluster.Password)
	if err != nil {
		response.Error(context, err)
		return
	}
	params, _ := json.Marshal(gin.H{"host": host, "port": port, "username": cluster.Username, "password": password})
	result, err := handler.gateway.Execute(context, instanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("fluentbit-apply-%d", time.Now().UnixNano()), Type: "custom", Action: "configure_fluent_bit_opensearch", ParamsJson: string(params), TimeoutSeconds: 60})
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	if result.Status != "success" {
		reason := firstNonEmpty(strings.TrimSpace(result.ErrorMessage), strings.TrimSpace(result.Stderr), strings.TrimSpace(result.Stdout))
		response.BusinessError(context, 400, "Fluent Bit 配置下发失败: "+reason, nil)
		return
	}
	now := time.Now().UTC()
	if err = generated.New(handler.db).MarkLogTargetApplied(context, generated.MarkLogTargetAppliedParams{
		LastAppliedTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: id,
	}); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"skipped": false, "applied_at": now})
}

// firstOpenSearchEndpoint 从集群 hosts 列表（逗号分隔，可带 scheme）取第一个端点。
func firstOpenSearchEndpoint(clusterHosts string) (string, string, error) {
	for _, entry := range strings.Split(clusterHosts, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "://") {
			entry = "http://" + entry
		}
		parsed, err := url.Parse(entry)
		if err != nil {
			continue
		}
		if parsed.Hostname() == "" {
			continue
		}
		port := parsed.Port()
		if port == "" {
			port = "9200"
		}
		return parsed.Hostname(), port, nil
	}
	return "", "", fmt.Errorf("OpenSearch 集群地址无效: %s", clusterHosts)
}

// ---- 取消/删除/批量 ----

func (handler *Handler) CancelLogTarget(context *gin.Context) {
	id := parseID(context.Param("id"))
	pending, history, err := logTargetPending(context, handler.db, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	if !pending {
		response.BusinessError(context, 400, "current task has ended and does not need cancellation", nil)
		return
	}
	now := time.Now().UTC()
	queries := generated.New(handler.db)
	// 时长由应用层算：该流程的历史行 start_time 为 NULL，create_time 就是派发时刻
	//（原实现用 TIMESTAMPDIFF(MICROSECOND,create_time,?)/1000000）。
	if err = queries.CancelInstallHistory(context, generated.CancelInstallHistoryParams{
		EndTime:         sql.NullTime{Time: now, Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: now.Sub(history.CreateTime).Seconds(), Valid: true},
		UpdateTime:      now, ID: history.ID,
	}); err != nil {
		response.Error(context, err)
		return
	}
	if err = queries.MarkLogTargetInstallCancelled(context, generated.MarkLogTargetInstallCancelledParams{
		UpdateTime: now, ID: id,
	}); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": id})
}

func logTargetIDs(context *gin.Context) ([]int64, bool) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.BusinessError(context, 400, "ids must be a non-empty array", nil)
		return nil, false
	}
	return input.IDs, true
}

func (handler *Handler) batchLogTargets(context *gin.Context, label string, run func(logTargetRow) (gin.H, error)) {
	ids, ok := logTargetIDs(context)
	if !ok {
		return
	}
	results := make([]gin.H, 0, len(ids))
	success := 0
	for _, id := range ids {
		row, err := loadLogTarget(context, handler.db, id)
		if err == sql.ErrNoRows {
			results = append(results, gin.H{"id": id, "host": fmt.Sprintf("log-target-%d", id), "ok": false, "message": "log collection target not found"})
			continue
		}
		if err != nil {
			results = append(results, gin.H{"id": id, "host": row.label(), "ok": false, "message": err.Error()})
			continue
		}
		detail, err := run(row)
		if err != nil {
			results = append(results, gin.H{"id": id, "host": row.label(), "ok": false, "message": err.Error()})
			continue
		}
		success++
		results = append(results, gin.H{"id": id, "host": row.label(), "ok": true, "message": "", "detail": detail})
	}
	response.Success(context, gin.H{"total": len(results), "success": success, "failed": len(results) - success, "results": results})
}

func (handler *Handler) BatchRetryLogTargets(context *gin.Context) {
	handler.batchLogTargets(context, "retry", func(row logTargetRow) (gin.H, error) {
		return handler.dispatchLogTargetInstall(context, row)
	})
}

func (handler *Handler) BatchStartLogTargets(context *gin.Context) {
	handler.batchServiceControl(context, "start")
}

func (handler *Handler) BatchStopLogTargets(context *gin.Context) {
	handler.batchServiceControl(context, "stop")
}

func (handler *Handler) BatchApplyLogTargets(context *gin.Context) {
	handler.batchLogTargets(context, "apply", func(row logTargetRow) (gin.H, error) {
		return handler.applyLogTargetConfigRow(context, row)
	})
}

func (handler *Handler) BatchDeleteLogTargets(context *gin.Context) {
	handler.batchLogTargets(context, "delete", func(row logTargetRow) (gin.H, error) {
		if row.ManagedEnabled {
			return nil, fmt.Errorf("disable the monitor target before deleting it")
		}
		pending, _, err := logTargetPending(context, handler.db, row.ID)
		if err != nil {
			return nil, err
		}
		if pending {
			return nil, fmt.Errorf("wait for the uninstall task to finish before deleting")
		}
		queries := generated.New(handler.db)
		if err = queries.DetachInstallHistoryFromLogTarget(context, sql.NullInt64{Int64: row.ID, Valid: true}); err != nil {
			return nil, err
		}
		if err = deleteRowsAffected(queries.DeleteLogCollectionTarget(context, row.ID)); err != nil {
			return nil, err
		}
		return gin.H{"id": row.ID}, nil
	})
}

func (handler *Handler) batchServiceControl(context *gin.Context, action string) {
	handler.batchLogTargets(context, action, func(row logTargetRow) (gin.H, error) {
		return handler.dispatchLogTargetServiceControl(context, row.ID, action)
	})
}

func (handler *Handler) applyLogTargetConfigRow(context *gin.Context, row logTargetRow) (gin.H, error) {
	if handler.gateway == nil || !handler.gateway.IsOnline(row.HostName) {
		return nil, fmt.Errorf("host agent is offline")
	}
	queries := generated.New(handler.db)
	cluster, err := queries.GetDefaultEnabledOpenSearchCluster(context)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("没有已启用的默认 OpenSearch 集群，请先在日志存储里配置")
	}
	if err != nil {
		return nil, err
	}
	indexPrefix := cluster.IndexPrefix
	password, err := handler.secrets.Decrypt(cluster.Password)
	if err != nil {
		return nil, err
	}

	// 渲染服务级 inputs.d/outputs.d 片段（Index = logs-<项目>-<环境>-<业务>-<服务>-<档位>），
	// 指纹一致则跳过，避免无谓重启 Fluent Bit。
	entries, instances, renderErr := handler.loadHostLogRenderInput(context, row.HostID)
	if renderErr != nil {
		return nil, fmt.Errorf("渲染 Fluent Bit 配置失败: %w", renderErr)
	}
	for index := range entries {
		entries[index].Prefix = indexPrefix
	}
	rendered := renderHostLogConfig(entries, instances)
	if len(rendered.Fragments) == 0 {
		return nil, fmt.Errorf("主机没有可下发的日志片段：请先在服务的日志设置里开启采集（服务与日志定义的采集开关均需启用，且日志定义的部署模板需与服务一致）")
	}
	// 启用多行时随片段一并下发主配置：老主机安装时的主配置可能没有
	// Parsers_File 行（缺失会让 fluent-bit 启动失败），主配置内容始终由
	// backend 托管（fluentBitMainConfig 契约），重写为期望状态即自愈。
	for _, fragment := range rendered.Fragments {
		if fragment.Path == fluentBitParsersFile {
			rendered.Fragments = append(rendered.Fragments, logConfigFragment{
				Path:    fluentBitMainConfigPath,
				Content: fluentBitMainConfig(),
			})
			break
		}
	}

	currentFingerprint, _ := queries.GetLogTargetConfigFingerprint(context, row.ID)
	if rendered.Fingerprint != "" && currentFingerprint == rendered.Fingerprint {
		now := time.Now().UTC()
		_ = queries.MarkLogTargetConfigApplied(context, generated.MarkLogTargetConfigAppliedParams{
			LastAppliedTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: row.ID,
		})
		return gin.H{"skipped": true, "applied_at": now, "fingerprint": rendered.Fingerprint, "service_num": rendered.ServiceNum, "warnings": rendered.Warnings}, nil
	}

	host, port, err := firstOpenSearchEndpoint(cluster.Hosts)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(gin.H{"host": host, "port": port, "username": cluster.Username, "password": password})
	result, err := handler.gateway.Execute(context, row.HostName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("fluentbit-apply-%d", time.Now().UnixNano()), Type: "custom", Action: "configure_fluent_bit_opensearch", ParamsJson: string(params), TimeoutSeconds: 60})
	if err != nil {
		return nil, err
	}
	if result.Status != "success" {
		reason := firstNonEmpty(strings.TrimSpace(result.ErrorMessage), strings.TrimSpace(result.Stderr), strings.TrimSpace(result.Stdout))
		return nil, fmt.Errorf("Fluent Bit 配置下发失败: %s", reason)
	}
	// 片段内容全部由 backend 渲染，agent 只落盘 + 重启（见 apply_fluent_bit_config）。
	fragmentParams, _ := json.Marshal(gin.H{"files": rendered.Fragments, "restart": "true"})
	fragmentResult, err := handler.gateway.Execute(context, row.HostName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("fluentbit-fragments-%d", time.Now().UnixNano()), Type: "custom", Action: "apply_fluent_bit_config", ParamsJson: string(fragmentParams), TimeoutSeconds: 120})
	if err != nil {
		return nil, err
	}
	if fragmentResult.Status != "success" {
		reason := firstNonEmpty(strings.TrimSpace(fragmentResult.ErrorMessage), strings.TrimSpace(fragmentResult.Stderr), strings.TrimSpace(fragmentResult.Stdout))
		return nil, fmt.Errorf("Fluent Bit 片段下发失败: %s", reason)
	}
	now := time.Now().UTC()
	if err = queries.MarkLogTargetConfigSynced(context, generated.MarkLogTargetConfigSyncedParams{
		LastAppliedTime: sql.NullTime{Time: now, Valid: true}, ConfigFingerprint: rendered.Fingerprint,
		UpdateTime: now, ID: row.ID,
	}); err != nil {
		return nil, err
	}
	return gin.H{"skipped": false, "applied_at": now, "fingerprint": rendered.Fingerprint, "service_num": rendered.ServiceNum, "warnings": rendered.Warnings}, nil
}

// ---- 批量创建（纳管 Fluent Bit） ----

func (handler *Handler) BatchCreateLogTargets(context *gin.Context) {
	var input struct {
		HostIDs    []int64 `json:"host_ids"`
		InstallNow bool    `json:"install_now"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.HostIDs) == 0 {
		response.BusinessError(context, 400, "host_ids must be a non-empty array", nil)
		return
	}
	queries := generated.New(handler.db)
	results := make([]gin.H, 0, len(input.HostIDs))
	success := 0
	for _, hostID := range input.HostIDs {
		// 主机读取复用监控目标域的 GetHostTargetIdentity（列集一致：实例名、IP、是否已下线）。
		host, err := queries.GetHostTargetIdentity(context, hostID)
		if err != nil || host.IsDeletedInCloud {
			results = append(results, gin.H{"host_id": hostID, "host": host.InstanceName, "ok": false, "message": "host not found"})
			continue
		}
		name, ip := host.InstanceName, host.Ip
		label := name
		if label == "" {
			label = ip
		}
		now := time.Now().UTC()
		targetID, created, err := createLogCollectionTargetIfAbsent(context, queries, generated.CreateLogCollectionTargetIfAbsentParams{
			CreateTime: now, UpdateTime: now, HostID: hostID,
		})
		if err != nil {
			results = append(results, gin.H{"host_id": hostID, "host": label, "ok": false, "message": err.Error()})
			continue
		}
		if !created {
			results = append(results, gin.H{"host_id": hostID, "host": label, "ok": false, "message": "target already managed"})
			continue
		}
		if input.InstallNow {
			row := logTargetRow{ID: targetID, HostID: hostID, ManagedEnabled: true, HostName: name, HostIP: ip}
			if _, err := handler.dispatchLogTargetInstall(context, row); err != nil {
				results = append(results, gin.H{"host_id": hostID, "host": label, "ok": false, "message": err.Error()})
				continue
			}
		}
		success++
		results = append(results, gin.H{"host_id": hostID, "host": label, "ok": true, "message": ""})
	}
	response.Success(context, gin.H{"total": len(results), "success": success, "failed": len(results) - success, "results": results})
}
