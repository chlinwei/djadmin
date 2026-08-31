package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

const (
	actionGetAgentVersion              = "get_agent_version"
	actionGetHostInfo                  = "get_host_info"
	actionGetLocalAddresses            = "get_local_addresses"
	actionSyncAutomationSSHKey         = "sync_automation_ssh_key"
	actionStartExporter                = "start_exporter"
	actionStopExporter                 = "stop_exporter"
	actionCheckExporterStatus          = "check_exporter_status"
	actionCheckApplicationBaseline     = "check_application_baseline"
	actionControlApplication           = "control_application"
	actionReloadFluentBit              = "reload_fluent_bit"
	actionConfigureFluentBitOpenSearch = "configure_fluent_bit_opensearch"
	actionApplyAgentUpdate             = "apply_agent_update"
	defaultAgentVersion                = "v1"
)

const (
	// 与 backend agent_install_service.py 的目录/文件约定保持一致，两边不允许各自硬编码一份。
	agentUpdateStagingBinaryPath = "/var/lib/dj-agent/update/dj-agent.new"
	agentBinaryLivePath          = "/usr/local/bin/dj-agent"
	agentConfigLivePath          = "/etc/dj-agent/config.env"
	agentServiceUnitLivePath     = "/usr/lib/systemd/system/dj-agent.service"
)

// runBuiltinAction 分发并执行内置操作
func (e *Executor) runBuiltinAction(ctx context.Context, job protocol.Job) (protocol.JobResult, bool) {
	switch strings.TrimSpace(job.Action) {
	case actionGetAgentVersion:
		return e.getAgentVersion(ctx, job), true
	case actionGetHostInfo:
		return e.getHostInfo(ctx, job), true
	case actionGetLocalAddresses:
		return e.getLocalAddresses(ctx, job), true
	case actionSyncAutomationSSHKey:
		return e.syncAutomationSSHKey(ctx, job), true
	case actionStartExporter:
		return e.startExporter(ctx, job), true
	case actionStopExporter:
		return e.stopExporter(ctx, job), true
	case actionCheckExporterStatus:
		return e.checkExporterStatus(ctx, job), true
	case actionCheckApplicationBaseline:
		return e.checkApplicationBaseline(ctx, job), true
	case actionControlApplication:
		return e.controlApplication(ctx, job), true
	case actionReloadFluentBit:
		return e.reloadFluentBit(ctx, job), true
	case actionConfigureFluentBitOpenSearch:
		return e.configureFluentBitOpenSearch(ctx, job), true
	case actionApplyAgentUpdate:
		return e.applyAgentUpdate(ctx, job), true
	default:
		return protocol.JobResult{}, false
	}
}

func writeFileIfChanged(path string, content []byte, mode os.FileMode) (bool, error) {
	current, err := os.ReadFile(path)
	if err == nil && bytes.Equal(current, content) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".djadmin-")
	if err != nil {
		return false, err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(content); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return false, err
	}
	return true, nil
}

func (e *Executor) configureFluentBitOpenSearch(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	host := strings.TrimSpace(fmt.Sprint(job.Params["host"]))
	port := strings.TrimSpace(fmt.Sprint(job.Params["port"]))
	username := fmt.Sprint(job.Params["username"])
	password := fmt.Sprint(job.Params["password"])
	if host == "" || port == "" {
		return failedJobResult(job, started, fmt.Errorf("OpenSearch host and port are required"))
	}
	envContent := []byte(fmt.Sprintf("OS_HOST=%s\nOS_PORT=%s\nOS_USER=%s\nOS_PASSWORD=%s\n", host, port, username, password))
	dropInContent := []byte("[Service]\nEnvironmentFile=/etc/fluent-bit/djadmin-opensearch.env\n")
	envChanged, err := writeFileIfChanged("/etc/fluent-bit/djadmin-opensearch.env", envContent, 0o600)
	if err != nil {
		return failedJobResult(job, started, err)
	}
	dropInChanged, err := writeFileIfChanged("/etc/systemd/system/fluent-bit.service.d/djadmin.conf", dropInContent, 0o644)
	if err != nil {
		return failedJobResult(job, started, err)
	}
	if envChanged || dropInChanged {
		if output, err := exec.CommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput(); err != nil {
			return failedJobResult(job, started, fmt.Errorf("systemd daemon-reload failed: %s", strings.TrimSpace(string(output))))
		}
		if output, err := exec.CommandContext(ctx, "systemctl", "restart", "fluent-bit.service").CombinedOutput(); err != nil {
			return failedJobResult(job, started, fmt.Errorf("Fluent Bit restart failed: %s", strings.TrimSpace(string(output))))
		}
	}
	finished := time.Now()
	return protocol.JobResult{JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusSuccess, ExitCode: 0, StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds()}
}

