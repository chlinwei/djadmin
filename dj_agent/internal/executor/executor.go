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
	"strconv"
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
const defaultMetricsSampleWindow = 200 * time.Millisecond
const minMetricsSampleWindow = 100 * time.Millisecond
const maxMetricsSampleWindow = 2 * time.Second

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

	// 收集操作系统真实运行时长（非 agent 进程时长）。
	osUptimeMetrics, osUptimeErr := collectOSUptimeMetrics()
	if osUptimeErr != nil {
		result.Data["os_uptime_error"] = osUptimeErr.Error()
	} else {
		for key, value := range osUptimeMetrics {
			result.Data[key] = value
		}
	}

	sampleWindow := resolveMetricsSampleWindow()
	result.Data["metrics_sample_window_ms"] = sampleWindow.Milliseconds()

	cpuMetrics, cpuErr := collectCPUMetrics(ctx, sampleWindow)
	if cpuErr != nil {
		result.Data["cpu_error"] = cpuErr.Error()
	} else {
		for key, value := range cpuMetrics {
			result.Data[key] = value
		}
	}

	memoryMetrics, memoryErr := collectMemoryMetrics()
	if memoryErr != nil {
		result.Data["memory_error"] = memoryErr.Error()
	} else {
		for key, value := range memoryMetrics {
			result.Data[key] = value
		}
	}

	diskMetrics, diskErr := collectDiskIOMetrics(ctx, sampleWindow)
	if diskErr != nil {
		result.Data["disk_io_error"] = diskErr.Error()
	} else {
		for key, value := range diskMetrics {
			result.Data[key] = value
		}
	}
	return result
}

type cpuStatSnapshot struct {
	user   uint64
	system uint64
	nice   uint64
	idle   uint64
	iowait uint64
	hirq   uint64
	sirq   uint64
	steal  uint64
}

func (s cpuStatSnapshot) total() uint64 {
	return s.user + s.system + s.nice + s.idle + s.iowait + s.hirq + s.sirq + s.steal
}

func collectCPUMetrics(ctx context.Context, sampleWindow time.Duration) (map[string]any, error) {
	first, err := readCPUStatSnapshot()
	if err != nil {
		return nil, err
	}

	timer := time.NewTimer(sampleWindow)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}

	second, err := readCPUStatSnapshot()
	if err != nil {
		return nil, err
	}

	deltaTotal := second.total() - first.total()
	if deltaTotal == 0 {
		return nil, fmt.Errorf("invalid cpu sample interval")
	}

	toPct := func(delta uint64) float64 {
		return float64(delta) * 100 / float64(deltaTotal)
	}

	deltaUser := second.user - first.user
	deltaSystem := second.system - first.system
	deltaNice := second.nice - first.nice
	deltaIdle := second.idle - first.idle
	deltaIowait := second.iowait - first.iowait
	deltaHirq := second.hirq - first.hirq
	deltaSirq := second.sirq - first.sirq
	deltaSteal := second.steal - first.steal

	idlePct := toPct(deltaIdle)
	usagePct := 100 - idlePct
	if usagePct < 0 {
		usagePct = 0
	}

	return map[string]any{
		"cpu_usage_percent": usagePct,
		"cpu_times": map[string]any{
			"us": toPct(deltaUser),
			"sy": toPct(deltaSystem),
			"ni": toPct(deltaNice),
			"id": idlePct,
			"wa": toPct(deltaIowait),
			"hi": toPct(deltaHirq),
			"si": toPct(deltaSirq),
			"st": toPct(deltaSteal),
		},
	}, nil
}

