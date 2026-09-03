package assets

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Agent 在线自更新：经 gRPC 文件通道推送新二进制到主机，agent 调内置 apply_agent_update
// 自替换并重启；重启后回查重连状态作为最终结论。语义与 Django assets/agent_install_service.py
// 的 run_agent_update_via_grpc 一致。全新主机走 operation=install：SSH 凭证 + 本机
// ansible-playbook 执行 agent_install.yml 引导安装，语义与 run_agent_install_job 一致，
// 见 agent_install.go。env/unit 配置统一以磁盘上的 agent_install.yml 为唯一来源（agent_playbook.go），
// 找不到或被改坏时入口直接报错，无内嵌兜底。

const (
	agentUpdateRemoteDir  = "/var/lib/dj-agent/update"
	agentUpdateRemoteFile = "dj-agent.new"
	agentGrpcAdvertiseKey = "sys.assets.agent.grpc_advertise_addr"
)

type agentInstallInput struct {
	HostIDs      []int64 `json:"host_ids"`
	Operation    string  `json:"operation"`
	CredentialID int64   `json:"credential_id"`
}

type agentUpdateHost struct {
	ID         int64
	HostName   string
	HostIP     string
	AgentID    string
	AgentJobID string
	LogID      int64
}

func (handler *Handler) AgentInstall(context *gin.Context) {
	var input agentInstallInput
	if err := context.ShouldBindJSON(&input); err != nil || len(input.HostIDs) == 0 {
		response.BusinessError(context, 400, "host_ids不能为空数组", nil)
		return
	}
	operation := strings.ToLower(strings.TrimSpace(input.Operation))
	if operation == "" {
		operation = "install"
	}
	if operation != "update" && operation != "install" {
		response.BusinessError(context, 400, "operation仅支持install或update", nil)
		return
	}
	var credential db.AssetsCredential
	credentialID := input.CredentialID
	if operation == "install" {
		// 安装是全新主机的引导过程，agent 还没起来，只能走 SSH；更新复用已连通的 gRPC 通道，不需要 SSH 凭证。
		if credentialID <= 0 {
			response.BusinessError(context, 400, "credential_id必须是整数", nil)
			return
		}
		row, err := handler.service.repository.GetCredential(context, credentialID)
		if err != nil {
			response.BusinessError(context, 400, "SSH 凭证不存在", nil)
			return
		}
		credential = row
	}

	hosts := make([]agentUpdateHost, 0, len(input.HostIDs))
	placeholders := make([]string, 0, len(input.HostIDs))
	arguments := make([]any, 0, len(input.HostIDs))
	for _, id := range input.HostIDs {
		if id <= 0 {
			response.BusinessError(context, 400, "host_ids 必须是正整数", nil)
			return
		}
		placeholders = append(placeholders, "?")
		arguments = append(arguments, id)
	}
	rows, err := handler.service.repository.pool.QueryContext(context,
		`SELECT id,COALESCE(instance_name,''),COALESCE(ip,''),COALESCE(agent_id,'') FROM assets_host WHERE id IN (`+strings.Join(placeholders, ",")+`) ORDER BY id`, arguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var host agentUpdateHost
		if err = rows.Scan(&host.ID, &host.HostName, &host.HostIP, &host.AgentID); err != nil {
			response.Error(context, err)
			return
		}
		hosts = append(hosts, host)
	}
	if err = rows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	if len(hosts) != len(input.HostIDs) {
		response.BusinessError(context, 400, "部分主机不存在，请刷新后重试", nil)
		return
	}
	for _, host := range hosts {
		if operation == "update" {
			if strings.TrimSpace(host.AgentID) == "" {
				response.BusinessError(context, 400, fmt.Sprintf("主机 %s 未绑定 Agent，更新操作仅支持已绑定 Agent 的主机", host.HostName), nil)
				return
			}
			if !handler.gateway.IsOnline(host.AgentID) {
				response.BusinessError(context, 400, fmt.Sprintf("主机 %s 的 Agent 离线，更新操作仅支持当前在线的主机", host.HostName), nil)
				return
			}
		}
	}
	if err = handler.rejectActiveAgentJobs(context, hosts); err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}

	binary, err := handler.loadAgentBinary()
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	playbook, err := handler.loadAgentInstallPlaybook()
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	contents, err := playbookCopyContents(playbook)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	envTemplate, unitTemplate, err := playbookAgentTemplates(contents)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	advertised, err := handler.agentAdvertiseAddr()
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}

	userID, username := int32(0), "system"
	if claims, ok := identity.ClaimsFromContext(context); ok {
		userID, username = claims.UserID, claims.Username
	}

	// 与 Django 一致：创建 RUNNING 状态的自动化执行记录，主机明细挂在 target log 上，
	// 前端提交成功后跳转 /sys/automation/logs?job_id=<automation_job_id> 查看进度。
	now := time.Now().UTC()
	extraVars, taskName, templateName, templateContent := `{"operation":"update"}`, "Agent 更新", "dj-agent 在线自更新", ""
	if operation == "install" {
		extraVars = fmt.Sprintf(`{"operation":"install","credential_id":%d}`, credentialID)
		taskName, templateName, templateContent = "Agent 安装", "dj-agent Ansible Playbook", playbook
	}
	inventory := gin.H{"hosts": func() []gin.H {
		items := make([]gin.H, 0, len(hosts))
		for _, host := range hosts {
			items = append(items, gin.H{"host_id": host.ID, "name": host.HostName, "ip": host.HostIP})
		}
		return items
	}()}
	inventoryJSON, _ := json.Marshal(inventory)
	result, err := handler.service.repository.pool.ExecContext(context, `INSERT INTO automation_execution_job
		(create_time,update_time,remark,job_id,status,trigger_type,inventory_snapshot,extra_vars,result_summary,
		 task_name_snapshot,template_name_snapshot,template_content_snapshot,`+"`limit`"+`,run_as_user_snapshot,run_as_group_snapshot,work_directory_snapshot,
		 requested_user_id,requested_username,start_time)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		now, now, nil, uuid.NewString(), "running", "manual", inventoryJSON, extraVars,
		`{"message":"Agent update running"}`, taskName, templateName, templateContent, "", "", "", "", nullableUserID(userID), username, now)
	if err != nil {
		response.Error(context, err)
		return
	}
	executionID, _ := result.LastInsertId()

	for index := range hosts {
		host := &hosts[index]
		host.AgentJobID = fmt.Sprintf("install-agent-%s", uuid.NewString()[:16])
		jobType, jobParams := "grpc", `{"operation":"update"}`
		jobAgentID := host.AgentID
		if operation == "install" {
			jobType = "ansible"
			jobParams = fmt.Sprintf(`{"credential_id":%d,"operation":"install"}`, credentialID)
			// 与 Django 一致：全新主机没有 agent_id，作业行回填 host-<id> 占位。
			if strings.TrimSpace(jobAgentID) == "" {
				jobAgentID = fmt.Sprintf("host-%d", host.ID)
			}
		}
		jobResult, err := handler.service.repository.pool.ExecContext(context, `INSERT INTO assets_agent_job
			(create_time,update_time,remark,job_id,agent_id,job_type,action,params,timeout_seconds,status,result_data,error_message,host_id,exit_code,stderr,stdout)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			now, now, nil, host.AgentJobID, jobAgentID, jobType, "install_agent",
			jobParams, 300, "queued", `{}`, "", sql.NullInt64{Int64: host.ID, Valid: true}, 0, "", "")
		if err != nil {
			response.Error(context, err)
			return
		}
		jobRowID, _ := jobResult.LastInsertId()
		_ = jobRowID
		logResult, err := handler.service.repository.pool.ExecContext(context, `INSERT INTO automation_execution_host_log
			(create_time,update_time,remark,host_id_snapshot,host_name_snapshot,host_ip_snapshot,agent_job_id,status,exit_code,stdout,stderr,error_message,result_data,host_id,job_id)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			now, now, nil, host.ID, host.HostName, host.HostIP, host.AgentJobID, "queued", 0, "", "", "", `{}`, sql.NullInt64{Int64: host.ID, Valid: true}, executionID)
		if err != nil {
			response.Error(context, err)
			return
		}
		host.LogID, _ = logResult.LastInsertId()
	}

	go func() {
		if operation == "install" {
			handler.runAgentInstalls(binary, advertised, playbook, hosts, credential, executionID, userID, username)
			return
		}
		handler.runAgentUpdates(binary, advertised, envTemplate, unitTemplate, hosts, executionID, userID, username)
	}()

	jobs := make([]gin.H, 0, len(hosts))
	for _, host := range hosts {
		jobs = append(jobs, gin.H{"job_id": host.AgentJobID, "host_id": host.ID})
	}
	response.Success(context, gin.H{"automation_job_id": executionID, "jobs": jobs})
}

func (handler *Handler) rejectActiveAgentJobs(context *gin.Context, hosts []agentUpdateHost) error {
	placeholders := make([]string, 0, len(hosts))
	arguments := make([]any, 0, len(hosts))
	for _, host := range hosts {
		placeholders = append(placeholders, "?")
		arguments = append(arguments, host.ID)
	}
	staleBefore := time.Now().UTC().Add(-30 * time.Second)
	now := time.Now().UTC()
	// Django 语义：先把超过 30 秒未动的 queued/running 安装任务标记失败，再拦截仍然活跃的任务。
	// 占位符顺序：finished_at=?, update_time=?, host_id IN (...), update_time < ?
	updateArgs := append([]any{now, now}, arguments...)
	updateArgs = append(updateArgs, staleBefore)
	if _, err := handler.service.repository.pool.ExecContext(context, `UPDATE assets_agent_job
		SET status='failed',error_message='Agent 任务执行进程已失联，请重新提交',exit_code=1,finished_at=?,update_time=?
		WHERE host_id IN (`+strings.Join(placeholders, ",")+`) AND action='install_agent'
		  AND status IN ('queued','running') AND update_time < ?`,
		updateArgs...); err != nil {
		return err
	}
	var active int
	if err := handler.service.repository.pool.QueryRowContext(context, `SELECT COUNT(*) FROM assets_agent_job
		WHERE host_id IN (`+strings.Join(placeholders, ",")+`) AND action='install_agent' AND status IN ('queued','running')`, arguments...).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return fmt.Errorf("所选主机已有 Agent 任务执行中，请等待完成或先取消")
	}
	return nil
}

func (handler *Handler) loadAgentBinary() ([]byte, error) {
	binaryPath, err := filepath.Abs(filepath.Join("..", "dj_agent", "bin", "dj-agent"))
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("Agent 二进制不存在: %s", binaryPath)
	}
	// Django 同款双标记校验：拒绝旧 RabbitMQ 构建产物，也拒绝缺少当前 gRPC 配置标记的未知版本。
	if bytes.Contains(data, []byte("connect rabbitmq failed")) {
		return nil, fmt.Errorf("Agent 二进制仍是旧 RabbitMQ 版本，请先执行 CGO_ENABLED=0 go build -trimpath -o bin/dj-agent ./cmd/agent 后重试")
	}
	if !bytes.Contains(data, []byte("DJ_AGENT_GRPC_FILE_ADDR")) {
		return nil, fmt.Errorf("Agent 二进制缺少当前 gRPC 配置标记，拒绝部署未知版本")
	}
	return data, nil
}

func (handler *Handler) agentAdvertiseAddr() (string, error) {
	var value string
	err := handler.service.repository.pool.QueryRowContext(context.Background(),
		`SELECT value FROM sys_config WHERE `+"`key`"+`=?`, agentGrpcAdvertiseKey).Scan(&value)
	if err != nil {
		return "", fmt.Errorf("未配置“Agent gRPC 对外地址”（sys.assets.agent.grpc_advertise_addr），请先在系统参数中填写")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("未配置“Agent gRPC 对外地址”，请先在系统参数中填写")
	}
	return value, nil
}

// agentGRPCAddrForHost 与 Django _agent_grpc_addr_for_host 一致：与对外地址同机的主机走回环地址。
func agentGRPCAddrForHost(hostIP, advertisedAddr string) (string, error) {
	advertisedHost, advertisedPort, found := strings.Cut(advertisedAddr, ":")
	if !found || advertisedHost == "" || advertisedPort == "" {
		return "", fmt.Errorf("“Agent gRPC 对外地址”格式错误，应为 主机:端口")
	}
	if strings.EqualFold(strings.TrimSpace(hostIP), strings.TrimSpace(advertisedHost)) {
		return fmt.Sprintf("127.0.0.1:%s", advertisedPort), nil
	}
	return advertisedAddr, nil
}

func (handler *Handler) runAgentUpdates(binary []byte, advertisedAddr, envTemplate, unitTemplate string, hosts []agentUpdateHost, executionID int64, userID int32, username string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	background := context.Background()

	successCount := 0
	for _, host := range hosts {
		ok := handler.runAgentUpdateOnce(ctx, background, host, binary, advertisedAddr, envTemplate, unitTemplate, executionID)
		if ok {
			successCount++
		}
	}
	failed := len(hosts) - successCount
	status := "success"
	message := "Agent 在线更新完成"
	if successCount == 0 {
		status = "failed"
		message = "Agent 在线更新失败"
	} else if failed > 0 {
		message = fmt.Sprintf("Agent 更新完成：成功 %d 台，失败 %d 台", successCount, failed)
	}
	now := time.Now().UTC()
	summary := fmt.Sprintf(`{"message":%q,"succeeded":%d,"failed":%d}`, message, successCount, failed)
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_job
		SET status=?,end_time=?,duration_seconds=TIMESTAMPDIFF(MICROSECOND,start_time,?)/1000000,result_summary=?,update_time=? WHERE id=?`,
		status, now, now, summary, now, executionID)
	_ = userID
	_ = username
}