// applyAgentUpdate 处理在线自更新：backend 已经通过 gRPC 文件通道把新二进制暂存到
// agentUpdateStagingBinaryPath，这里只做校验、原地替换配置/二进制，然后异步重启自己。
// 重启命令必须放在响应发出之后执行，否则 systemctl restart 杀掉当前进程时，
// 这次 gRPC 调用还没来得及把 JobResult 发回 backend。
func (e *Executor) applyAgentUpdate(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	if err := validateAgentBinaryFile(agentUpdateStagingBinaryPath); err != nil {
		return failedJobResult(job, started, err)
	}

	envContent := fmt.Sprint(job.Params["env_content"])
	if envContent != "" {
		if _, err := writeFileIfChanged(agentConfigLivePath, []byte(envContent), 0o600); err != nil {
			return failedJobResult(job, started, err)
		}
	}
	unitContent := fmt.Sprint(job.Params["unit_content"])
	if unitContent != "" {
		unitChanged, err := writeFileIfChanged(agentServiceUnitLivePath, []byte(unitContent), 0o644)
		if err != nil {
			return failedJobResult(job, started, err)
		}
		if unitChanged {
			if output, err := exec.CommandContext(ctx, "systemctl", "daemon-reload").CombinedOutput(); err != nil {
				return failedJobResult(job, started, fmt.Errorf("systemd daemon-reload failed: %s", strings.TrimSpace(string(output))))
			}
		}
	}

	if err := os.Chmod(agentUpdateStagingBinaryPath, 0o755); err != nil {
		return failedJobResult(job, started, err)
	}
	// 用 rename 而不是拷贝：同分区内是原子操作，不会让 systemd 在替换过程中读到半写文件。
	if err := os.Rename(agentUpdateStagingBinaryPath, agentBinaryLivePath); err != nil {
		return failedJobResult(job, started, fmt.Errorf("替换 dj-agent 二进制失败: %w", err))
	}

	finished := time.Now()
	result := protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusSuccess,
		ExitCode: 0, StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
	}
	go func() {
		time.Sleep(2 * time.Second)
		_ = exec.Command("systemctl", "restart", "dj-agent.service").Run()
	}()
	return result
}

// validateAgentBinaryFile 只做正向的 gRPC 标记检查：旧版本二进制的排除交给 backend 在
// 暂存/下发前用 _validate_agent_binary 完成，这里不能重复写死同一串字面量去比对——
// 字面量一旦出现在 Go 源码里，就会被编译进本次构建产物本身，导致往后所有新版二进制
// 都会"自证"命中旧版特征，永久把校验拖垮（历史上这个坑真的踩过）。
func validateAgentBinaryFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("暂存的新二进制不存在或为空: %s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("读取新二进制失败: %w", err)
	}
	defer file.Close()

	hasGRPCMarker, err := fileContainsMarker(file, []byte("DJ_AGENT_GRPC_FILE_ADDR"))
	if err != nil {
		return err
	}
	if !hasGRPCMarker {
		return fmt.Errorf("新二进制缺少当前 gRPC 配置标记，拒绝部署未知版本")
	}
	return nil
}

