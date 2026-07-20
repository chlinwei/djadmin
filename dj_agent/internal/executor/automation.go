package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
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
	becomeEnabled := toBool(job.Params["become_enabled"])
	becomeMethod := strings.TrimSpace(toString(job.Params["become_method"]))
	becomeUser := strings.TrimSpace(toString(job.Params["become_user"]))

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

	var cmd *exec.Cmd
	var tempResources []string // 需要清理的临时文件/目录

	if templateType == "shell_script" {
		cmd, tempResources = buildShellScriptCommand(ctx, templateContent, shellParameters, envVars, job.WorkDir)
		if cmd == nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = "failed to build shell script command"
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
	} else {
		cmd, tempResources = buildPlaybookCommand(ctx, templateContent, envVars, extraVars, becomeEnabled, becomeMethod, becomeUser, job.WorkDir)
		if cmd == nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = "failed to build playbook command"
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

// buildShellScriptCommand 构建 Shell 脚本执行命令
// 返回 (cmd, tempResourceList, error)
func buildShellScriptCommand(ctx context.Context, templateContent, shellParameters string, envVars map[string]string, jobWorkDir string) (*exec.Cmd, []string) {
	tempFile, createErr := os.CreateTemp("", "dj-agent-automation-*.sh")
	if createErr != nil {
		return nil, nil
	}
	tempPath := tempFile.Name()

	if _, err := tempFile.WriteString(templateContent + "\n"); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempPath)
		return nil, nil
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempPath)
		return nil, nil
	}
	if err := os.Chmod(tempPath, 0o700); err != nil {
		_ = os.Remove(tempPath)
		return nil, nil
	}

	scriptArgs := []string{tempPath}
	if shellParameters != "" {
		scriptArgs = append(scriptArgs, strings.Fields(shellParameters)...)
	}

	cmd := exec.CommandContext(ctx, "/bin/bash", scriptArgs...)
	cmd.Env = append(os.Environ(), mapToEnv(envVars)...)
	cmd.Dir = resolveAutomationWorkDir(jobWorkDir)

	return cmd, []string{tempPath}
}

// buildPlaybookCommand 构建 Ansible Playbook 执行命令
func buildPlaybookCommand(ctx context.Context, templateContent string, envVars map[string]string, extraVars map[string]any, becomeEnabled bool, becomeMethod, becomeUser, jobWorkDir string) (*exec.Cmd, []string) {
	workDir, mkErr := os.MkdirTemp("", "dj-agent-playbook-*")
	if mkErr != nil {
		return nil, nil
	}

	playbookPath := workDir + "/playbook.yml"
	inventoryPath := workDir + "/inventory.ini"

	if writeErr := os.WriteFile(playbookPath, []byte(templateContent+"\n"), 0o600); writeErr != nil {
		_ = os.RemoveAll(workDir)
		return nil, nil
	}

	inventoryContent := "[all]\nlocalhost ansible_connection=local ansible_python_interpreter=auto_silent\n"
	if writeErr := os.WriteFile(inventoryPath, []byte(inventoryContent), 0o600); writeErr != nil {
		_ = os.RemoveAll(workDir)
		return nil, nil
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

	privilegeOptions := &options.AnsiblePrivilegeEscalationOptions{}
	if becomeEnabled {
		privilegeOptions.Become = true
		if becomeMethod != "" {
			privilegeOptions.BecomeMethod = becomeMethod
		}
		if becomeUser != "" {
			privilegeOptions.BecomeUser = becomeUser
		}
	}

	playbookCmd := &playbook.AnsiblePlaybookCmd{
		Playbooks:                  []string{playbookPath},
		Options:                    playbookOptions,
		ConnectionOptions:          connectionOptions,
		PrivilegeEscalationOptions: privilegeOptions,
	}
	cmdArgs, cmdErr := playbookCmd.Command()
	if cmdErr != nil || len(cmdArgs) == 0 {
		_ = os.RemoveAll(workDir)
		return nil, nil
	}

	cmd := exec.CommandContext(ctx, cmdArgs[0], cmdArgs[1:]...)
	cmd.Env = append(os.Environ(), mapToEnv(envVars)...)
	cmd.Dir = resolveAutomationWorkDir(jobWorkDir)

	return cmd, []string{workDir}
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
