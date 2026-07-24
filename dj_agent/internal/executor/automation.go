package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/apenella/go-ansible/pkg/options"
	"github.com/apenella/go-ansible/pkg/playbook"
	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

const fallbackAutomationWorkDir = "/tmp"

// runAutomationTask 执行自动化任务（支持 Shell 脚本和 Ansible Playbook）
func (e *Executor) runAutomationTask(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	result := protocol.JobResult{
		JobID:     job.JobID,
		Type:      job.Type,
		Action:    job.Action,
		StartedAt: started,
	}

	templateType := strings.TrimSpace(toString(job.Params["template_type"]))
	templateContent := toString(job.Params["template_content"])
	shellParameters := strings.TrimSpace(toString(job.Params["shell_parameters"]))
	envVars := toStringMap(job.Params["env_vars"])
	extraVars := toAnyMap(job.Params["extra_vars"])
	// 执行身份：dj-agent 进程本身以 root 运行，run_as_user/run_as_group 通过 setuid/setgid 降权到
	// 目标系统用户/组执行（见 resolveRunAsCredential），不再使用 ansible become/sudo 机制。
	runAsUser := strings.TrimSpace(toString(job.Params["run_as_user"]))
	runAsGroup := strings.TrimSpace(toString(job.Params["run_as_group"]))
	// 工作目录通过 params.work_dir 传递（而非 protocol.Job 顶层 WorkDir 字段），与 backend
	// automation.agent_grpc_runner 的 extra params 约定保持一致；未指定时回退 /tmp。
	jobWorkDir := strings.TrimSpace(toString(job.Params["work_dir"]))

	// 默认为 shell_script 类型
	if templateType == "" {
		templateType = "shell_script"
	}

	// 验证模板类型
	if templateType != "shell_script" && templateType != "playbook" {
		finished := time.Now()
		result.Status = protocol.StatusFailed
		result.ExitCode = 1
		result.Error = fmt.Sprintf("unsupported template_type for agent mode: %s", templateType)
		result.FinishedAt = finished
		result.CostMS = finished.Sub(started).Milliseconds()
		return result
	}

	// 验证模板内容
	if strings.TrimSpace(templateContent) == "" {
		finished := time.Now()
		result.Status = protocol.StatusFailed
		result.ExitCode = 1
		result.Error = "template_content is empty"
		result.FinishedAt = finished
		result.CostMS = finished.Sub(started).Milliseconds()
		return result
	}

	// 解析执行身份：run_as_user 必填，缺失/无法解析直接失败，避免自动化任务静默以 root 身份执行。
	credential, homeDir, credErr := resolveRunAsCredential(runAsUser, runAsGroup)
	if credErr != nil {
		finished := time.Now()
		result.Status = protocol.StatusFailed
		result.ExitCode = 1
		result.Error = fmt.Sprintf("failed to resolve run_as identity: %v", credErr)
		result.FinishedAt = finished
		result.CostMS = finished.Sub(started).Milliseconds()
		return result
	}

	var cmd *exec.Cmd
	var tempResources []string // 需要清理的临时文件/目录

	var buildErr error
	if templateType == "shell_script" {
		cmd, tempResources, buildErr = buildShellScriptCommand(ctx, job.JobID, templateContent, shellParameters, envVars, jobWorkDir, runAsUser, homeDir, credential)
		if cmd == nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			// buildErr 携带具体失败步骤（mkdir/chown/write/...）及底层系统调用错误，直接回传给
			// backend 作业结果，避免运维必须登录目标主机查看 dj-agent 日志才能定位原因。
			result.Error = fmt.Sprintf("failed to build shell script command: %v", buildErr)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
	} else {
		cmd, tempResources, buildErr = buildPlaybookCommand(ctx, job.JobID, templateContent, envVars, extraVars, jobWorkDir, runAsUser, homeDir, credential)
		if cmd == nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			// buildErr 携带具体失败步骤（mkdir/chown/write/生成 ansible-playbook 参数等）及底层
			// 系统调用错误，直接回传给 backend 作业结果，避免运维必须登录目标主机查看
			// dj-agent 日志才能定位原因。
			result.Error = fmt.Sprintf("failed to build playbook command: %v", buildErr)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
	}

	// 清理临时资源
	defer func() {
		for _, path := range tempResources {
			_ = os.RemoveAll(path)
		}
	}()

	// 执行命令
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	finished := time.Now()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.FinishedAt = finished
	result.CostMS = finished.Sub(started).Milliseconds()
	result.ExitCode = readExitCode(err)

	switch {
	case err == nil:
		result.Status = protocol.StatusSuccess
		return result
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		result.Status = protocol.StatusTimeout
		result.Error = ctx.Err().Error()
		return result
	case errors.Is(ctx.Err(), context.Canceled):
		result.Status = protocol.StatusCanceled
		result.Error = ctx.Err().Error()
		return result
	default:
		result.Status = protocol.StatusFailed
		result.Error = err.Error()
		return result
	}
}