// fileContainsMarker 按块扫描文件查找 marker，跨块边界用重叠窗口拼接，避免整份读入内存。
func fileContainsMarker(file *os.File, marker []byte) (bool, error) {
	buffer := make([]byte, 1024*1024)
	var overlap []byte
	keep := len(marker) - 1
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			data := append(overlap, buffer[:n]...)
			if bytes.Contains(data, marker) {
				return true, nil
			}
			if keep > 0 {
				if len(data) > keep {
					overlap = append([]byte(nil), data[len(data)-keep:]...)
				} else {
					overlap = append([]byte(nil), data...)
				}
			}
		}
		if err == io.EOF {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
}

func (e *Executor) reloadFluentBit(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://127.0.0.1:2020/api/v2/reload", nil)
	if err != nil {
		return failedJobResult(job, started, err)
	}
	response, err := (&http.Client{Timeout: 15 * time.Second}).Do(request)
	if err != nil {
		return failedJobResult(job, started, fmt.Errorf("Fluent Bit 热重载请求失败: %w", err))
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return failedJobResult(job, started, fmt.Errorf("Fluent Bit 热重载返回 HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body))))
	}
	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusSuccess,
		ExitCode: 0, Stdout: string(body), StartedAt: started, FinishedAt: finished,
		CostMS: finished.Sub(started).Milliseconds(),
	}
}

func failedJobResult(job protocol.Job, started time.Time, err error) protocol.JobResult {
	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusFailed,
		ExitCode: 1, Error: err.Error(), StartedAt: started, FinishedAt: finished,
		CostMS: finished.Sub(started).Milliseconds(),
	}
}

func localIPv4Addresses() ([]string, error) {
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return nil, err
	}
	result := make([]string, 0)
	for _, address := range addresses {
		ipNet, ok := address.(*net.IPNet)
		if !ok || ipNet.IP == nil || ipNet.IP.IsLoopback() {
			continue
		}
		if ipv4 := ipNet.IP.To4(); ipv4 != nil {
			result = append(result, ipv4.String())
		}
	}
	sort.Strings(result)
	return result, nil
}

func (e *Executor) getLocalAddresses(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	if ctx.Err() != nil {
		return canceledJobResult(job, started, ctx.Err(), 0)
	}
	addresses, err := localIPv4Addresses()
	if err != nil {
		return protocol.JobResult{
			JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusFailed,
			ExitCode: 1, StartedAt: started, FinishedAt: time.Now(), Error: err.Error(),
		}
	}
	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action, Status: protocol.StatusSuccess,
		ExitCode: 0, StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
		Data: map[string]any{"local_ipv4": addresses},
	}
}

// canceledJobResult 构造因 ctx 取消而终止的任务结果。
func canceledJobResult(job protocol.Job, started time.Time, cause error, exitCode int) protocol.JobResult {
	finished := time.Now()
	return protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Status:     protocol.StatusCanceled,
		ExitCode:   exitCode,
		StartedAt:  started,
		FinishedAt: finished,
		CostMS:     finished.Sub(started).Milliseconds(),
		Error:      cause.Error(),
	}
}

// getAgentVersion 返回当前 agent 的版本与运行时信息
func (e *Executor) getAgentVersion(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	select {
	case <-ctx.Done():
		return canceledJobResult(job, started, ctx.Err(), 0)
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

// getHostInfo 返回当前主机基础信息与系统指标
func (e *Executor) getHostInfo(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	select {
	case <-ctx.Done():
		return canceledJobResult(job, started, ctx.Err(), 0)
	default:
	}

	// 收集基本主机信息
	hostname, err := os.Hostname()
	if err != nil {
		hostname = ""
	}

	ips, addrErr := localIPv4Addresses()

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

	// 收集静态资产信息（发行版/内核/CPU 型号/内存容量/磁盘容量），替代原后端 SSH 采集
	for key, value := range collectStaticInventory() {
		result.Data[key] = value
	}
	for key, value := range collectTimezoneInfo() {
		result.Data[key] = value
	}

	// 收集操作系统启动时间
	osUptimeMetrics, osUptimeErr := collectOSUptimeMetrics()
	if osUptimeErr != nil {
		result.Data["os_uptime_error"] = osUptimeErr.Error()
	} else {
		for key, value := range osUptimeMetrics {
			result.Data[key] = value
		}
	}

	// 收集系统性能指标
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