func readCPUStatSnapshot() (cpuStatSnapshot, error) {
	content, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuStatSnapshot{}, fmt.Errorf("read /proc/stat failed: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "cpu ") {
		return cpuStatSnapshot{}, fmt.Errorf("invalid /proc/stat format")
	}

	parts := strings.Fields(lines[0])
	if len(parts) < 9 {
		return cpuStatSnapshot{}, fmt.Errorf("incomplete cpu stat fields")
	}

	parseUint := func(raw string) (uint64, error) {
		v, convErr := strconv.ParseUint(raw, 10, 64)
		if convErr != nil {
			return 0, convErr
		}
		return v, nil
	}

	user, err := parseUint(parts[1])
	if err != nil {
		return cpuStatSnapshot{}, err
	}
	system, err := parseUint(parts[3])
	if err != nil {
		return cpuStatSnapshot{}, err
	}
	nice, err := parseUint(parts[2])
	if err != nil {
		return cpuStatSnapshot{}, err
	}
	idle, err := parseUint(parts[4])
	if err != nil {
		return cpuStatSnapshot{}, err
	}
	iowait, err := parseUint(parts[5])
	if err != nil {
		return cpuStatSnapshot{}, err
	}
	hirq, err := parseUint(parts[6])
	if err != nil {
		return cpuStatSnapshot{}, err
	}
	sirq, err := parseUint(parts[7])
	if err != nil {
		return cpuStatSnapshot{}, err
	}
	steal, err := parseUint(parts[8])
	if err != nil {
		return cpuStatSnapshot{}, err
	}

	return cpuStatSnapshot{
		user:   user,
		system: system,
		nice:   nice,
		idle:   idle,
		iowait: iowait,
		hirq:   hirq,
		sirq:   sirq,
		steal:  steal,
	}, nil
}

func collectMemoryMetrics() (map[string]any, error) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return nil, fmt.Errorf("read /proc/meminfo failed: %w", err)
	}

	values := map[string]uint64{}
	for _, line := range strings.Split(string(content), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		key := strings.TrimSuffix(parts[0], ":")
		v, convErr := strconv.ParseUint(parts[1], 10, 64)
		if convErr != nil {
			continue
		}
		// /proc/meminfo 默认单位是 kB
		values[key] = v
	}

	totalKB, hasTotal := values["MemTotal"]
	availableKB, hasAvailable := values["MemAvailable"]
	freeKB, hasFree := values["MemFree"]
	if !hasTotal || totalKB == 0 {
		return nil, fmt.Errorf("MemTotal not found in /proc/meminfo")
	}

	if !hasAvailable {
		availableKB = freeKB
	}
	if !hasFree {
		freeKB = availableKB
	}

	usedKB := uint64(0)
	if totalKB > availableKB {
		usedKB = totalKB - availableKB
	}

	usagePct := float64(usedKB) * 100 / float64(totalKB)

	return map[string]any{
		"memory_usage_percent": usagePct,
		"memory": map[string]any{
			"total":     formatKBToHuman(totalKB),
			"used":      formatKBToHuman(usedKB),
			"free":      formatKBToHuman(freeKB),
			"available": formatKBToHuman(availableKB),
		},
	}, nil
}

func formatKBToHuman(kb uint64) string {
	const kbPerGiB = 1024.0 * 1024.0
	giB := float64(kb) / kbPerGiB
	if giB >= 10 {
		return fmt.Sprintf("%.0fGi", giB)
	}
	return fmt.Sprintf("%.1fGi", giB)
}

func collectOSUptimeMetrics() (map[string]any, error) {
	content, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return nil, fmt.Errorf("read /proc/uptime failed: %w", err)
	}

	fields := strings.Fields(strings.TrimSpace(string(content)))
	if len(fields) < 1 {
		return nil, fmt.Errorf("invalid /proc/uptime format")
	}

	uptimeSecondsFloat, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return nil, fmt.Errorf("parse /proc/uptime failed: %w", err)
	}
	if uptimeSecondsFloat < 0 {
		uptimeSecondsFloat = 0
	}

	uptimeSeconds := int64(uptimeSecondsFloat)
	bootAt := time.Now().Add(-time.Duration(uptimeSecondsFloat * float64(time.Second))).UTC()

	return map[string]any{
		"os_uptime_seconds": uptimeSeconds,
		"os_boot_time":      bootAt.Format(time.RFC3339),
	}, nil
}

type diskStatSnapshot struct {
	readSectors  uint64
	writeSectors uint64
}

