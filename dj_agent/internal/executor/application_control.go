package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

func (e *Executor) controlApplication(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	action := valueString(job.Params["control_action"])
	command, commandErr := applicationControlCommand(ctx, job.Params, action)
	if commandErr != nil {
		return applicationControlResult(job, action, started, nil, commandErr)
	}
	output, runErr := command.CombinedOutput()
	return applicationControlResult(job, action, started, output, runErr)
}

func applicationControlResult(job protocol.Job, action string, started time.Time, output []byte, err error) protocol.JobResult {
	finished := time.Now()
	exitCode := readExitCode(err)
	result := protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action,
		Status: protocol.StatusSuccess, ExitCode: exitCode, Stdout: strings.TrimSpace(string(output)),
		StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
		Data: map[string]any{"control_action": action, "exit_code": exitCode},
	}
	// 状态命令的非零退出码通常表示 stopped/inactive；配置和命令构造错误仍需正常报错。
	var exitErr *exec.ExitError
	if action == "status" && (err == nil || errors.As(err, &exitErr)) {
		return result
	}
	if err != nil {
		result.Status = protocol.StatusFailed
		result.Error = err.Error()
		result.Stderr = result.Stdout
		result.Stdout = ""
	}
	return result
}

func applicationControlCommand(ctx context.Context, params map[string]any, action string) (*exec.Cmd, error) {
	if action != "start" && action != "stop" && action != "status" {
		return nil, fmt.Errorf("应用控制动作必须为 start、stop 或 status")
	}
	switch valueString(params["control_type"]) {
	case "systemd":
		return applicationSystemdActionCommand(ctx, params, action)
	case "command":
		return applicationCustomControlCommand(ctx, params, action)
	case "docker":
		return applicationDockerControlCommand(ctx, params, action)
	case "docker_compose":
		return applicationComposeControlCommand(ctx, params, action)
	case "external_ha":
		if action != "status" {
			return nil, fmt.Errorf("外部 HA 应用不支持从 dj-agent 执行启停")
		}
		return applicationCustomControlCommand(ctx, params, action)
	default:
		return nil, fmt.Errorf("不支持的应用控制方式")
	}
}

func applicationCustomControlCommand(ctx context.Context, params map[string]any, action string) (*exec.Cmd, error) {
	actions, ok := params["control_actions"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("缺少命令行控制动作配置")
	}
	actionConfig, ok := actions[action].(map[string]any)
	if !ok || strings.TrimSpace(valueString(actionConfig["command"])) == "" {
		return nil, fmt.Errorf("未配置 %s 控制命令", action)
	}
	command, err := applicationRunUserCommand(ctx, valueString(actionConfig["command"]), valueString(params["run_user"]))
	if err != nil {
		return nil, err
	}
	workDirectory := valueString(params["work_directory"])
	if workDirectory != "" {
		if !filepath.IsAbs(workDirectory) {
			return nil, fmt.Errorf("工作目录必须为绝对路径")
		}
		command.Dir = workDirectory
	}
	return command, nil
}

func applicationRunUserCommand(ctx context.Context, commandText, runUser string) (*exec.Cmd, error) {
	if runUser == "" {
		return nil, fmt.Errorf("命令行控制必须配置运行用户")
	}
	targetUser, err := user.Lookup(runUser)
	if err != nil {
		return nil, fmt.Errorf("无法查找应用运行用户 %q: %w", runUser, err)
	}
	targetUID, parseErr := strconv.ParseUint(targetUser.Uid, 10, 32)
	if parseErr != nil {
		return nil, fmt.Errorf("应用运行用户 %q 的 UID 无效", runUser)
	}
	// login shell 加载目标用户 profile，使 JAVA_HOME 等用户级环境生效。
	command := exec.CommandContext(ctx, "/bin/bash", "-lc", commandText)
	command.Env = systemdUserEnvironment(os.Environ(), targetUser, targetUID)
	if uint64(os.Geteuid()) == targetUID {
		return command, nil
	}
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("dj-agent 必须以 root 或目标用户 %q 运行", runUser)
	}
	targetGID, gidErr := strconv.ParseUint(targetUser.Gid, 10, 32)
	if gidErr != nil {
		return nil, fmt.Errorf("应用运行用户 %q 的 GID 无效", runUser)
	}
	// 直接 setuid/setgid 降权，不走 sudo：sudoers 常带 requiretty，Agent 无 tty 会被拒绝执行。
	command.SysProcAttr = &syscall.SysProcAttr{Credential: &syscall.Credential{
		Uid:    uint32(targetUID),
		Gid:    uint32(targetGID),
		Groups: supplementaryGroupIDs(targetUser),
	}}
	return command, nil
}

func supplementaryGroupIDs(targetUser *user.User) []uint32 {
	groupIDs, err := targetUser.GroupIds()
	if err != nil {
		return nil
	}
	groups := make([]uint32, 0, len(groupIDs))
	for _, rawGID := range groupIDs {
		gid, parseErr := strconv.ParseUint(rawGID, 10, 32)
		if parseErr != nil {
			continue
		}
		groups = append(groups, uint32(gid))
	}
	return groups
}

func applicationDockerControlCommand(ctx context.Context, params map[string]any, action string) (*exec.Cmd, error) {
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("Docker 控制命令必须由 root 执行")
	}
	config, ok := params["docker_config"].(map[string]any)
	containerName := ""
	if ok {
		containerName = valueString(config["container_name"])
	}
	if !safeResourceNamePattern.MatchString(containerName) {
		return nil, fmt.Errorf("Docker 容器名称格式无效")
	}
	if action == "status" {
		return exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", containerName), nil
	}
	return exec.CommandContext(ctx, "docker", action, containerName), nil
}

func applicationComposeControlCommand(ctx context.Context, params map[string]any, action string) (*exec.Cmd, error) {
	config, ok := params["compose_config"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("缺少 Docker Compose 配置")
	}
	projectName := valueString(config["project_name"])
	serviceName := valueString(config["service_name"])
	composeFile := valueString(config["compose_file_path"])
	workingDirectory := valueString(config["working_directory"])
	if !safeResourceNamePattern.MatchString(projectName) || !safeResourceNamePattern.MatchString(serviceName) || !filepath.IsAbs(composeFile) || !filepath.IsAbs(workingDirectory) {
		return nil, fmt.Errorf("Docker Compose 项目、服务或路径格式无效")
	}
	args := []string{"compose", "-f", composeFile, "--project-name", projectName}
	if action == "status" {
		args = append(args, "ps", "--status", "running", "--services", serviceName)
	} else {
		args = append(args, action, serviceName)
	}
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = workingDirectory
	return command, nil
}
