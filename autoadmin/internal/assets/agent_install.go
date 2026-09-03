package assets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// Agent SSH 引导安装：全新主机 agent 尚未上线，用主机上配置的 SSH 凭证在本机执行
// ansible-playbook（agent_install.yml）完成安装。语义与 Django assets/agent_install_service.py
// 的 run_agent_install_job 一致：流式回写 stdout、解析 recap、等待 agent 回连 gRPC、
// 成功后回填 host.agent_id。playbook 运行时从磁盘加载（见 agent_playbook.go），
// 找不到时入口直接报错，无内嵌兜底。

const (
	agentInstallTimeoutSeconds = 300
	agentAuthTypePassword      = 1
	agentAuthTypeSSHKey        = 2
)

var ansibleRecapPattern = regexp.MustCompile(`\b(ok|changed|unreachable|failed|skipped)=(\d+)`)

func (handler *Handler) runAgentInstalls(binary []byte, advertisedAddr, playbook string, hosts []agentUpdateHost, credential db.AssetsCredential, executionID int64, userID int32, username string) {
	// 与 Django 一致：每台主机一个独立执行流并发跑，互不阻塞。
	results := make([]bool, len(hosts))
	var group sync.WaitGroup
	for index := range hosts {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index] = handler.runAgentInstallOnce(hosts[index], binary, advertisedAddr, playbook, credential)
		}(index)
	}
	group.Wait()

	successCount := 0
	for _, ok := range results {
		if ok {
			successCount++
		}
	}
	failed := len(hosts) - successCount
	status := "success"
	message := "Agent 安装完成"
	if successCount == 0 {
		status = "failed"
		message = "Agent 安装失败"
	} else if failed > 0 {
		message = fmt.Sprintf("Agent 安装完成：成功 %d 台，失败 %d 台", successCount, failed)
	}
	now := time.Now().UTC()
	summary := fmt.Sprintf(`{"message":%q,"succeeded":%d,"failed":%d}`, message, successCount, failed)
	_, _ = handler.service.repository.pool.ExecContext(context.Background(), `UPDATE automation_execution_job
		SET status=?,end_time=?,duration_seconds=TIMESTAMPDIFF(MICROSECOND,start_time,?)/1000000,result_summary=?,update_time=? WHERE id=?`,
		status, now, now, summary, now, executionID)
	_ = userID
	_ = username
}

