package monitor

import (
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
	AgentID        string
	OSType         string
	OSIDLike       string
	OSVersionID    string
}

func loadLogTarget(context *gin.Context, db *sql.DB, id int64) (logTargetRow, error) {
	var row logTargetRow
	err := db.QueryRowContext(context, `SELECT l.id,l.host_id,l.managed_enabled,l.install_status,
		COALESCE(h.instance_name,''),COALESCE(h.ip,''),COALESCE(h.agent_id,''),
		COALESCE(s.os_type,''),COALESCE(s.os_id_like,''),COALESCE(s.os_version_id,'')
		FROM monitor_log_collection_target l
		JOIN assets_host h ON h.id=l.host_id
		LEFT JOIN assets_hostsystem s ON s.host_id=l.host_id
		WHERE l.id=?`, id).Scan(&row.ID, &row.HostID, &row.ManagedEnabled, &row.InstallStatus,
		&row.HostName, &row.HostIP, &row.AgentID, &row.OSType, &row.OSIDLike, &row.OSVersionID)
	return row, err
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

func logTargetPending(context *gin.Context, db *sql.DB, id int64) (bool, int64, error) {
	var historyID int64
	var status string
	err := db.QueryRowContext(context, `SELECT id,status FROM monitor_target_install_history WHERE log_collection_target_id=? ORDER BY id DESC LIMIT 1`, id).Scan(&historyID, &status)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	return status == "pending" || status == "running", historyID, nil
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

func (handler *Handler) pickFluentBitPackage(context *gin.Context, row logTargetRow, uninstall bool) (*fluentBitPackage, error) {
	playbookColumn := "install_playbook_template_id"
	if uninstall {
		playbookColumn = "uninstall_playbook_template_id"
	}
	rows, err := handler.db.QueryContext(context, `SELECT id,platform_family,platform_major,package_format,file,sha256,`+playbookColumn+`
		FROM monitor_software_package
		WHERE package_type='fluent_bit' AND enabled=TRUE AND `+playbookColumn+` IS NOT NULL ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	candidates := make([]*fluentBitPackage, 0)
	for rows.Next() {
		item := &fluentBitPackage{}
		if err = rows.Scan(&item.ID, &item.Family, &item.Major, &item.Format, &item.File, &item.SHA256, &item.PlaybookID); err != nil {
			return nil, err
		}
		candidates = append(candidates, item)
	}
	if err = rows.Err(); err != nil {
		return nil, err
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

func playbookContent(context *gin.Context, db *sql.DB, playbookID int64) (string, error) {
	var content string
	err := db.QueryRowContext(context, `SELECT content FROM automation_playbook_template WHERE id=?`, playbookID).Scan(&content)
	return content, err
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
	if handler.gateway == nil || !handler.gateway.IsOnline(row.AgentID) {
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
	result, err := handler.db.ExecContext(ginContext, `INSERT INTO automation_execution_job
		(create_time,update_time,remark,job_id,status,trigger_type,inventory_snapshot,extra_vars,result_summary,
		 task_name_snapshot,template_name_snapshot,template_content_snapshot,`+"`limit`"+`,run_as_user_snapshot,run_as_group_snapshot,work_directory_snapshot,requested_user_id,requested_username)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		now, now, nil, uuid.NewString(), "pending", "manual", string(inventoryJSON), string(extraJSON),
		`{"message":"Fluent Bit install/uninstall job queued"}`, fmt.Sprintf("Fluent Bit %s", action), "fluent-bit", content, "", "", "", "", nil, "system")
	if err != nil {
		return nil, err
	}
	jobID, _ := result.LastInsertId()
	historyResult, err := handler.db.ExecContext(ginContext, `INSERT INTO monitor_target_install_history
		(create_time,update_time,remark,action,trigger_type,status,host_id_snapshot,host_name_snapshot,host_ip_snapshot,
		 exporter_type_snapshot,summary_message,stdout_snapshot,stderr_snapshot,error_message_snapshot,result_summary_snapshot,
		 requested_user_id_snapshot,requested_username_snapshot,start_time,host_id,log_collection_target_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,NULL,?,?)`,
		now, now, nil, action, "manual", "pending", sql.NullInt64{Int64: row.HostID, Valid: true},
		row.HostName, row.HostIP, "fluent_bit", "", "", "", "", `{"checks":[]}`, nil, "system", row.HostID, row.ID)
	if err != nil {
		return nil, err
	}
	historyID, _ := historyResult.LastInsertId()
	if _, err = handler.db.ExecContext(ginContext, `UPDATE monitor_log_collection_target SET install_status='pending',install_message='',last_dispatch_manual=TRUE,update_time=? WHERE id=?`, now, row.ID); err != nil {
		return nil, err
	}
	// playbook 可能执行数分钟，异步跑，前端通过列表刷新和安装历史查看进度。
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_ = handler.jobs.RunJobByID(ctx, jobID)
		// 任务结束后把 install_status 落成终态，供列表直接展示；安装历史本身由 automation 侧结果快照追溯。
		finalStatus := "failed"
		var summary string
		var jobStatus string
		if err := handler.db.QueryRowContext(context.Background(), `SELECT status,COALESCE(JSON_UNQUOTE(JSON_EXTRACT(result_summary,'$.message')),'') FROM automation_execution_job WHERE id=?`, jobID).Scan(&jobStatus, &summary); err == nil && jobStatus == "success" {
			finalStatus = desiredStatus
		} else if summary == "" {
			summary = "Fluent Bit 任务执行失败"
		}
		message := summary
		if finalStatus == desiredStatus {
			message = ""
		}
		_, _ = handler.db.ExecContext(context.Background(), `UPDATE monitor_log_collection_target SET install_status=?,install_message=?,runtime_status=CASE WHEN ?='success' THEN 'running' ELSE runtime_status END,update_time=? WHERE id=? AND install_status='pending'`, finalStatus, message, finalStatus, time.Now().UTC(), row.ID)
		_, _ = handler.db.ExecContext(context.Background(), `UPDATE monitor_target_install_history SET status=?,summary_message=?,end_time=?,duration_seconds=TIMESTAMPDIFF(MICROSECOND,create_time,?)/1000000,update_time=? WHERE id=? AND status='pending'`, finalStatus, message, time.Now().UTC(), time.Now().UTC(), time.Now().UTC(), historyID)
	}()
	_ = desiredStatus
	return gin.H{"id": row.ID, "action": action, "history_id": historyID, "job_id": jobID}, nil
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
	var agentID string
	if err := handler.db.QueryRowContext(context, `SELECT COALESCE(h.agent_id,'') FROM monitor_log_collection_target l JOIN assets_host h ON h.id=l.host_id WHERE l.id=?`, id).Scan(&agentID); err != nil {
		return nil, fmt.Errorf("log collection target not found")
	}
	if handler.gateway == nil || !handler.gateway.IsOnline(agentID) {
		return nil, fmt.Errorf("host agent is offline")
	}
	// agent 通用命令通道：agent 以 root 运行，直接 systemctl，无需 sudo。
	params, _ := json.Marshal(gin.H{"command": "systemctl", "args": []string{action, fluentBitServiceName + ".service"}})
	result, err := handler.gateway.Execute(context, agentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("fluentbit-service-%d", time.Now().UnixNano()), Type: "command", Action: "fluent_bit_service_control", ParamsJson: string(params), TimeoutSeconds: 30})
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
	_, _ = handler.db.ExecContext(context, `UPDATE monitor_log_collection_target SET runtime_status=?,update_time=? WHERE id=?`, runtimeStatus, time.Now().UTC(), id)
}