// resolveRunAsCredential 解析 run_as_user/run_as_group 对应的 uid/gid/附加组，用于把子进程
// （shell 脚本或 ansible-playbook）通过 exec.Cmd.SysProcAttr.Credential 降权到目标系统用户执行。
// dj-agent 进程本身以 root 运行，这是任务实际执行身份收敛到普通用户的唯一关卡：runAsUser 必填，
// 留空视为配置错误直接失败，避免自动化任务静默以 root 身份执行。
// runAsGroup 留空时使用该用户的主组；附加组会保留 runAsUser 本身所属的其它系统组（如 docker），
// 保证降权后仍具备其原本的组权限。
func resolveRunAsCredential(runAsUser, runAsGroup string) (*syscall.Credential, string, error) {
	if strings.TrimSpace(runAsUser) == "" {
		return nil, "", fmt.Errorf("run_as_user is required")
	}

	u, lookupErr := user.Lookup(runAsUser)
	if lookupErr != nil {
		return nil, "", fmt.Errorf("lookup run_as_user %q failed: %w", runAsUser, lookupErr)
	}

	uid, uidErr := strconv.ParseUint(u.Uid, 10, 32)
	if uidErr != nil {
		return nil, "", fmt.Errorf("invalid uid for user %q: %w", runAsUser, uidErr)
	}

	gid := uid
	if trimmedGroup := strings.TrimSpace(runAsGroup); trimmedGroup != "" {
		g, groupErr := user.LookupGroup(trimmedGroup)
		if groupErr != nil {
			return nil, "", fmt.Errorf("lookup run_as_group %q failed: %w", trimmedGroup, groupErr)
		}
		parsedGid, gidErr := strconv.ParseUint(g.Gid, 10, 32)
		if gidErr != nil {
			return nil, "", fmt.Errorf("invalid gid for group %q: %w", trimmedGroup, gidErr)
		}
		gid = parsedGid
	} else {
		parsedGid, gidErr := strconv.ParseUint(u.Gid, 10, 32)
		if gidErr != nil {
			return nil, "", fmt.Errorf("invalid primary gid for user %q: %w", runAsUser, gidErr)
		}
		gid = parsedGid
	}

	supplementaryGids := []uint32{uint32(gid)}
	if groupIDs, groupIDsErr := u.GroupIds(); groupIDsErr == nil {
		for _, gidText := range groupIDs {
			parsed, parseErr := strconv.ParseUint(gidText, 10, 32)
			if parseErr != nil {
				continue
			}
			supplementaryGids = append(supplementaryGids, uint32(parsed))
		}
	}

	return &syscall.Credential{
		Uid:    uint32(uid),
		Gid:    uint32(gid),
		Groups: dedupeUint32(supplementaryGids),
	}, u.HomeDir, nil
}

