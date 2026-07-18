package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

const defaultTimeout = 30 * time.Second
const actionGetAgentVersion = "get_agent_version"
const actionGetHostInfo = "get_host_info"
const actionRunAutomationTask = "run_automation_task"
const defaultAgentVersion = "v1"
const fallbackAutomationWorkDir = "/tmp"

// Executor 负责执行单个任务并采集结果。
type Executor struct {
	DefaultTimeout time.Duration
}

func New(defaultJobTimeout time.Duration) *Executor {
	return &Executor{DefaultTimeout: defaultJobTimeout}
}

func (e *Executor) Run(ctx context.Context, job protocol.Job) (protocol.JobResult, error) {
	if job.JobID == "" {
		return protocol.JobResult{}, fmt.Errorf("job_id is required")
	}
	if job.Type == "" {
		return protocol.JobResult{}, fmt.Errorf("type is required")
	}
	if job.Action == "" {
		return protocol.JobResult{}, fmt.Errorf("action is required")
	}

	if err := validateJobByType(job); err != nil {
		return protocol.JobResult{}, err
	}

	timeout := e.resolveTimeout(job.Timeout)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if result, handled := e.runBuiltinAction(runCtx, job); handled {
		return result, nil
	}

	cmd := exec.CommandContext(runCtx, job.Command, job.Args...)
	if job.WorkDir != "" {
		cmd.Dir = job.WorkDir
	}
	if len(job.Env) > 0 {
		cmd.Env = append(os.Environ(), mapToEnv(job.Env)...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err := cmd.Run()
	finished := time.Now()

	result := protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		StartedAt:  started,
		FinishedAt: finished,
		CostMS:     finished.Sub(started).Milliseconds(),
		ExitCode:   readExitCode(err),
	}

	switch {
	case err == nil:
		result.Status = protocol.StatusSuccess
		return result, nil
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		result.Status = protocol.StatusTimeout
		result.Error = runCtx.Err().Error()
		return result, err
	case errors.Is(runCtx.Err(), context.Canceled):
		result.Status = protocol.StatusCanceled
		result.Error = runCtx.Err().Error()
		return result, err
	default:
		result.Status = protocol.StatusFailed
		result.Error = err.Error()
		return result, err
	}
}

func validateJobByType(job protocol.Job) error {
	switch job.Type {
	case protocol.TaskTypeCommand, protocol.TaskTypeAutomation:
		if job.Command == "" {
			return fmt.Errorf("command is required for type=%s", job.Type)
		}
	case protocol.TaskTypeInventory, protocol.TaskTypeCustom:
		if job.Action != actionGetAgentVersion && job.Action != actionGetHostInfo && len(job.Params) == 0 {
			return fmt.Errorf("params is required for type=%s", job.Type)
		}
	default:
		return fmt.Errorf("unsupported type: %s", job.Type)
	}

	return nil
}

func (e *Executor) runBuiltinAction(ctx context.Context, job protocol.Job) (protocol.JobResult, bool) {
	switch strings.TrimSpace(job.Action) {
	case actionGetAgentVersion:
		return e.getAgentVersion(ctx, job), true
	case actionGetHostInfo:
		return e.getHostInfo(ctx, job), true
	case actionRunAutomationTask:
		return e.runAutomationTask(ctx, job), true
	default:
		return protocol.JobResult{}, false
	}
}

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

	if templateType == "" {
		templateType = "shell_script"
	}
	if templateType != "shell_script" && templateType != "playbook" {
		finished := time.Now()
		result.Status = protocol.StatusFailed
		result.ExitCode = 1
		result.Error = fmt.Sprintf("unsupported template_type for agent mode: %s", templateType)
		result.FinishedAt = finished
		result.CostMS = finished.Sub(started).Milliseconds()
		return result
	}
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
	var err error

	if templateType == "shell_script" {
		tempFile, createErr := os.CreateTemp("", "dj-agent-automation-*.sh")
		if createErr != nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = fmt.Sprintf("create temp script failed: %v", createErr)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
		tempPath := tempFile.Name()
		defer os.Remove(tempPath)

		if _, err = tempFile.WriteString(templateContent + "\n"); err != nil {
			_ = tempFile.Close()
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = fmt.Sprintf("write temp script failed: %v", err)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
		if err = tempFile.Close(); err != nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = fmt.Sprintf("close temp script failed: %v", err)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
		if err = os.Chmod(tempPath, 0o700); err != nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = fmt.Sprintf("chmod temp script failed: %v", err)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}

		scriptArgs := []string{tempPath}
		if shellParameters != "" {
			scriptArgs = append(scriptArgs, strings.Fields(shellParameters)...)
		}
		cmd = exec.CommandContext(ctx, "/bin/bash", scriptArgs...)
		cmd.Env = append(os.Environ(), mapToEnv(envVars)...)
		cmd.Dir = resolveAutomationWorkDir(job.WorkDir)
	} else {
		workDir, mkErr := os.MkdirTemp("", "dj-agent-playbook-*")
		if mkErr != nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = fmt.Sprintf("create temp playbook dir failed: %v", mkErr)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
		defer os.RemoveAll(workDir)

		playbookPath := workDir + "/playbook.yml"
		inventoryPath := workDir + "/inventory.ini"

		if writeErr := os.WriteFile(playbookPath, []byte(templateContent+"\n"), 0o600); writeErr != nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = fmt.Sprintf("write playbook file failed: %v", writeErr)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}
		if writeErr := os.WriteFile(inventoryPath, []byte("[all]\nlocalhost ansible_connection=local\n"), 0o600); writeErr != nil {
			finished := time.Now()
			result.Status = protocol.StatusFailed
			result.ExitCode = 1
			result.Error = fmt.Sprintf("write inventory file failed: %v", writeErr)
			result.FinishedAt = finished
			result.CostMS = finished.Sub(started).Milliseconds()
			return result
		}

		args := []string{"-i", inventoryPath, playbookPath, "-l", "localhost"}
		if len(extraVars) > 0 {
			extraVarsPath := workDir + "/extra_vars.json"
			body, marshalErr := json.Marshal(extraVars)
			if marshalErr != nil {
				finished := time.Now()
				result.Status = protocol.StatusFailed
				result.ExitCode = 1
				result.Error = fmt.Sprintf("marshal extra_vars failed: %v", marshalErr)
				result.FinishedAt = finished
				result.CostMS = finished.Sub(started).Milliseconds()
				return result
			}
			if writeErr := os.WriteFile(extraVarsPath, body, 0o600); writeErr != nil {
				finished := time.Now()
				result.Status = protocol.StatusFailed
				result.ExitCode = 1
				result.Error = fmt.Sprintf("write extra_vars file failed: %v", writeErr)
				result.FinishedAt = finished
				result.CostMS = finished.Sub(started).Milliseconds()
				return result
			}
			args = append(args, "-e", "@"+extraVarsPath)
		}
		if becomeEnabled {
			args = append(args, "--become")
			if becomeMethod != "" {
				args = append(args, "--become-method", becomeMethod)
			}
			if becomeUser != "" {
				args = append(args, "--become-user", becomeUser)
			}
		}

		cmd = exec.CommandContext(ctx, "ansible-playbook", args...)
		cmd.Env = append(os.Environ(), mapToEnv(envVars)...)
		cmd.Dir = resolveAutomationWorkDir(job.WorkDir)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
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

func toString(value any) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%v", value)
}

