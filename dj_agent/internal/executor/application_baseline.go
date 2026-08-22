package executor

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

var safeResourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]*$`)

type applicationCheckResult struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (e *Executor) checkApplicationBaseline(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	checks := make([]applicationCheckResult, 0)

	checks = append(checks, checkApplicationControl(ctx, job.Params))
	checks = append(checks, checkApplicationPorts(job.Params)...)
	checks = append(checks, checkApplicationPaths(job.Params)...)
	checks = append(checks, checkApplicationLogs(job.Params)...)

	passed := true
	for _, check := range checks {
		if check.Status != "pass" && check.Status != "skipped" {
			passed = false
			break
		}
	}

	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action,
		Status: protocol.StatusSuccess, ExitCode: 0,
		StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
		Data: map[string]any{"passed": passed, "checks": checks},
	}
}

func checkApplicationControl(ctx context.Context, params map[string]any) applicationCheckResult {
	controlType := valueString(params["control_type"])
	result := applicationCheckResult{Key: "control", Type: "control_status", Name: "运行状态", Expected: "running"}

	switch controlType {
	case "systemd":
		command, commandErr := applicationSystemdCommand(ctx, params)
		if commandErr != nil {
			result.Status, result.Message = "error", commandErr.Error()
			return result
		}
		output, err := command.CombinedOutput()
		actual := strings.TrimSpace(string(output))
		result.Actual = actual
		if err == nil && actual == "active" {
			result.Status = "pass"
		} else {
			result.Status, result.Message = "fail", "systemd 服务未处于 active 状态"
		}
	case "docker":
		config, ok := params["docker_config"].(map[string]any)
		containerName := ""
		if ok {
			containerName = valueString(config["container_name"])
		}
		if !safeResourceNamePattern.MatchString(containerName) {
			result.Status, result.Message = "error", "Docker 容器名称格式无效"
			return result
		}
		output, err := exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.State.Status}}", containerName).CombinedOutput()
		actual := strings.TrimSpace(string(output))
		result.Actual = actual
		if err == nil && actual == "running" {
			result.Status = "pass"
		} else {
			result.Status, result.Message = "fail", "Docker 容器未运行"
		}
	case "docker_compose":
		config, ok := params["compose_config"].(map[string]any)
		if !ok {
			result.Status, result.Message = "error", "缺少 Docker Compose 配置"
			return result
		}
		projectName := valueString(config["project_name"])
		serviceName := valueString(config["service_name"])
		composeFile := valueString(config["compose_file_path"])
		workingDirectory := valueString(config["working_directory"])
		if !safeResourceNamePattern.MatchString(projectName) || !safeResourceNamePattern.MatchString(serviceName) || !filepath.IsAbs(composeFile) || !filepath.IsAbs(workingDirectory) {
			result.Status, result.Message = "error", "Docker Compose 项目、服务或路径格式无效"
			return result
		}
		command := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "--project-name", projectName, "ps", "--status", "running", "--services", serviceName)
		command.Dir = workingDirectory
		output, err := command.CombinedOutput()
		actual := strings.TrimSpace(string(output))
		result.Actual = actual
		if err == nil && actual == serviceName {
			result.Status = "pass"
		} else {
			result.Status, result.Message = "fail", "Docker Compose 服务未运行"
		}
	case "external_ha":
		result.Status, result.Actual, result.Message = "skipped", "external", "外部 HA 由外部控制器管理"
	case "command":
		result.Status, result.Actual, result.Message = "skipped", "not_checked", "命令行状态检查暂不执行自定义命令"
	default:
		result.Status, result.Message = "error", "不支持的控制方式"
	}
	return result
}

func applicationSystemdCommand(ctx context.Context, params map[string]any) (*exec.Cmd, error) {
	return applicationSystemdActionCommand(ctx, params, "status")
}

func applicationSystemdActionCommand(ctx context.Context, params map[string]any, action string) (*exec.Cmd, error) {
	serviceName := valueString(params["service_name"])
	if !safeResourceNamePattern.MatchString(serviceName) {
		return nil, fmt.Errorf("systemd 服务名格式无效")
	}
	systemdAction := map[string]string{"start": "start", "stop": "stop", "status": "is-active"}[action]
	if systemdAction == "" {
		return nil, fmt.Errorf("不支持的应用控制动作")
	}

	scope := valueString(params["systemd_scope"])
	switch scope {
	case "system":
		return exec.CommandContext(ctx, "systemctl", systemdAction, serviceName), nil
	case "user":
		runUser := valueString(params["run_user"])
		if runUser == "" {
			return nil, fmt.Errorf("用户级 Systemd 必须配置运行用户")
		}
		targetUser, err := user.Lookup(runUser)
		if err != nil {
			return nil, fmt.Errorf("无法查找 Systemd 运行用户 %q: %w", runUser, err)
		}
		uid, uidErr := strconv.ParseUint(targetUser.Uid, 10, 32)
		gid, gidErr := strconv.ParseUint(targetUser.Gid, 10, 32)
		if uidErr != nil || gidErr != nil {
			return nil, fmt.Errorf("Systemd 运行用户 %q 的 UID/GID 无效", runUser)
		}
		if os.Geteuid() != 0 && uint64(os.Geteuid()) != uid {
			return nil, fmt.Errorf("dj-agent 必须以 root 或目标用户 %q 运行，才能执行用户级 Systemd", runUser)
		}

		command := exec.CommandContext(ctx, "systemctl", "--user", systemdAction, serviceName)
		command.Env = systemdUserEnvironment(os.Environ(), targetUser, uid)
		if uint64(os.Geteuid()) != uid {
			groupIDs, groupErr := targetUser.GroupIds()
			if groupErr != nil {
				return nil, fmt.Errorf("无法获取 Systemd 运行用户 %q 的附加组: %w", runUser, groupErr)
			}
			groups := make([]uint32, 0, len(groupIDs))
			for _, groupID := range groupIDs {
				parsedGroupID, parseErr := strconv.ParseUint(groupID, 10, 32)
				if parseErr != nil {
					return nil, fmt.Errorf("Systemd 运行用户 %q 的附加组 ID 无效", runUser)
				}
				groups = append(groups, uint32(parsedGroupID))
			}
			command.SysProcAttr = &syscall.SysProcAttr{
				Credential: &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid), Groups: groups},
			}
		}
		return command, nil
	default:
		return nil, fmt.Errorf("Systemd 作用域无效")
	}
}

func systemdUserEnvironment(environment []string, targetUser *user.User, uid uint64) []string {
	overrides := map[string]string{
		"HOME":            targetUser.HomeDir,
		"USER":            targetUser.Username,
		"LOGNAME":         targetUser.Username,
		"XDG_RUNTIME_DIR": fmt.Sprintf("/run/user/%d", uid),
	}
	result := make([]string, 0, len(environment)+len(overrides))
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, overridden := overrides[key]; overridden {
				continue
			}
		}
		result = append(result, entry)
	}
	for _, key := range []string{"HOME", "USER", "LOGNAME", "XDG_RUNTIME_DIR"} {
		result = append(result, key+"="+overrides[key])
	}
	return result
}

func checkApplicationPorts(params map[string]any) []applicationCheckResult {
	rawPorts, _ := params["ports"].([]any)
	results := make([]applicationCheckResult, 0, len(rawPorts))
	for _, raw := range rawPorts {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := valueString(item["name"])
		protocolName := strings.ToLower(valueString(item["protocol"]))
		port, err := numberToInt(item["port"])
		result := applicationCheckResult{Key: fmt.Sprintf("port:%s:%d", protocolName, port), Type: "port_listening", Name: name, Expected: true}
		if err != nil || port < 1 || port > 65535 || (protocolName != "tcp" && protocolName != "udp") {
			result.Status, result.Message = "error", "端口配置无效"
			results = append(results, result)
			continue
		}
		listening, checkErr := isLocalPortListening(protocolName, port)
		result.Actual = listening
		if checkErr != nil {
			result.Status, result.Message = "error", checkErr.Error()
		} else if listening {
			result.Status = "pass"
		} else {
			result.Status, result.Message = "fail", "端口未监听"
		}
		results = append(results, result)
	}
	return results
}

func checkApplicationPaths(params map[string]any) []applicationCheckResult {
	rawPaths, _ := params["paths"].([]any)
	results := make([]applicationCheckResult, 0, len(rawPaths))
	for _, raw := range rawPaths {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := valueString(item["name"])
		path := valueString(item["path"])
		result := applicationCheckResult{Key: "path:" + name, Type: "path", Name: name, Expected: true}
		if !filepath.IsAbs(path) {
			result.Status, result.Message = "error", "路径必须为绝对路径"
			results = append(results, result)
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			result.Actual = false
			result.Status, result.Message = "fail", err.Error()
			results = append(results, result)
			continue
		}
		result.Actual = true
		result.Status = "pass"
		expectedMode := valueString(item["expected_mode"])
		if expectedMode != "" && fmt.Sprintf("%04o", info.Mode().Perm()) != expectedMode {
			result.Status, result.Message = "fail", "文件权限不符合预期"
			result.Expected = expectedMode
			result.Actual = fmt.Sprintf("%04o", info.Mode().Perm())
		}
		if stat, ok := info.Sys().(*syscall.Stat_t); ok {
			expectedOwner := valueString(item["expected_owner"])
			if expectedOwner != "" {
				owner, lookupErr := user.LookupId(strconv.FormatUint(uint64(stat.Uid), 10))
				actualOwner := strconv.FormatUint(uint64(stat.Uid), 10)
				if lookupErr == nil {
					actualOwner = owner.Username
				}
				if actualOwner != expectedOwner {
					result.Status, result.Message, result.Expected, result.Actual = "fail", "文件属主不符合预期", expectedOwner, actualOwner
				}
			}
		}
		results = append(results, result)
	}
	return results
}

func checkApplicationLogs(params map[string]any) []applicationCheckResult {
	rawLogs, _ := params["logs"].([]any)
	results := make([]applicationCheckResult, 0, len(rawLogs))
	for _, raw := range rawLogs {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name := valueString(item["name"])
		pathPattern := valueString(item["path_pattern"])
		result := applicationCheckResult{Key: "log:" + name, Type: "log", Name: name, Expected: true}
		if !filepath.IsAbs(pathPattern) {
			result.Status, result.Message = "error", "日志路径模式必须为绝对路径"
			results = append(results, result)
			continue
		}
		matches, err := filepath.Glob(pathPattern)
		if err != nil {
			result.Status, result.Message = "error", "日志路径模式无效: "+err.Error()
			results = append(results, result)
			continue
		}
		matchedFileCount := 0
		for _, match := range matches {
			if info, statErr := os.Stat(match); statErr == nil && !info.IsDir() {
				matchedFileCount++
			}
		}
		result.Actual = matchedFileCount > 0
		if matchedFileCount == 0 {
			result.Status, result.Message = "fail", "未找到匹配的日志文件"
		} else {
			result.Status = "pass"
		}
		results = append(results, result)
	}
	return results
}

func isLocalPortListening(protocolName string, port int) (bool, error) {
	files := []string{"/proc/net/" + protocolName, "/proc/net/" + protocolName + "6"}
	target := strings.ToUpper(fmt.Sprintf("%04X", port))
	for _, path := range files {
		file, err := os.Open(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return false, err
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 4 {
				continue
			}
			parts := strings.Split(fields[1], ":")
			if len(parts) == 2 && strings.ToUpper(parts[1]) == target {
				state := strings.ToUpper(fields[3])
				if (protocolName == "tcp" && state == "0A") || protocolName == "udp" {
					file.Close()
					return true, nil
				}
			}
		}
		scanErr := scanner.Err()
		file.Close()
		if scanErr != nil {
			return false, scanErr
		}
	}
	return false, nil
}

func numberToInt(value any) (int, error) {
	switch typed := value.(type) {
	case int:
		return typed, nil
	case float64:
		return int(typed), nil
	case string:
		return strconv.Atoi(typed)
	default:
		return 0, fmt.Errorf("invalid number")
	}
}

func valueString(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