func dedupeUint32(values []uint32) []uint32 {
	seen := make(map[uint32]struct{}, len(values))
	result := make([]uint32, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

// runAsEnv 覆盖 HOME/USER/LOGNAME 环境变量为目标降权用户的身份，避免子进程继续沿用 root 的
// $HOME（很多工具/ansible 模块会依赖 $HOME 写临时文件、缓存），并将文件所有权 chown 给目标用户，
// 使降权后的进程能够读取 dj-agent（root 身份）预先写好的临时脚本/Playbook 文件。
func runAsEnv(baseEnv []string, runAsUser, homeDir string) []string {
	env := append([]string{}, baseEnv...)
	if homeDir != "" {
		env = append(env, "HOME="+homeDir)
	}
	if runAsUser != "" {
		env = append(env, "USER="+runAsUser, "LOGNAME="+runAsUser)
	}
	return env
}

// buildShellScriptCommand 构建 Shell 脚本执行命令
// 返回 (cmd, tempResourceList, error)；error 非 nil 时 cmd 必为 nil，调用方据此判定失败并将
// error 原样回传到作业结果（result.Error），便于无法登录目标主机查日志时也能定位具体原因。
func buildShellScriptCommand(ctx context.Context, jobID, templateContent, shellParameters string, envVars map[string]string, jobWorkDir, runAsUser, homeDir string, credential *syscall.Credential) (*exec.Cmd, []string, error) {
	tempFile, createErr := os.CreateTemp("", "dj-agent-automation-*.sh")
	if createErr != nil {
		err := fmt.Errorf("create temp file: %w", createErr)
		slog.Error("build shell script command failed", "job_id", jobID, "error", err)
		return nil, nil, err
	}
	tempPath := tempFile.Name()

	if _, err := tempFile.WriteString(templateContent + "\n"); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		wrapped := fmt.Errorf("write temp file: %w", err)
		slog.Error("build shell script command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		wrapped := fmt.Errorf("close temp file: %w", err)
		slog.Error("build shell script command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}
	if err := os.Chmod(tempPath, 0o700); err != nil {
		_ = os.Remove(tempPath)
		wrapped := fmt.Errorf("chmod temp file: %w", err)
		slog.Error("build shell script command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}
	// 脚本文件由 dj-agent（root）创建，需 chown 给目标降权用户才能被非 root 子进程读取/执行。
	// 若 dj-agent 自身未以 root 运行，或 systemd unit 剥夺了 CAP_CHOWN 能力（NoNewPrivileges/
	// CapabilityBoundingSet 等加固参数），此处会因权限不足失败，故把 uid/gid 和底层错误一并
	// 包进返回的 error，直接回传到作业结果，而不仅仅记录到本地日志。
	if err := os.Chown(tempPath, int(credential.Uid), int(credential.Gid)); err != nil {
		_ = os.Remove(tempPath)
		wrapped := fmt.Errorf("chown temp file to uid=%d gid=%d (dj-agent 是否以 root 运行/systemd 是否限制了 CAP_CHOWN？): %w", credential.Uid, credential.Gid, err)
		slog.Error("build shell script command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}

	scriptArgs := []string{tempPath}
	if shellParameters != "" {
		scriptArgs = append(scriptArgs, strings.Fields(shellParameters)...)
	}

	cmd := exec.CommandContext(ctx, "/bin/bash", scriptArgs...)
	cmd.Env = runAsEnv(append(os.Environ(), mapToEnv(envVars)...), runAsUser, homeDir)
	cmd.Dir = resolveAutomationWorkDir(jobWorkDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: credential}

	return cmd, []string{tempPath}, nil
}

// buildPlaybookCommand 构建 Ansible Playbook 执行命令
// 返回 (cmd, tempResourceList, error)；error 非 nil 时 cmd 必为 nil，调用方据此判定失败并将
// error 原样回传到作业结果（result.Error），便于无法登录目标主机查日志时也能定位具体原因。
func buildPlaybookCommand(ctx context.Context, jobID, templateContent string, envVars map[string]string, extraVars map[string]any, jobWorkDir, runAsUser, homeDir string, credential *syscall.Credential) (*exec.Cmd, []string, error) {
	workDir, mkErr := os.MkdirTemp("", "dj-agent-playbook-*")
	if mkErr != nil {
		err := fmt.Errorf("mkdir temp dir: %w", mkErr)
		slog.Error("build playbook command failed", "job_id", jobID, "error", err)
		return nil, nil, err
	}
	// 目录本身由 dj-agent（root）创建，需 chown 给目标降权用户，否则非 root 子进程无法遍历该目录。
	// 若 dj-agent 自身未以 root 运行，或 systemd unit 剥夺了 CAP_CHOWN 能力（NoNewPrivileges/
	// CapabilityBoundingSet 等加固参数），此处会因权限不足失败，故把 uid/gid 和底层错误一并
	// 包进返回的 error，直接回传到作业结果，而不仅仅记录到本地日志。
	if err := os.Chown(workDir, int(credential.Uid), int(credential.Gid)); err != nil {
		_ = os.RemoveAll(workDir)
		wrapped := fmt.Errorf("chown work dir to uid=%d gid=%d (dj-agent 是否以 root 运行/systemd 是否限制了 CAP_CHOWN？): %w", credential.Uid, credential.Gid, err)
		slog.Error("build playbook command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}

	playbookPath := workDir + "/playbook.yml"
	inventoryPath := workDir + "/inventory.ini"

	if writeErr := os.WriteFile(playbookPath, []byte(templateContent+"\n"), 0o600); writeErr != nil {
		_ = os.RemoveAll(workDir)
		wrapped := fmt.Errorf("write playbook file: %w", writeErr)
		slog.Error("build playbook command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}
	if err := os.Chown(playbookPath, int(credential.Uid), int(credential.Gid)); err != nil {
		_ = os.RemoveAll(workDir)
		wrapped := fmt.Errorf("chown playbook file to uid=%d gid=%d: %w", credential.Uid, credential.Gid, err)
		slog.Error("build playbook command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}

	inventoryContent := "[all]\nlocalhost ansible_connection=local ansible_python_interpreter=auto_silent\n"
	if writeErr := os.WriteFile(inventoryPath, []byte(inventoryContent), 0o600); writeErr != nil {
		_ = os.RemoveAll(workDir)
		wrapped := fmt.Errorf("write inventory file: %w", writeErr)
		slog.Error("build playbook command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}
	if err := os.Chown(inventoryPath, int(credential.Uid), int(credential.Gid)); err != nil {
		_ = os.RemoveAll(workDir)
		wrapped := fmt.Errorf("chown inventory file to uid=%d gid=%d: %w", credential.Uid, credential.Gid, err)
		slog.Error("build playbook command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}

	playbookOptions := &playbook.AnsiblePlaybookOptions{
		Inventory: inventoryPath,
		Limit:     "localhost",
	}
	if len(extraVars) > 0 {
		playbookOptions.ExtraVars = extraVars
	}

	connectionOptions := &options.AnsibleConnectionOptions{
		Connection: "local",
	}

	// 不再使用 ansible become/sudo：整个 ansible-playbook 进程通过 SysProcAttr.Credential 以
	// run_as_user/run_as_group 身份运行，playbook 内的任务（ansible_connection=local）自然以该
	// 用户身份执行，效果等价于 become 但不依赖目标机器上的 sudoers 配置。
	playbookCmd := &playbook.AnsiblePlaybookCmd{
		Playbooks:         []string{playbookPath},
		Options:           playbookOptions,
		ConnectionOptions: connectionOptions,
	}
	cmdArgs, cmdErr := playbookCmd.Command()
	if cmdErr != nil || len(cmdArgs) == 0 {
		_ = os.RemoveAll(workDir)
		wrapped := fmt.Errorf("generate ansible-playbook args: %w", cmdErr)
		slog.Error("build playbook command failed", "job_id", jobID, "error", wrapped)
		return nil, nil, wrapped
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = runAsEnv(append(os.Environ(), mapToEnv(envVars)...), runAsUser, homeDir)
	cmd.Dir = resolveAutomationWorkDir(jobWorkDir)
	cmd.SysProcAttr = &syscall.SysProcAttr{Credential: credential}

	return cmd, []string{workDir}, nil
}

// resolveAutomationWorkDir 解析自动化任务工作目录
func resolveAutomationWorkDir(jobWorkDir string) string {
	trimmedJobWorkDir := strings.TrimSpace(jobWorkDir)
	if trimmedJobWorkDir != "" {
		return trimmedJobWorkDir
	}

	// Default to /tmp for automation tasks when work_dir is not specified.
	if isDirectory(fallbackAutomationWorkDir) {
		return fallbackAutomationWorkDir
	}

	return ""
}

// isDirectory 检查路径是否为目录
func isDirectory(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