func (handler *Handler) runAgentInstallOnce(host agentUpdateHost, binary []byte, advertisedAddr, playbook string, credential db.AssetsCredential) bool {
	background := context.Background()
	now := time.Now().UTC()
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job SET status='running',picked_at=?,update_time=? WHERE job_id=?`, now, now, host.AgentJobID)
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log SET status='running',update_time=? WHERE id=?`, now, host.LogID)

	fail := func(message string, exitCode int64, stdout, stderr string) bool {
		now := time.Now().UTC()
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job
			SET status='failed',error_message=?,exit_code=?,stdout=?,stderr=?,finished_at=?,update_time=? WHERE job_id=?`,
			message, exitCode, stdout, stderr, now, now, host.AgentJobID)
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log
			SET status='failed',error_message=?,exit_code=?,stdout=?,stderr=?,update_time=? WHERE id=?`,
			message, exitCode, stdout, stderr, now, host.LogID)
		return false
	}

	agentID := strings.TrimSpace(host.AgentID)
	if agentID == "" {
		agentID = fmt.Sprintf("host-%d", host.ID)
	}
	grpcAddr, err := agentGRPCAddrForHost(host.HostIP, advertisedAddr)
	if err != nil {
		return fail(err.Error(), 1, "", "")
	}
	advertisedHost, _, _ := strings.Cut(advertisedAddr, ":")
	isLocal := strings.EqualFold(strings.TrimSpace(host.HostIP), strings.TrimSpace(advertisedHost))

	username := strings.TrimSpace(credential.Username)
	if username == "" {
		username = "root"
	}
	isNonRoot := strings.ToLower(username) != "root"

	directory, err := os.MkdirTemp("", "autoadmin-agent-install-")
	if err != nil {
		return fail("创建临时工作目录失败: "+err.Error(), 1, "", "")
	}
	defer os.RemoveAll(directory)

	variables := gin.H{
		"ansible_host": host.HostIP,
		"ansible_user": username,
		"ansible_port": credential.Port,
	}
	switch credential.AuthType {
	case agentAuthTypeSSHKey:
		privateKey := ""
		if credential.PrivateKey.Valid {
			privateKey, err = handler.service.encryptor.Decrypt(credential.PrivateKey.String)
		}
		if err != nil || strings.TrimSpace(privateKey) == "" {
			return fail("SSH Key 凭证为空", 1, "", "")
		}
		keyPath := directory + "/ssh_key"
		if err = os.WriteFile(keyPath, []byte(privateKey), 0o600); err != nil {
			return fail("写入 SSH 私钥失败: "+err.Error(), 1, "", "")
		}
		variables["ansible_ssh_private_key_file"] = keyPath
	default:
		password := ""
		if credential.Password.Valid {
			password, err = handler.service.encryptor.Decrypt(credential.Password.String)
		}
		if err != nil || password == "" {
			return fail("SSH 密码凭证为空", 1, "", "")
		}
		variables["ansible_password"] = password
		if isNonRoot {
			variables["ansible_become_password"] = password
		}
	}
	if isNonRoot {
		variables["ansible_become"] = true
		variables["ansible_become_method"] = "sudo"
		variables["ansible_become_user"] = "root"
	}
	inventory, _ := json.Marshal(gin.H{"all": gin.H{"hosts": gin.H{"target": variables}}})
	inventoryPath := directory + "/inventory.json"
	if err = os.WriteFile(inventoryPath, inventory, 0o600); err != nil {
		return fail("写入 inventory 失败: "+err.Error(), 1, "", "")
	}

	binaryPath := directory + "/dj-agent"
	if err = os.WriteFile(binaryPath, binary, 0o755); err != nil {
		return fail("写入 Agent 二进制失败: "+err.Error(), 1, "", "")
	}
	playbookPath := directory + "/agent_install.yml"
	if err = os.WriteFile(playbookPath, []byte(playbook), 0o600); err != nil {
		return fail("写入 Playbook 失败: "+err.Error(), 1, "", "")
	}

	timeout := agentInstallTimeoutSeconds
	commandCtx, cancel := context.WithTimeout(background, time.Duration(timeout)*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, "ansible-playbook", "-i", inventoryPath, "--timeout", "10",
		"-e", "dj_agent_binary_source="+binaryPath,
		"-e", "dj_agent_id="+agentID,
		"-e", "dj_agent_grpc_addr="+grpcAddr,
		"-e", "dj_agent_is_local="+strconv.FormatBool(isLocal),
		playbookPath)
	command.Dir = directory
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	}
	command.WaitDelay = 3 * time.Second

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return fail("创建输出管道失败: "+err.Error(), 1, "", "")
	}
	command.Stdout, command.Stderr = stdoutWriter, stdoutWriter
	if err = command.Start(); err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		return fail("启动 ansible-playbook 失败: "+err.Error(), 1, "", "")
	}
	_ = stdoutWriter.Close()

	var outputLock sync.Mutex
	var output strings.Builder
	persist := func() {
		outputLock.Lock()
		live := output.String()
		outputLock.Unlock()
		now := time.Now().UTC()
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job SET stdout=?,update_time=? WHERE job_id=?`, live, now, host.AgentJobID)
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log SET stdout=?,update_time=? WHERE id=?`, live, now, host.LogID)
	}
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buffer := make([]byte, 65536)
		for {
			count, readErr := stdoutReader.Read(buffer)
			if count > 0 {
				outputLock.Lock()
				output.Write(buffer[:count])
				outputLock.Unlock()
			}
			if readErr != nil {
				return
			}
		}
	}()
	tickerDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-tickerDone:
				return
			case <-ticker.C:
				persist()
			}
		}
	}()

	waitErr := command.Wait()
	close(tickerDone)
	_ = stdoutReader.Close()
	<-readerDone
	stdout := output.String()
	persist()

	if commandCtx.Err() == context.DeadlineExceeded {
		now := time.Now().UTC()
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job
			SET status='timeout',error_message='Ansible Agent 安装任务超时',exit_code=124,stdout=?,stderr='',finished_at=?,update_time=? WHERE job_id=?`,
			stdout, now, now, host.AgentJobID)
		_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log
			SET status='failed',error_message='Ansible Agent 安装任务超时',exit_code=124,stdout=?,update_time=? WHERE id=?`,
			stdout, now, host.LogID)
		return false
	}

	exitCode := 0
	if waitErr != nil {
		var exitError *exec.ExitError
		if errors.As(waitErr, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			return fail("执行 ansible-playbook 失败: "+waitErr.Error(), 1, stdout, "")
		}
	}

	recap := parseAnsibleRecap(stdout)
	finalStatus, message := "success", ""
	if exitCode != 0 || recap["failed"] > 0 || recap["unreachable"] > 0 {
		finalStatus, message = "failed", "Ansible 安装失败"
	}
	agentConnected := false
	if finalStatus == "success" {
		agentConnected = handler.waitForAgentConnection(agentID, 10*time.Second)
		if !agentConnected {
			finalStatus = "failed"
			message = "Ansible 安装完成，但 Agent 未连接到后端 gRPC 服务"
		}
	}
	resultData, _ := json.Marshal(gin.H{
		"host_id": host.ID, "agent_id": agentID, "operation": "install",
		"ansible_recap": recap, "agent_connected": agentConnected,
	})
	now = time.Now().UTC()
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE assets_agent_job
		SET status=?,exit_code=?,error_message=?,result_data=?,finished_at=?,update_time=? WHERE job_id=?`,
		finalStatus, exitCode, message, string(resultData), now, now, host.AgentJobID)
	_, _ = handler.service.repository.pool.ExecContext(background, `UPDATE automation_execution_host_log
		SET status=?,exit_code=?,error_message=?,result_data=?,update_time=? WHERE id=?`,
		finalStatus, exitCode, message, string(resultData), now, host.LogID)

	if finalStatus == "success" && strings.TrimSpace(host.AgentID) == "" {
		_, _ = handler.service.repository.pool.ExecContext(background,
			`UPDATE assets_host SET agent_id=?,update_time=? WHERE id=? AND (agent_id IS NULL OR agent_id='')`,
			agentID, now, host.ID)
	}
	return finalStatus == "success"
}

func (handler *Handler) waitForAgentConnection(agentID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if handler.gateway != nil && handler.gateway.IsOnline(agentID) {
			return true
		}
		if time.Now().After(deadline) {
			return handler.gateway != nil && handler.gateway.IsOnline(agentID)
		}
		time.Sleep(time.Second)
	}
}

func parseAnsibleRecap(output string) map[string]int {
	recap := map[string]int{"ok": 0, "changed": 0, "unreachable": 0, "failed": 0, "skipped": 0}
	match := ansibleRecapPattern.FindStringIndex(output)
	if match == nil {
		return recap
	}
	line := output[match[0]:]
	if newline := strings.IndexByte(line, '\n'); newline >= 0 {
		line = line[:newline]
	}
	for _, pair := range ansibleRecapPattern.FindAllStringSubmatch(line, -1) {
		if value, err := strconv.Atoi(pair[2]); err == nil {
			recap[pair[1]] = value
		}
	}
	return recap
}