func (handler *Handler) runAgentUpdateOnce(ctx context.Context, background context.Context, host agentUpdateHost, binary []byte, advertisedAddr, envTemplate, unitTemplate string, executionID int64) bool {
	now := time.Now().UTC()
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job SET status='running',picked_at=?,update_time=? WHERE job_id=?`, now, now, host.AgentJobID)
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log SET status='running',update_time=? WHERE id=?`, now, host.LogID)

	fail := func(message string, exitCode int32, stdout, stderr string) bool {
		now := time.Now().UTC()
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job
			SET status='failed',error_message=?,exit_code=?,stdout=?,stderr=?,finished_at=?,update_time=? WHERE job_id=?`,
			message, exitCode, stdout, stderr, now, now, host.AgentJobID)
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log
			SET status='failed',error_message=?,exit_code=?,stdout=?,stderr=?,update_time=? WHERE id=?`,
			message, exitCode, stdout, stderr, now, host.LogID)
		return false
	}

	grpcAddr, err := agentGRPCAddrForHost(host.HostIP, advertisedAddr)
	if err != nil {
		return fail(err.Error(), 1, "", "")
	}
	if handler.gateway == nil || !handler.gateway.IsOnline(host.AgentID) {
		return fail("agent 已离线，无法走在线更新", 1, "", "")
	}
	// env/unit 内容以 agent_install.yml 中的 copy 模板为唯一来源（入口处已校验）。
	envContent := renderAgentEnvTemplate(envTemplate, host.AgentID, grpcAddr)
	unitContent := unitTemplate + "\n"

	if _, err = handler.gateway.MakeDirectory(ctx, host.AgentID, "/var/lib/dj-agent", "update"); err != nil {
		return fail("创建远端 update 目录失败: "+err.Error(), 1, "", "")
	}
	if _, err = handler.gateway.WriteFile(ctx, host.AgentID, "/var/lib/dj-agent/update", agentUpdateRemoteFile, bytes.NewReader(binary)); err != nil {
		return fail("推送 Agent 二进制失败: "+err.Error(), 1, "", "")
	}
	params, _ := json.Marshal(gin.H{"env_content": envContent, "unit_content": unitContent})
	result, err := handler.gateway.Execute(ctx, host.AgentID, &pb.AutomationExecuteRequest{
		JobId:          fmt.Sprintf("agent-update-%s", host.AgentJobID),
		Type:           "custom",
		Action:         "apply_agent_update",
		ParamsJson:     string(params),
		TimeoutSeconds: 30,
	})
	if err != nil {
		return fail("下发自更新指令失败: "+err.Error(), 1, "", "")
	}
	if result.Status != "success" || (result.ExitCode != 0 && result.ExitCode != int32(0)) {
		reason := firstNonEmpty(strings.TrimSpace(result.ErrorMessage), strings.TrimSpace(result.Stderr), strings.TrimSpace(result.Stdout))
		return fail("自更新失败: "+reason, result.ExitCode, result.Stdout, result.Stderr)
	}

	// agent 重启期间会短暂断线；轮询等它用新版本重新上线（略长于 agent 侧重启窗口）。
	reconnected := false
	for attempt := 0; attempt < 15; attempt++ {
		time.Sleep(time.Second)
		if handler.gateway.IsOnline(host.AgentID) {
			reconnected = true
			break
		}
	}
	now = time.Now().UTC()
	finalStatus, exitCode, message := "success", int64(0), ""
	if !reconnected {
		finalStatus, exitCode = "failed", int64(1)
		message = "自更新已下发，但重启后 Agent 未重新连接，请人工检查该主机"
	}
	resultData := fmt.Sprintf(`{"host_id":%d,"agent_id":%q,"operation":"update","agent_connected":%t}`, host.ID, host.AgentID, reconnected)
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job
		SET status=?,exit_code=?,error_message=?,result_data=?,finished_at=?,update_time=? WHERE job_id=?`,
		finalStatus, exitCode, message, resultData, now, now, host.AgentJobID)
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log
		SET status=?,exit_code=?,error_message=?,update_time=? WHERE id=?`,
		finalStatus, exitCode, message, now, host.LogID)
	return reconnected
}

func nullableUserID(value int32) any {
	if value == 0 {
		return nil
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