func collectDiskIOMetrics(ctx context.Context, sampleWindow time.Duration) (map[string]any, error) {
	first, err := readDiskStatSnapshots()
	if err != nil {
		return nil, err
	}

	timer := time.NewTimer(sampleWindow)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
	}

	second, err := readDiskStatSnapshots()
	if err != nil {
		return nil, err
	}

	seconds := sampleWindow.Seconds()
	if seconds <= 0 {
		seconds = 0.2
	}

	diskIO := map[string]any{}
	for device, after := range second {
		before, ok := first[device]
		if !ok {
			continue
		}
		if after.readSectors < before.readSectors || after.writeSectors < before.writeSectors {
			continue
		}

		readBytes := float64(after.readSectors-before.readSectors) * 512
		writeBytes := float64(after.writeSectors-before.writeSectors) * 512
		readBps := readBytes / seconds
		writeBps := writeBytes / seconds

		speedData := map[string]any{
			"read_bps":    readBps,
			"write_bps":   writeBps,
			"read_speed":  formatBytesPerSecond(readBps),
			"write_speed": formatBytesPerSecond(writeBps),
		}

		diskIO[device] = speedData
		for _, alias := range resolveDiskStatAliases(device) {
			diskIO[alias] = speedData
		}
	}

	return map[string]any{"disk_io": diskIO}, nil
}

func readDiskStatSnapshots() (map[string]diskStatSnapshot, error) {
	content, err := os.ReadFile("/proc/diskstats")
	if err != nil {
		return nil, fmt.Errorf("read /proc/diskstats failed: %w", err)
	}

	result := map[string]diskStatSnapshot{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 10 {
			continue
		}

		device := strings.TrimSpace(parts[2])
		if shouldSkipDiskDevice(device) {
			continue
		}

		readSectors, err := strconv.ParseUint(parts[5], 10, 64)
		if err != nil {
			continue
		}
		writeSectors, err := strconv.ParseUint(parts[9], 10, 64)
		if err != nil {
			continue
		}

		result[device] = diskStatSnapshot{
			readSectors:  readSectors,
			writeSectors: writeSectors,
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no eligible disk stats found")
	}
	return result, nil
}

func shouldSkipDiskDevice(device string) bool {
	// Skip virtual/optical devices that are not useful for host data disks.
	prefixes := []string{"loop", "ram", "fd", "sr"}
	for _, prefix := range prefixes {
		if strings.HasPrefix(device, prefix) {
			return true
		}
	}
	return false
}

func resolveDiskStatAliases(device string) []string {
	if !strings.HasPrefix(device, "dm-") {
		return nil
	}

	namePath := fmt.Sprintf("/sys/block/%s/dm/name", device)
	body, err := os.ReadFile(namePath)
	if err != nil {
		return nil
	}

	mapperName := strings.TrimSpace(string(body))
	if mapperName == "" {
		return nil
	}

	return []string{mapperName, "/dev/mapper/" + mapperName}
}

func formatBytesPerSecond(bytesPerSecond float64) string {
	const (
		kiB = 1024.0
		miB = 1024.0 * 1024.0
		giB = 1024.0 * 1024.0 * 1024.0
	)

	switch {
	case bytesPerSecond >= giB:
		return fmt.Sprintf("%.2f GiB/s", bytesPerSecond/giB)
	case bytesPerSecond >= miB:
		return fmt.Sprintf("%.2f MiB/s", bytesPerSecond/miB)
	case bytesPerSecond >= kiB:
		return fmt.Sprintf("%.2f KiB/s", bytesPerSecond/kiB)
	default:
		return fmt.Sprintf("%.0f B/s", bytesPerSecond)
	}
}

func resolveMetricsSampleWindow() time.Duration {
	raw := strings.TrimSpace(os.Getenv("DJ_AGENT_METRICS_SAMPLE_MS"))
	if raw == "" {
		return defaultMetricsSampleWindow
	}

	ms, err := strconv.Atoi(raw)
	if err != nil || ms <= 0 {
		return defaultMetricsSampleWindow
	}

	window := time.Duration(ms) * time.Millisecond
	if window < minMetricsSampleWindow {
		return minMetricsSampleWindow
	}
	if window > maxMetricsSampleWindow {
		return maxMetricsSampleWindow
	}
	return window
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