// ---- 下发配置（agent 内置 configure_fluent_bit_opensearch） ----

func (handler *Handler) ApplyLogTargetConfig(context *gin.Context) {
	id := parseID(context.Param("id"))
	var agentID string
	var clusterHosts, username, encryptedPassword string
	err := handler.db.QueryRowContext(context, `SELECT COALESCE(h.agent_id,''), c.hosts, c.username, c.password
		FROM monitor_log_collection_target l
		JOIN assets_host h ON h.id=l.host_id
		JOIN monitor_opensearch_cluster c ON c.enabled=TRUE
		WHERE l.id=? ORDER BY c.is_default DESC, c.id LIMIT 1`, id).Scan(&agentID, &clusterHosts, &username, &encryptedPassword)
	if err == sql.ErrNoRows {
		response.BusinessError(context, 400, "没有已启用的默认 OpenSearch 集群，请先在日志存储里配置", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if handler.gateway == nil || !handler.gateway.IsOnline(agentID) {
		response.BusinessError(context, 400, "host agent is offline", nil)
		return
	}
	host, port, err := firstOpenSearchEndpoint(clusterHosts)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	password, err := handler.secrets.Decrypt(encryptedPassword)
	if err != nil {
		response.Error(context, err)
		return
	}
	params, _ := json.Marshal(gin.H{"host": host, "port": port, "username": username, "password": password})
	result, err := handler.gateway.Execute(context, agentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("fluentbit-apply-%d", time.Now().UnixNano()), Type: "custom", Action: "configure_fluent_bit_opensearch", ParamsJson: string(params), TimeoutSeconds: 60})
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
	if _, err = handler.db.ExecContext(context, `UPDATE monitor_log_collection_target SET last_applied_time=?,runtime_status='running',last_error='',update_time=? WHERE id=?`, now, now, id); err != nil {
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
	pending, historyID, err := logTargetPending(context, handler.db, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	if !pending {
		response.BusinessError(context, 400, "current task has ended and does not need cancellation", nil)
		return
	}
	now := time.Now().UTC()
	if _, err = handler.db.ExecContext(context, `UPDATE monitor_target_install_history SET status='cancelled',summary_message='任务已取消',error_message_snapshot='任务已由用户取消',end_time=?,duration_seconds=TIMESTAMPDIFF(MICROSECOND,create_time,?)/1000000,update_time=? WHERE id=?`, now, now, now, historyID); err != nil {
		response.Error(context, err)
		return
	}
	if _, err = handler.db.ExecContext(context, `UPDATE monitor_log_collection_target SET install_status='failed',install_message='安装/卸载任务已取消',update_time=? WHERE id=?`, now, id); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": id})
}

func (handler *Handler) DeleteLogTarget(context *gin.Context) {
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
	if row.ManagedEnabled {
		response.BusinessError(context, 400, "disable the monitor target before deleting it", nil)
		return
	}
	pending, _, err := logTargetPending(context, handler.db, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	if pending {
		response.BusinessError(context, 400, "wait for the uninstall task to finish before deleting", nil)
		return
	}
	if _, err = handler.db.ExecContext(context, `UPDATE monitor_target_install_history SET log_collection_target_id=NULL WHERE log_collection_target_id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	if _, err = handler.db.ExecContext(context, `DELETE FROM monitor_log_collection_target WHERE id=?`, id); err != nil {
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
		if _, err = handler.db.ExecContext(context, `UPDATE monitor_target_install_history SET log_collection_target_id=NULL WHERE log_collection_target_id=?`, row.ID); err != nil {
			return nil, err
		}
		if _, err = handler.db.ExecContext(context, `DELETE FROM monitor_log_collection_target WHERE id=?`, row.ID); err != nil {
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
	if handler.gateway == nil || !handler.gateway.IsOnline(row.AgentID) {
		return nil, fmt.Errorf("host agent is offline")
	}
	var clusterHosts, username, encryptedPassword string
	err := handler.db.QueryRowContext(context, `SELECT hosts,username,password FROM monitor_opensearch_cluster WHERE enabled=TRUE ORDER BY is_default DESC, id LIMIT 1`).Scan(&clusterHosts, &username, &encryptedPassword)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("没有已启用的默认 OpenSearch 集群，请先在日志存储里配置")
	}
	if err != nil {
		return nil, err
	}
	host, port, err := firstOpenSearchEndpoint(clusterHosts)
	if err != nil {
		return nil, err
	}
	password, err := handler.secrets.Decrypt(encryptedPassword)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(gin.H{"host": host, "port": port, "username": username, "password": password})
	result, err := handler.gateway.Execute(context, row.AgentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("fluentbit-apply-%d", time.Now().UnixNano()), Type: "custom", Action: "configure_fluent_bit_opensearch", ParamsJson: string(params), TimeoutSeconds: 60})
	if err != nil {
		return nil, err
	}
	if result.Status != "success" {
		reason := firstNonEmpty(strings.TrimSpace(result.ErrorMessage), strings.TrimSpace(result.Stderr), strings.TrimSpace(result.Stdout))
		return nil, fmt.Errorf("Fluent Bit 配置下发失败: %s", reason)
	}
	now := time.Now().UTC()
	if _, err = handler.db.ExecContext(context, `UPDATE monitor_log_collection_target SET last_applied_time=?,runtime_status='running',last_error='',update_time=? WHERE id=?`, now, now, row.ID); err != nil {
		return nil, err
	}
	return gin.H{"skipped": false, "applied_at": now}, nil
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
	results := make([]gin.H, 0, len(input.HostIDs))
	success := 0
	for _, hostID := range input.HostIDs {
		var name, ip, agentID string
		var deleted bool
		if err := handler.db.QueryRowContext(context, `SELECT COALESCE(instance_name,''),COALESCE(ip,''),COALESCE(agent_id,''),is_deleted_in_cloud FROM assets_host WHERE id=?`, hostID).Scan(&name, &ip, &agentID, &deleted); err != nil || deleted {
			results = append(results, gin.H{"host_id": hostID, "host": name, "ok": false, "message": "host not found"})
			continue
		}
		label := name
		if label == "" {
			label = ip
		}
		now := time.Now().UTC()
		result, err := handler.db.ExecContext(context, `INSERT IGNORE INTO monitor_log_collection_target
			(create_time,update_time,remark,host_id,agent_installed,agent_version,runtime_status,config_fingerprint,last_error,install_status,install_message,last_dispatch_manual,managed_enabled,retry_count)
			VALUES(?,?,?,?,FALSE,'','unknown','','','unknown','',FALSE,TRUE,0)`, now, now, nil, hostID)
		if err != nil {
			results = append(results, gin.H{"host_id": hostID, "host": label, "ok": false, "message": err.Error()})
			continue
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			results = append(results, gin.H{"host_id": hostID, "host": label, "ok": false, "message": "target already managed"})
			continue
		}
		targetID, _ := result.LastInsertId()
		if input.InstallNow {
			row := logTargetRow{ID: targetID, HostID: hostID, ManagedEnabled: true, HostName: name, HostIP: ip, AgentID: agentID}
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