func toStringMap(value any) map[string]string {
	result := map[string]string{}
	if value == nil {
		return result
	}
	if typed, ok := value.(map[string]string); ok {
		for key, item := range typed {
			result[key] = item
		}
		return result
	}
	if typed, ok := value.(map[string]any); ok {
		for key, item := range typed {
			result[key] = toString(item)
		}
	}
	return result
}

func toAnyMap(value any) map[string]any {
	result := map[string]any{}
	if value == nil {
		return result
	}
	if typed, ok := value.(map[string]any); ok {
		for key, item := range typed {
			result[key] = item
		}
		return result
	}
	if typed, ok := value.(map[string]string); ok {
		for key, item := range typed {
			result[key] = item
		}
	}
	return result
}

func toBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		text := strings.TrimSpace(strings.ToLower(typed))
		return text == "1" || text == "true" || text == "yes" || text == "on"
	case int:
		return typed != 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

// getAgentVersion 返回当前 agent 的版本与运行时信息。
func (e *Executor) getAgentVersion(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	select {
	case <-ctx.Done():
		finished := time.Now()
		return protocol.JobResult{
			JobID:      job.JobID,
			Type:       job.Type,
			Action:     job.Action,
			Status:     protocol.StatusCanceled,
			StartedAt:  started,
			FinishedAt: finished,
			CostMS:     finished.Sub(started).Milliseconds(),
			Error:      ctx.Err().Error(),
		}
	default:
	}

	version := strings.TrimSpace(os.Getenv("DJ_AGENT_VERSION"))
	if version == "" {
		version = defaultAgentVersion
	}
	versionTag := fmt.Sprintf("dj_agent:%s", version)

	finished := time.Now()
	return protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Status:     protocol.StatusSuccess,
		ExitCode:   0,
		StartedAt:  started,
		FinishedAt: finished,
		CostMS:     finished.Sub(started).Milliseconds(),
		Data: map[string]any{
			"agent_version":     versionTag,
			"agent_version_raw": version,
			"go_version":        runtime.Version(),
			"os":                runtime.GOOS,
			"arch":              runtime.GOARCH,
		},
	}
}

