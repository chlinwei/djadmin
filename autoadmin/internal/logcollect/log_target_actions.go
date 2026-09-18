package logcollect

import (
	generated "autoadmin/internal/platform/database/generated"

	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/shared/filebeat"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Filebeat 纳管目标（monitor_log_collection_target）的运维操作：
// 安装/卸载（离线 playbook，与 exporter 安装同链路）、启停/查状态（agent 通用命令）、
// 下发配置（agent 内置 configure_filebeat_output + apply_filebeat_config 动作）、取消/删除/批量操作。

const filebeatServiceName = "filebeat"

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

// ---- 安装/卸载（离线 playbook，仅 Filebeat tar.gz 便携包） ----

type filebeatPackage struct {
	ID                 int64
	Arch               string
	File               string
	SHA256             string
	ServiceFileContent string
	PlaybookID         sql.NullInt64
}

// pickFilebeatPackage 只认 package_type=filebeat、format=tar.gz(any) 的启用包，按主机架构匹配。
// Filebeat 官方包是单静态二进制（自带依赖），只关心架构，不再区分发行版/主版本/包格式。
func (handler *Handler) pickFilebeatPackage(ctx context.Context, row logTargetRow, uninstall bool) (*filebeatPackage, error) {
	queries := generated.New(handler.db)
	arch := handler.hostArchitecture(ctx, row.HostID)
	if arch == "" && handler.refreshHostInfo != nil && row.HostID > 0 {
		// 架构来自资产采集，缺了先自动补采一次（与 exporter 选包同一策略）。
		if err := handler.refreshHostInfo(ctx, row.HostID); err == nil {
			arch = handler.hostArchitecture(ctx, row.HostID)
		}
	}
	if arch == "" {
		return nil, fmt.Errorf("主机架构信息缺失且自动采集未获取到，请确认 agent 在线后重试（或先执行资产采集）")
	}
	candidates := make([]*filebeatPackage, 0)
	if uninstall {
		rows, err := queries.ListUninstallableFilebeatPackages(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range rows {
			candidates = append(candidates, &filebeatPackage{
				ID: item.ID, Arch: item.Arch, File: item.File, SHA256: item.Sha256, ServiceFileContent: item.ServiceFileContent, PlaybookID: item.UninstallPlaybookTemplateID,
			})
		}
	} else {
		rows, err := queries.ListInstallableFilebeatPackages(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range rows {
			candidates = append(candidates, &filebeatPackage{
				ID: item.ID, Arch: item.Arch, File: item.File, SHA256: item.Sha256, ServiceFileContent: item.ServiceFileContent, PlaybookID: item.InstallPlaybookTemplateID,
			})
		}
	}
	role := "安装"
	if uninstall {
		role = "卸载"
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("软件仓库没有可用的 Filebeat %s包：需存在 package_type=filebeat、package_format=tar.gz、enabled 的软件包", role)
	}
	// 按架构筛选，并把仓库里现有的架构列出来，便于排查是没建包还是架构填错。
	existingArch := map[string]bool{}
	matched := make([]*filebeatPackage, 0)
	for _, item := range candidates {
		if arch := strings.TrimSpace(item.Arch); arch != "" {
			existingArch[arch] = true
		}
		if item.Arch == arch {
			matched = append(matched, item)
		}
	}
	if len(matched) == 0 {
		archList := make([]string, 0, len(existingArch))
		for value := range existingArch {
			archList = append(archList, value)
		}
		sort.Strings(archList)
		if len(archList) == 0 {
			return nil, fmt.Errorf("软件仓库的 Filebeat 包未填写架构（arch），请在包配置里选择 amd64/arm64")
		}
		return nil, fmt.Errorf("没有匹配主机架构 %s 的 Filebeat tar.gz 包（仓库现有架构：%s）", arch, strings.Join(archList, ", "))
	}
	for _, item := range matched {
		if strings.TrimSpace(item.File) == "" {
			return nil, fmt.Errorf("Filebeat %s包缺少已上传的 tar.gz 文件，请先在软件仓库上传", role)
		}
		// 未配置 playbook 时用内置默认（见 filebeat_playbook.go），不再强制用户手写。
		return item, nil
	}
	return nil, fmt.Errorf("没有匹配主机架构 %s 的 Filebeat tar.gz 包", arch)
}

// hostArchitecture 读取资产采集的 CPU 架构并归一为仓库匹配键（amd64/arm64）。
func (handler *Handler) hostArchitecture(ctx context.Context, hostID int64) string {
	if hostID < 1 {
		return ""
	}
	row, err := generated.New(handler.db).GetHostHardware(ctx, hostID)
	if err != nil {
		return ""
	}
	return normalizeHostArch(row.Architecture.String)
}

func normalizeHostArch(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	}
	return ""
}

func playbookContent(context context.Context, pool *sql.DB, playbookID int64) (string, error) {
	// 复用 automation 域已有的取模板查询（内容是唯一需要的列）。
	playbook, err := generated.New(pool).GetAutomationPlaybook(context, playbookID)
	if err != nil {
		return "", err
	}
	return playbook.Content, nil
}

// logTargetInstallDispatch 是一次已派发的 Filebeat 安装/卸载：作业与历史行都已建好、
// 目标已置 pending，剩下的是"等作业跑完 + 收尾落库"。
type logTargetInstallDispatch struct {
	row           logTargetRow
	jobID         int64
	historyID     int64
	action        string
	desiredStatus string
	dispatchedAt  time.Time
}

// prepareLogTargetInstall 建作业/历史行并把目标置为 pending，返回待执行句柄。
//
// 拆成"准备"与"执行+收尾"两步是为了两类调用方：
//   - 单台重试（HTTP 请求）走 dispatchLogTargetInstall：准备完就返回，后台 goroutine 跑；
//   - 批量作业（worker 的批量 runner）要**等待本次安装真正结束**才能把 item 落成终态，
//     否则进度会报"已成功"而 ansible 还在跑，批量并发上限也就形同虚设。
func (handler *Handler) prepareLogTargetInstall(context context.Context, row logTargetRow) (logTargetInstallDispatch, error) {
	if handler.gateway == nil || !handler.gateway.IsOnline(row.HostName) {
		return logTargetInstallDispatch{}, fmt.Errorf("host agent is offline")
	}
	pending, _, err := logTargetPending(context, handler.db, row.ID)
	if err != nil {
		return logTargetInstallDispatch{}, err
	}
	if pending {
		return logTargetInstallDispatch{}, fmt.Errorf("已有安装/卸载任务在执行中，请等待完成或先取消")
	}
	action, desiredStatus := "install", "success"
	if !row.ManagedEnabled {
		action, desiredStatus = "uninstall", "uninstalled"
	}
	item, err := handler.pickFilebeatPackage(context, row, !row.ManagedEnabled)
	if err != nil {
		return logTargetInstallDispatch{}, err
	}
	if !item.PlaybookID.Valid {
		return logTargetInstallDispatch{}, fmt.Errorf("Filebeat %s包未配置 %s Playbook，请在软件仓库编辑该包并保存（系统会自动填入默认 Playbook），或手动填写", action, action)
	}
	content, err := playbookContent(context, handler.db, item.PlaybookID.Int64)
	if err != nil {
		return logTargetInstallDispatch{}, fmt.Errorf("Filebeat %s playbook 不存在: %w", action, err)
	}
	// systemd unit 内容从软件包配置取（创建/编辑/回填时已写默认值），空则用内置默认兜底。
	serviceFileContent := strings.TrimSpace(item.ServiceFileContent)
	if serviceFileContent == "" {
		serviceFileContent = strings.TrimSpace(filebeat.ServiceUnitContent())
	}
	extra := gin.H{"service_name": filebeatServiceName, "package_format": "tar.gz", "service_file_content": serviceFileContent}
	packageDirectory := ""
	if action == "install" {
		if strings.TrimSpace(item.File) == "" || strings.TrimSpace(item.SHA256) == "" {
			return logTargetInstallDispatch{}, fmt.Errorf("选中的 Filebeat 软件包缺少离线文件或校验和，请先在软件仓库上传")
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
	jobID, err := queries.CreateMonitorTargetJob(context, generated.CreateMonitorTargetJobParams{
		CreateTime: now, UpdateTime: now, JobID: uuid.NewString(),
		InventorySnapshot: inventoryJSON, ExtraVars: extraJSON,
		ResultSummary:        json.RawMessage(`{"message":"Filebeat install/uninstall job queued"}`),
		TaskNameSnapshot:     fmt.Sprintf("Filebeat %s", action),
		TemplateNameSnapshot: "filebeat", TemplateContentSnapshot: content,
		RunAsUserSnapshot: "", RunAsGroupSnapshot: "", WorkDirectorySnapshot: "",
		RequestedUserID: sql.NullInt32{}, RequestedUsername: "system",
	})
	if err != nil {
		return logTargetInstallDispatch{}, err
	}
	historyID, err := queries.CreateLogTargetInstallHistory(context, generated.CreateLogTargetInstallHistoryParams{
		CreateTime: now, UpdateTime: now, Action: action,
		HostIDSnapshot:          sql.NullInt32{Int32: int32(row.HostID), Valid: true},
		HostNameSnapshot:        row.HostName,
		HostIpSnapshot:          row.HostIP,
		ExporterTypeSnapshot:    "filebeat",
		SummaryMessage:          "",
		RequestedUserIDSnapshot: sql.NullInt32{}, RequestedUsernameSnapshot: "system",
		HostID:                  sql.NullInt64{Int64: row.HostID, Valid: true},
		LogCollectionTargetID:   sql.NullInt64{Int64: row.ID, Valid: true},
		AutomationJobIDSnapshot: sql.NullInt64{Int64: jobID, Valid: true},
	})
	if err != nil {
		return logTargetInstallDispatch{}, err
	}
	if err = queries.MarkLogTargetInstallPending(context, generated.MarkLogTargetInstallPendingParams{
		UpdateTime: now, ID: row.ID,
	}); err != nil {
		return logTargetInstallDispatch{}, err
	}
	return logTargetInstallDispatch{
		row: row, jobID: jobID, historyID: historyID, action: action,
		desiredStatus: desiredStatus, dispatchedAt: now,
	}, nil
}

// runLogTargetInstall 阻塞执行 ansible 作业并收尾（安装历史终态、目标的 install_status /
// runtime_status、安装成功后自动下发一次采集配置），返回收尾后的 install_status 与提示信息
// （批量作业用返回值决定这一台算成功还是失败；单台派发丢弃返回值，前端看列表）。
//
// 收尾写入用 `context.WithoutCancel`：作业执行上下文可能因超时/取消结束，但"这次安装已经结束"
// 这个事实必须落库，否则目标会永久停在 install_status=pending。
func (handler *Handler) runLogTargetInstall(ctx context.Context, dispatch logTargetInstallDispatch) (string, string) {
	row, jobID, historyID := dispatch.row, dispatch.jobID, dispatch.historyID
	if handler.jobs != nil {
		_ = handler.jobs.RunJobByID(ctx, jobID)
	}
	background, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	asyncQueries := generated.New(handler.db)
	finalStatus := "failed"
	var summary, jobStatus string
	if job, scanErr := asyncQueries.GetJobResultSummary(background, jobID); scanErr == nil {
		jobStatus = job.Status
		summary = jobResultMessage(job.ResultSummary)
		if jobStatus == "success" {
			finalStatus = dispatch.desiredStatus
		}
	}
	if summary == "" {
		summary = "Filebeat 任务执行失败"
	}
	message := summary
	if finalStatus == dispatch.desiredStatus {
		message = ""
	}
	// 时长由应用层算：历史的 create_time 就是上面的 now
	//（原实现用 TIMESTAMPDIFF(MICROSECOND,create_time,?)/1000000）。
	finishedAt := time.Now().UTC()
	succeeded := 0
	if finalStatus == "success" {
		succeeded = 1
	}
	// agent_installed 是"Filebeat 二进制已装"的持久态：安装成功置 TRUE、卸载成功置 FALSE；
	// 失败时传 NULL 保持原值（安装/卸载失败不该抹掉已知的已装/未装状态）。
	agentInstalled := sql.NullBool{}
	switch finalStatus {
	case "success":
		agentInstalled = sql.NullBool{Bool: true, Valid: true}
	case "uninstalled":
		agentInstalled = sql.NullBool{Bool: false, Valid: true}
	}
	_, _ = asyncQueries.FinishLogTargetInstallState(background, generated.FinishLogTargetInstallStateParams{
		InstallStatus: finalStatus, InstallMessage: message, InstallSucceeded: succeeded,
		AgentInstalled: agentInstalled, UpdateTime: finishedAt, ID: row.ID,
	})
	_, _ = asyncQueries.FinishLogTargetInstallHistory(background, generated.FinishLogTargetInstallHistoryParams{
		Status: finalStatus, SummaryMessage: message,
		EndTime:         sql.NullTime{Time: finishedAt, Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: finishedAt.Sub(dispatch.dispatchedAt).Seconds(), Valid: true},
		UpdateTime:      finishedAt, ID: historyID,
	})
	// 安装成功后自动下发一次采集配置（写 /etc/filebeat/filebeat.yml + inputs.d 并启动），
	// 这样"安装"即可用，不用再手动点「下发配置」；失败不影响安装结论，但把原因写进
	// last_error 供前端展示（常见：agent 版本落后不认新动作、没有默认 ES 集群、没有可下发片段）。
	if finalStatus == dispatch.desiredStatus && dispatch.action == "install" {
		if _, applyErr := handler.applyLogTargetConfigRow(background, row); applyErr != nil {
			_ = asyncQueries.SetLogTargetLastError(background, generated.SetLogTargetLastErrorParams{
				LastError: applyErr.Error(), UpdateTime: time.Now().UTC(), ID: row.ID,
			})
		}
	}
	return finalStatus, message
}

// dispatchLogTargetInstall 单台重试/安装：准备完立即返回，作业在后台跑（前端看列表与安装历史）。
func (handler *Handler) dispatchLogTargetInstall(ctx context.Context, row logTargetRow) (gin.H, error) {
	dispatch, err := handler.prepareLogTargetInstall(ctx, row)
	if err != nil {
		return nil, err
	}
	go func() {
		runContext, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_, _ = handler.runLogTargetInstall(runContext, dispatch)
	}()
	return gin.H{"id": row.ID, "action": dispatch.action, "history_id": dispatch.historyID, "job_id": dispatch.jobID}, nil
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

// dispatchLogTargetServiceControl 走 agent 通用命令通道启停/查状态。
//
// 入参是 context.Context（而非 *gin.Context）：批量启停要脱离请求执行，
// 而 *gin.Context 一离开请求生命周期就被取消，会把已在主机上执行的 systemctl 结果一起丢掉。
func (handler *Handler) dispatchLogTargetServiceControl(context context.Context, id int64, action string) (gin.H, error) {
	instanceName, err := generated.New(handler.db).GetLogTargetHostName(context, id)
	if err != nil {
		return nil, fmt.Errorf("log collection target not found")
	}
	if handler.gateway == nil || !handler.gateway.IsOnline(instanceName) {
		return nil, fmt.Errorf("host agent is offline")
	}
	// agent 通用命令通道：agent 以 root 运行，直接 systemctl，无需 sudo。
	params, _ := json.Marshal(gin.H{"command": "systemctl", "args": []string{action, filebeatServiceName + ".service"}})
	result, err := handler.gateway.Execute(context, instanceName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("filebeat-service-%d", time.Now().UnixNano()), Type: "command", Action: "filebeat_service_control", ParamsJson: string(params), TimeoutSeconds: 30})
	if err != nil {
		return nil, err
	}
	detail, err := serviceControlResult(result, action, filebeatServiceName)
	if err != nil {
		return nil, err
	}
	handler.persistLogTargetRuntimeStatus(context, id, action, result.ExitCode)
	return detail, nil
}

// persistLogTargetRuntimeStatus 把启停/查状态的真实结果落库，列表里的 Filebeat 状态列
// 读的就是这个字段；之前没人更新它，服务停了界面仍显示旧的"running"。
// systemctl status 退出码语义与 exporter 一致：0=运行中，3=已停止，其余=异常。
func (handler *Handler) persistLogTargetRuntimeStatus(context context.Context, id int64, action string, exitCode int32) {
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

// ---- 下发配置（agent 内置 configure_filebeat_output） ----

// ApplyLogTargetConfig 单条「下发配置」。
//
// 与批量下发（BatchApplyLogTargets）走**同一条**全流程 applyLogTargetConfigRow：读默认集群 →
// 渲染期望片段 → 指纹比对（一致则跳过）→ 下发 filebeat.yml（output）→ 下发 inputs.d 片段 →
// 回写 config_fingerprint。
//
// 历史缺陷：这里曾有一条只调 configure_filebeat_output 的旁路——不下发 inputs.d 片段、不写
// config_fingerprint，却把 runtime_status 置 running、清空 last_error，导致"下发成功"但主机上
// 根本没有采集片段，且指纹永远为空（漂移判定对这条路径恒为"从未下发"）。
func (handler *Handler) ApplyLogTargetConfig(context *gin.Context) {
	row, err := loadLogTarget(context, handler.db, parseID(context.Param("id")))
	if err == sql.ErrNoRows {
		response.BusinessError(context, 404, "log collection target not found", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	result, err := handler.applyLogTargetConfigRow(context, row)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, result)
}

// firstElasticsearchURL 从集群 hosts 列表（逗号分隔，可带 scheme）取第一个端点，返回完整 URL
// （Filebeat output.elasticsearch.hosts 需要带 scheme，https 表示启用 TLS）。
func firstElasticsearchURL(clusterHosts string) (string, error) {
	for _, entry := range strings.Split(clusterHosts, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if !strings.Contains(entry, "://") {
			entry = "http://" + entry
		}
		parsed, err := url.Parse(entry)
		if err != nil || parsed.Hostname() == "" {
			continue
		}
		port := parsed.Port()
		if port == "" {
			port = "9200"
		}
		return fmt.Sprintf("%s://%s:%s", parsed.Scheme, parsed.Hostname(), port), nil
	}
	return "", fmt.Errorf("Elasticsearch 集群地址无效: %s", clusterHosts)
}

// filebeatOutputParams agent 动作 configure_filebeat_output 的参数（json 键与 agent 侧约定一致）。
type filebeatOutputParams struct {
	URL       string `json:"url"`
	Username  string `json:"username"`
	Password  string `json:"password"`
	VerifyTLS bool   `json:"verify_tls"`
}

// buildFilebeatOutputParams 组装 agent configure_filebeat_output 的参数。
func buildFilebeatOutputParams(clusterHosts, username, password string, verifyTLS bool) (filebeatOutputParams, error) {
	url, err := firstElasticsearchURL(clusterHosts)
	if err != nil {
		return filebeatOutputParams{}, err
	}
	return filebeatOutputParams{URL: url, Username: username, Password: password, VerifyTLS: verifyTLS}, nil
}

// filebeatOutputIdentity 输出段对配置指纹的贡献：Elasticsearch 地址 / 账号 / TLS 开关。
// 主配置 filebeat.yml 的内容由这三者决定，所以它们变化必须能被判成配置漂移。
//
// **不含口令**：指纹会随 apply/preview 响应与主机列表返回给前端，把口令（哪怕只是哈希）
// 放进去等于给出离线猜测的口子。代价是"只改口令"不判为漂移，需人工重新下发一次。
func filebeatOutputIdentity(clusterHosts, username string, verifyTLS bool) (string, error) {
	url, err := firstElasticsearchURL(clusterHosts)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("url=%s\nusername=%s\nverify_tls=%t", url, username, verifyTLS), nil
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

func (handler *Handler) batchLogTargets(context *gin.Context, label string, run func(logTargetRow) (gin.H, error)) {
	ids, ok := ids(context)
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

// BatchStartLogTargets / BatchStopLogTargets 仍是同步批量：单台只是一次 30 秒超时的
// systemctl 调用，逐台串行的代价可接受；而"批量下发/批量安装"要跑分钟级操作，必须走
// 批量作业（见 log_batch_api.go 与 log_batch_job.go）。
func (handler *Handler) BatchStartLogTargets(context *gin.Context) {
	handler.batchServiceControl(context, "start")
}

func (handler *Handler) BatchStopLogTargets(context *gin.Context) {
	handler.batchServiceControl(context, "stop")
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

// applyLogTargetConfigRow 是全流程下发：读默认集群 → 渲染期望片段（含 output 指纹）→
// 指纹比对（一致则跳过）→ 下发 filebeat.yml → 下发 inputs.d 片段 → 回写 config_fingerprint。
//
// 入参是 context.Context：批量作业的执行在队列消费者里跑，没有 *gin.Context 可依附；
// 而 *gin.Context 不能带出请求生命周期（请求一结束就被取消，会把已下发的片段结果丢掉）。
func (handler *Handler) applyLogTargetConfigRow(context context.Context, row logTargetRow) (gin.H, error) {
	if handler.gateway == nil || !handler.gateway.IsOnline(row.HostName) {
		return nil, fmt.Errorf("host agent is offline")
	}
	queries := generated.New(handler.db)
	cluster, err := queries.GetDefaultEnabledElasticsearchCluster(context)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("没有已启用的默认 Elasticsearch 集群，请先在日志存储里配置")
	}
	if err != nil {
		return nil, err
	}
	indexPrefix := cluster.IndexPrefix
	// 输出段标识先算：指纹必须覆盖它，而它不需要解密口令。
	outputIdentity, err := filebeatOutputIdentity(cluster.Hosts, cluster.Username, cluster.VerifyTls)
	if err != nil {
		return nil, err
	}

	// 渲染服务级 inputs.d 片段（index = logs-<项目>-<业务>-<环境>-<服务>-<档位>），
	// 指纹一致则跳过，避免无谓重启 Filebeat。
	entries, instances, renderErr := handler.loadHostLogRenderInput(context, row.HostID)
	if renderErr != nil {
		return nil, fmt.Errorf("渲染 Filebeat 配置失败: %w", renderErr)
	}
	for index := range entries {
		entries[index].Prefix = indexPrefix
	}
	rendered := renderHostLogConfig(entries, instances, outputIdentity)
	if len(rendered.Fragments) == 0 {
		// 没有任何启用的日志采集配置时不报错：output 配置（filebeat.yml）仍然下发，
		// inputs 清空并启动，主机先"纳管可用"；之后开启采集再下发一次即可。
		rendered.Warnings = append(rendered.Warnings,
			"未发现启用的日志采集配置，仅下发 output.elasticsearch（请在服务的日志设置里开启采集后再下发一次）")
	}

	currentFingerprint, _ := queries.GetLogTargetConfigFingerprint(context, row.ID)
	if rendered.Fingerprint != "" && currentFingerprint == rendered.Fingerprint {
		now := time.Now().UTC()
		_ = queries.MarkLogTargetConfigApplied(context, generated.MarkLogTargetConfigAppliedParams{
			LastAppliedTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: row.ID,
		})
		return gin.H{"skipped": true, "applied_at": now, "fingerprint": rendered.Fingerprint, "service_num": rendered.ServiceNum, "warnings": rendered.Warnings}, nil
	}

	// 确实要下发时才解密口令。
	password, err := handler.secrets.Decrypt(cluster.Password)
	if err != nil {
		return nil, err
	}
	outputParams, err := buildFilebeatOutputParams(cluster.Hosts, cluster.Username, password, cluster.VerifyTls)
	if err != nil {
		return nil, err
	}
	params, _ := json.Marshal(outputParams)
	result, err := handler.gateway.Execute(context, row.HostName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("filebeat-output-%d", time.Now().UnixNano()), Type: "custom", Action: "configure_filebeat_output", ParamsJson: string(params), TimeoutSeconds: 60})
	if err != nil {
		return nil, err
	}
	if result.Status != "success" {
		reason := firstNonEmpty(strings.TrimSpace(result.ErrorMessage), strings.TrimSpace(result.Stderr), strings.TrimSpace(result.Stdout))
		return nil, fmt.Errorf("Filebeat 输出配置下发失败: %s", reason)
	}
	// 片段内容全部由 backend 渲染，agent 只落盘 + 校验 + 重启（见 apply_filebeat_config）。
	fragmentParams, _ := json.Marshal(gin.H{"files": rendered.Fragments, "restart": "true"})
	fragmentResult, err := handler.gateway.Execute(context, row.HostName, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("filebeat-fragments-%d", time.Now().UnixNano()), Type: "custom", Action: "apply_filebeat_config", ParamsJson: string(fragmentParams), TimeoutSeconds: 120})
	if err != nil {
		return nil, err
	}
	if fragmentResult.Status != "success" {
		reason := firstNonEmpty(strings.TrimSpace(fragmentResult.ErrorMessage), strings.TrimSpace(fragmentResult.Stderr), strings.TrimSpace(fragmentResult.Stdout))
		return nil, fmt.Errorf("Filebeat 片段下发失败: %s", reason)
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

// ---- 批量创建（纳管 Filebeat） ----

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
	installTargetIDs := make([]int64, 0, len(input.HostIDs))
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
		success++
		results = append(results, gin.H{"host_id": hostID, "host": label, "ok": true, "message": ""})
		if input.InstallNow {
			installTargetIDs = append(installTargetIDs, targetID)
		}
	}
	result := gin.H{"total": len(results), "success": success, "failed": len(results) - success, "results": results}
	// 「纳管并立即安装」不再逐台起无上限的 goroutine（1000 台会把 agent 侧打满），
	// 而是把新建的目标交给一个批量作业：入队 + 有界并发 + 进度可在作业详情里查。
	if len(installTargetIDs) > 0 {
		batch, err := handler.createLogBatchJob(context, LogBatchActionInstall, installTargetIDs, requestedUsername(context))
		if err != nil {
			result["install_error"] = err.Error()
		} else {
			result["install_batch"] = batch
		}
	}
	response.Success(context, result)
}
