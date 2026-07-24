package executor

import (
	"context"
	"fmt"
	"net"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

const (
	actionGetAgentVersion     = "get_agent_version"
	actionGetHostInfo         = "get_host_info"
	actionRunAutomationTask   = "run_automation_task"
	actionStartExporter       = "start_exporter"
	actionStopExporter        = "stop_exporter"
	actionCheckExporterStatus = "check_exporter_status"
	defaultAgentVersion       = "v1"
)

// runBuiltinAction 分发并执行内置操作
func (e *Executor) runBuiltinAction(ctx context.Context, job protocol.Job) (protocol.JobResult, bool) {
	switch strings.TrimSpace(job.Action) {
	case actionGetAgentVersion:
		return e.getAgentVersion(ctx, job), true
	case actionGetHostInfo:
		return e.getHostInfo(ctx, job), true
	case actionRunAutomationTask:
		return e.runAutomationTask(ctx, job), true
	case actionStartExporter:
		return e.startExporter(ctx, job), true
	case actionStopExporter:
		return e.stopExporter(ctx, job), true
	case actionCheckExporterStatus:
		return e.checkExporterStatus(ctx, job), true
	default:
		return protocol.JobResult{}, false
	}
}

// getAgentVersion 返回当前 agent 的版本与运行时信息
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

// getHostInfo 返回当前主机基础信息与系统指标
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

	// 收集基本主机信息
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

	// 收集静态资产信息（发行版/内核/CPU 型号/内存容量/磁盘容量），替代原后端 SSH 采集
	for key, value := range collectStaticInventory() {
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
