package monitor

import (
	"errors"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// exporter 监控目标（monitor_target）的人工重试/重新下发，对应 Django retry action。
// 前端"重新下发"按钮对 install_status 不做限制（失败重试 + 修复历史遗留），
// 后端只看 managed_enabled 决定装还是卸；安装/卸载复用监控软件仓库绑定 playbook，
// 与 Fluent Bit 派发同链路（automation_execution_job + monitor_target_install_history）。
// 业务失败（无 agent/无包/无 playbook/文件缺失）沿用 Django 行为：把原因写入
// target.install_message 后仍返回目标对象，只有数据库错误才走 400。

type targetInstallRow struct {
	ID             int64
	HostID         int64
	ManagedEnabled bool
	ExporterType   string
	HostName       string
	HostIP         string
	AgentID        string
	OSID           string
	OSIDLike       string
	OSVersionID    string
	Architecture   string
}

func loadMonitorTargetRow(ginContext *gin.Context, db *sql.DB, id int64) (targetInstallRow, error) {
	var row targetInstallRow
	err := db.QueryRowContext(ginContext, `SELECT t.id,t.host_id,t.managed_enabled,t.exporter_type,
		COALESCE(h.instance_name,''),COALESCE(h.ip,''),COALESCE(h.agent_id,''),
		COALESCE(s.os_id,''),COALESCE(s.os_id_like,''),COALESCE(s.os_version_id,''),
		COALESCE(hw.architecture,'')
		FROM monitor_target t
		JOIN assets_host h ON h.id=t.host_id
		LEFT JOIN assets_hostsystem s ON s.host_id=t.host_id
		LEFT JOIN assets_hosthardware hw ON hw.host_id=t.host_id
		WHERE t.id=?`, id).Scan(&row.ID, &row.HostID, &row.ManagedEnabled, &row.ExporterType,
		&row.HostName, &row.HostIP, &row.AgentID, &row.OSID, &row.OSIDLike, &row.OSVersionID, &row.Architecture)
	return row, err
}

func (handler *Handler) RetryTarget(ginContext *gin.Context) {
	id := parseID(ginContext.Param("id"))
	row, err := loadMonitorTargetRow(ginContext, handler.db, id)
	if err == sql.ErrNoRows {
		response.BusinessError(ginContext, 404, "monitor target not found", nil)
		return
	}
	if err != nil {
		response.Error(ginContext, err)
		return
	}
	if row.HostID == 0 {
		response.BusinessError(ginContext, 400, "监控目标未关联主机，无法重试", nil)
		return
	}
	// 人工触发视为新一轮操作周期，重置历史重试计数。
	// 注意：这里不先把 install_status 写成 pending，否则派发里的 pending 防重会直接短路。
	now := time.Now().UTC()
	if _, err = handler.db.ExecContext(ginContext, `UPDATE monitor_target SET retry_count=0,install_message='人工触发重试',update_time=? WHERE id=?`, now, id); err != nil {
		response.Error(ginContext, err)
		return
	}
	if err = handler.dispatchExporterJob(ginContext, row); err != nil {
		var guard guardFailure
		if !errors.As(err, &guard) {
			response.Error(ginContext, err)
			return
		}
		// 业务失败原因已写入 target，按 Django 行为仍返回目标对象。
	}
	handler.GetTarget(ginContext)
}

// guardFailure 表示派发前置守卫未通过（无 agent/无包/无 playbook/pending 冲突）。
// 原因已写入 target.install_message，调用方应继续返回目标对象而不是 500。
type guardFailure struct {
	status  string
	message string
}

func (failure guardFailure) Error() string {
	return failure.message
}

// setTargetInstallState 记录业务失败/挂起原因，成功时以 guardFailure 回传给上层识别。
func (handler *Handler) setTargetInstallState(ginContext *gin.Context, id int64, status, message string) error {
	if _, err := handler.db.ExecContext(ginContext, `UPDATE monitor_target SET install_status=?,install_message=?,update_time=? WHERE id=?`, status, message, time.Now().UTC(), id); err != nil {
		return err
	}
	return guardFailure{status: status, message: message}
}

func (handler *Handler) dispatchExporterJob(ginContext *gin.Context, row targetInstallRow) error {
	action := "install"
	if !row.ManagedEnabled {
		action = "uninstall"
	}
	exporter := strings.TrimSpace(row.ExporterType)
	if strings.TrimSpace(row.AgentID) == "" {
		return handler.setTargetInstallState(ginContext, row.ID, "failed", fmt.Sprintf("主机未绑定 agent 实例，无法下发 %s %s任务", exporter, action))
	}
	if handler.gateway == nil || !handler.gateway.IsOnline(row.AgentID) {
		return handler.setTargetInstallState(ginContext, row.ID, "failed", "主机 agent 离线，无法下发任务，请先确认 agent 运行状态")
	}
	pending, _, err := monitorTargetPending(ginContext, handler.db, row.ID)
	if err != nil {
		return err
	}
	if pending {
		// 与 Django 一致：保持 pending 并提示已有任务，由用户先取消再重试。
		return handler.setTargetInstallState(ginContext, row.ID, "pending", fmt.Sprintf("%s任务已存在（pending）", action))
	}

	packageRow, playbookID, extra, err := handler.prepareExporterDispatch(ginContext, row, action)
	if err != nil {
		return err
	}
	content, err := playbookContent(ginContext, handler.db, playbookID)
	if err != nil {
		return handler.setTargetInstallState(ginContext, row.ID, "failed", fmt.Sprintf("%s 绑定的 %s Playbook 已不存在，请重新在监控软件仓库中选择", exporter, action))
	}

	summary := "已下发安装任务"
	if action == "uninstall" {
		summary = "已下发卸载任务"
	}
	now := time.Now().UTC()
	inventory := gin.H{"selected_host_ids": []int64{row.HostID}, "hosts": []gin.H{{
		"host_id": row.HostID, "host_name": row.HostName, "host_ip": row.HostIP,
		"group_id": nil, "group_name": "", "group_path": "", "agent_online": true,
	}}}
	inventoryJSON, _ := json.Marshal(inventory)
	extraJSON, _ := json.Marshal(extra)
	jobResult, err := handler.db.ExecContext(ginContext, `INSERT INTO automation_execution_job
		(create_time,update_time,remark,job_id,status,trigger_type,inventory_snapshot,extra_vars,result_summary,
		 task_name_snapshot,template_name_snapshot,template_content_snapshot,`+"`limit`"+`,run_as_user_snapshot,run_as_group_snapshot,work_directory_snapshot,requested_user_id,requested_username)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		now, now, nil, uuid.NewString(), "pending", "manual", string(inventoryJSON), string(extraJSON),
		fmt.Sprintf(`{"message":"%s %s job queued"}`, exporter, action), fmt.Sprintf("%s %s", exporter, action), exporter, content,
		"", "", "", packageRow.WorkDirectory, nil, "system")
	if err != nil {
		return err
	}
	jobID, _ := jobResult.LastInsertId()
	historyResult, err := handler.db.ExecContext(ginContext, `INSERT INTO monitor_target_install_history
		(create_time,update_time,remark,action,trigger_type,status,host_id_snapshot,host_name_snapshot,host_ip_snapshot,
		 exporter_type_snapshot,summary_message,stdout_snapshot,stderr_snapshot,error_message_snapshot,result_summary_snapshot,
		 requested_user_id_snapshot,requested_username_snapshot,start_time,host_id,target_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		now, now, nil, action, "manual", "pending", row.HostID, row.HostName, row.HostIP,
		exporter, summary, "", "", "", "{}", nil, "system", now, row.HostID, row.ID)
	if err != nil {
		return err
	}
	historyID, _ := historyResult.LastInsertId()
	if _, err = handler.db.ExecContext(ginContext, `UPDATE monitor_target SET install_status='pending',install_message=?,last_dispatch_manual=TRUE,update_time=? WHERE id=?`, summary, now, row.ID); err != nil {
		return err
	}
	// playbook 可能执行数分钟，异步跑，前端通过列表刷新和安装历史查看进度。
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_ = handler.jobs.RunJobByID(ctx, jobID)
		finalStatus := "failed"
		var summaryMessage string
		var jobStatus string
		if err := handler.db.QueryRowContext(context.Background(), `SELECT status,COALESCE(JSON_UNQUOTE(JSON_EXTRACT(result_summary,'$.message')),'') FROM automation_execution_job WHERE id=?`, jobID).Scan(&jobStatus, &summaryMessage); err == nil && jobStatus == "success" {
			finalStatus = "success"
		} else if summaryMessage == "" {
			summaryMessage = fmt.Sprintf("%s %s任务执行失败", exporter, action)
		}
		message := summaryMessage
		if finalStatus == "success" {
			message = ""
		}
		finish := time.Now().UTC()
		_, _ = handler.db.ExecContext(context.Background(), `UPDATE monitor_target SET install_status=?,install_message=?,update_time=? WHERE id=? AND install_status='pending'`, finalStatus, message, finish, row.ID)
		_, _ = handler.db.ExecContext(context.Background(), `UPDATE monitor_target_install_history SET status=?,summary_message=?,end_time=?,duration_seconds=TIMESTAMPDIFF(MICROSECOND,create_time,?)/1000000,update_time=? WHERE id=? AND status='pending'`, finalStatus, message, finish, finish, finish, historyID)
	}()
	return nil
}

type exporterPackageRow struct {
	Version            string
	Arch               string
	Family             string
	Major              string
	Format             string
	File               string
	SHA256             string
	ServiceFileContent string
	RunAsUser          string
	RunAsGroup         string
	WorkDirectory      string
}

// prepareExporterDispatch 选包、取 playbook 并组装 extra_vars。
// 失败原因写入 target.install_message（返回 nil error 时 handler 仍会返回目标对象）。
func (handler *Handler) prepareExporterDispatch(ginContext *gin.Context, row targetInstallRow, action string) (exporterPackageRow, int64, gin.H, error) {
	exporter := strings.TrimSpace(row.ExporterType)
	noop := exporterPackageRow{}
	if action == "uninstall" {
		var pkg exporterPackageRow
		var playbookID sql.NullInt64
		err := handler.db.QueryRowContext(ginContext, `SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,COALESCE(file,''),COALESCE(sha256,''),
			service_file_content,service_run_as_user,service_run_as_group,work_directory,uninstall_playbook_template_id
			FROM monitor_software_package
			WHERE package_type='exporter' AND name=? AND enabled=TRUE
			ORDER BY create_time DESC LIMIT 1`, exporter).Scan(
			&pkg.Version, &pkg.Arch, &pkg.Family, &pkg.Major, &pkg.Format, &pkg.File, &pkg.SHA256,
			&pkg.ServiceFileContent, &pkg.RunAsUser, &pkg.RunAsGroup, &pkg.WorkDirectory, &playbookID)
		if err == sql.ErrNoRows {
			return noop, 0, nil, handler.setTargetInstallState(ginContext, row.ID, "failed", fmt.Sprintf("本地软件仓库缺少 %s 的启用安装包，无法下发卸载任务", exporter))
		}
		if err != nil {
			return noop, 0, nil, err
		}
		if !playbookID.Valid {
			return noop, 0, nil, handler.setTargetInstallState(ginContext, row.ID, "failed", fmt.Sprintf("%s 未配置卸载 Playbook，请在监控软件仓库中选择", exporter))
		}
		return pkg, playbookID.Int64, gin.H{"exporter_name": exporter, "service_name": exporter + ".service"}, nil
	}

	// 安装：按 /etc/os-release 与 CPU 架构选唯一适配包，rpm/deb 不做跨发行版兜底，tar.gz(any) 作可移植兜底。
	rows, err := handler.db.QueryContext(ginContext, `SELECT version,arch,COALESCE(platform_family,''),COALESCE(platform_major,''),package_format,file,sha256,
		service_file_content,service_run_as_user,service_run_as_group,work_directory,install_playbook_template_id
		FROM monitor_software_package
		WHERE package_type='exporter' AND name=? AND enabled=TRUE AND file<>''
		ORDER BY create_time DESC`, exporter)
	if err != nil {
		return noop, 0, nil, err
	}
	defer rows.Close()
	candidates := make([]exporterPackageRow, 0, 4)
	playbookIDs := make([]sql.NullInt64, 0, 4)
	for rows.Next() {
		var pkg exporterPackageRow
		var playbookID sql.NullInt64
		if err = rows.Scan(&pkg.Version, &pkg.Arch, &pkg.Family, &pkg.Major, &pkg.Format, &pkg.File, &pkg.SHA256,
			&pkg.ServiceFileContent, &pkg.RunAsUser, &pkg.RunAsGroup, &pkg.WorkDirectory, &playbookID); err != nil {
			return noop, 0, nil, err
		}
		candidates = append(candidates, pkg)
		playbookIDs = append(playbookIDs, playbookID)
	}
	if err = rows.Err(); err != nil {
		return noop, 0, nil, err
	}
	if len(candidates) == 0 {
		return noop, 0, nil, handler.setTargetInstallState(ginContext, row.ID, "failed", fmt.Sprintf("本地软件仓库缺少 %s 的启用 exporter 安装包，请先上传对应的离线安装包", exporter))
	}
	family, major, arch := normalizeExporterPlatform(row)
	if arch == "" {
		return noop, 0, nil, handler.setTargetInstallState(ginContext, row.ID, "failed", "主机架构信息缺失，请先执行资产采集")
	}
	// 包格式取决于平台族（rhel→rpm、ubuntu/debian→deb），架构只参与 family/arch 匹配键。
	expectedFormat := map[string]string{"rhel": "rpm", "ubuntu": "deb", "debian": "deb"}[family]
	selected, playbookID := -1, sql.NullInt64{}
	if expectedFormat != "" {
		for index, item := range candidates {
			if item.Arch == arch && item.Family == family && item.Major == major && item.Format == expectedFormat {
				selected, playbookID = index, playbookIDs[index]
				break
			}
		}
	}
	if selected < 0 {
		// 通用 tar.gz 包以 platform_family=any 发布，作为可移植兜底。
		for index, item := range candidates {
			if item.Arch == arch && item.Format == "tar.gz" && item.Family == "any" && item.Major == "" {
				selected, playbookID = index, playbookIDs[index]
				break
			}
		}
	}
	if selected < 0 {
		familyLabel, majorLabel := family, major
		if familyLabel == "" {
			familyLabel = "unknown"
		}
		if majorLabel == "" {
			majorLabel = "unknown"
		}
		return noop, 0, nil, handler.setTargetInstallState(ginContext, row.ID, "failed",
			fmt.Sprintf("本地软件仓库缺少 %s 的 %s-%s/%s 安装包；请先上传匹配的离线 rpm/deb 包", exporter, familyLabel, majorLabel, arch))
	}
	if !playbookID.Valid {
		return noop, 0, nil, handler.setTargetInstallState(ginContext, row.ID, "failed", fmt.Sprintf("%s 未配置安装 Playbook，请在监控软件仓库中选择", exporter))
	}
	pkg := candidates[selected]
	packageLocalPath := filepath.Join(handler.packageRoot, filepath.FromSlash(pkg.File))
	if info, statErr := os.Stat(packageLocalPath); statErr != nil || info.IsDir() {
		return noop, 0, nil, handler.setTargetInstallState(ginContext, row.ID, "failed", fmt.Sprintf("%s 的安装包文件不存在，请重新上传并启用对应包", exporter))
	}
	// 同名同版本跨平台包的校验和清单，供 playbook 做一致性校验。
	checksumRows, err := handler.db.QueryContext(ginContext, `SELECT os,arch,sha256 FROM monitor_software_package
		WHERE package_type='exporter' AND name=? AND version=? AND enabled=TRUE AND sha256<>''`, exporter, pkg.Version)
	if err != nil {
		return noop, 0, nil, err
	}
	defer checksumRows.Close()
	checksums := gin.H{}
	for checksumRows.Next() {
		var packageOS, packageArch, packageSHA string
		if err = checksumRows.Scan(&packageOS, &packageArch, &packageSHA); err != nil {
			return noop, 0, nil, err
		}
		checksums[packageOS+"-"+packageArch] = packageSHA
	}
	if err = checksumRows.Err(); err != nil {
		return noop, 0, nil, err
	}
	extra := gin.H{
		"exporter_name":           exporter,
		"exporter_version":        pkg.Version,
		"service_name":            exporter + ".service",
		"service_file_content":    pkg.ServiceFileContent,
		"service_run_as_user":     defaultString(pkg.RunAsUser, "dj-agent"),
		"service_run_as_group":    defaultString(pkg.RunAsGroup, "dj-agent"),
		"package_local_path":      packageLocalPath,
		"package_file_name":       filepath.Base(pkg.File),
		"package_format":          pkg.Format,
		"package_platform_family": pkg.Family,
		"package_platform_major":  pkg.Major,
		"package_sha256":          pkg.SHA256,
		"checksums":               checksums,
	}
	return pkg, playbookID.Int64, extra, nil
}

// normalizeExporterPlatform 将 agent 上报的 os-release 与 uname 架构归一为仓库匹配键，
// 与 Django monitor/package_selector.normalize_host_platform 契约一致。
func normalizeExporterPlatform(row targetInstallRow) (family, major, arch string) {
	osID := strings.ToLower(strings.TrimSpace(row.OSID))
	osIDLike := strings.ToLower(strings.TrimSpace(row.OSIDLike))
	rhelIDs := map[string]bool{"rhel": true, "centos": true, "rocky": true, "almalinux": true, "ol": true}
	if rhelIDs[osID] {
		family = "rhel"
	} else {
		for _, id := range strings.Fields(osIDLike) {
			if rhelIDs[id] {
				family = "rhel"
				break
			}
		}
	}
	if family == "" {
		switch {
		case osID == "ubuntu":
			family = "ubuntu"
		case osID == "debian" || strings.Contains(osIDLike, "debian"):
			family = "debian"
		}
	}
	switch strings.ToLower(strings.TrimSpace(row.Architecture)) {
	case "x86_64", "amd64":
		arch = "amd64"
	case "aarch64", "arm64":
		arch = "arm64"
	}
	version := strings.TrimSpace(row.OSVersionID)
	if index := strings.Index(version, "."); index > 0 {
		major = version[:index]
	} else {
		major = version
	}
	return family, major, arch
}

// monitorTargetPending 检查目标最近一次安装/卸载历史是否仍在执行。
// 派发 goroutine 兜底 30 分钟超时，因此超过 31 分钟的 pending 视为进程中断遗留，
// 置为 failed 后放行本次下发（对应 Django 的 stale pending 过期逻辑）。
func monitorTargetPending(ginContext *gin.Context, db *sql.DB, id int64) (bool, int64, error) {
	var historyID int64
	var status string
	var createTime time.Time
	err := db.QueryRowContext(ginContext, `SELECT id,status,create_time FROM monitor_target_install_history WHERE target_id=? ORDER BY id DESC LIMIT 1`, id).Scan(&historyID, &status, &createTime)
	if err == sql.ErrNoRows {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if status != "pending" && status != "running" {
		return false, historyID, nil
	}
	if time.Since(createTime) > 31*time.Minute {
		if _, err = db.ExecContext(ginContext, `UPDATE monitor_target_install_history SET status='failed',error_message_snapshot='任务执行超时（进程中断遗留），已自动过期',update_time=? WHERE id=? AND status IN ('pending','running')`, time.Now().UTC(), historyID); err != nil {
			return false, 0, err
		}
		return false, historyID, nil
	}
	return true, historyID, nil
}