// getHostInfo 返回当前主机基础信息。
func (e *Executor) getHostInfo(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	select {
	case <-ctx.Done():
		finished := time.Now()
		return protocol.JobResult{
			JobID:      job.JobID,
			Type:       job.Type,
			Action:     job.Action,
			Status:     protocol.StatusCanceled,
			StartedAt:  started,
			FinishedAt: finished,
			CostMS:     finished.Sub(started).Milliseconds(),
			Error:      ctx.Err().Error(),
		}
	default:
	}

	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}

	ips := make([]string, 0)
	addrs, addrErr := net.InterfaceAddrs()
	if addrErr == nil {
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				ips = append(ips, ip4.String())
			}
		}
		sort.Strings(ips)
	}

	version := strings.TrimSpace(os.Getenv("DJ_AGENT_VERSION"))
	if version == "" {
		version = defaultAgentVersion
	}

	finished := time.Now()
	result := protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Status:     protocol.StatusSuccess,
		ExitCode:   0,
		StartedAt:  started,
		FinishedAt: finished,
		CostMS:     finished.Sub(started).Milliseconds(),
		Data: map[string]any{
			"agent_version": fmt.Sprintf("dj_agent:%s", version),
			"hostname":      hostname,
			"os":            runtime.GOOS,
			"arch":          runtime.GOARCH,
			"go_version":    runtime.Version(),
			"cpu_count":     runtime.NumCPU(),
			"local_ipv4":    ips,
			"pid":           os.Getpid(),
		},
	}
	if addrErr != nil {
		result.Data["network_error"] = addrErr.Error()
	}
	return result
}

func (e *Executor) resolveTimeout(jobTimeout time.Duration) time.Duration {
	if jobTimeout > 0 {
		return jobTimeout
	}
	if e.DefaultTimeout > 0 {
		return e.DefaultTimeout
	}
	return defaultTimeout
}

func mapToEnv(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	env := make([]string, 0, len(keys))
	for _, k := range keys {
		env = append(env, fmt.Sprintf("%s=%s", k, values[k]))
	}
	return env
}

func readExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}
