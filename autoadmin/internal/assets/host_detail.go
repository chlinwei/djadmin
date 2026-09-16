package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"

	db "autoadmin/internal/platform/database/generated"
)

type HostDetail struct {
	Host
	System          any              `json:"system"`
	Hardware        any              `json:"hardware"`
	Runtime         any              `json:"runtime"`
	Disks           []map[string]any `json:"disks"`
	Monitors        []any            `json:"monitors"`
	OSType          any              `json:"os_type"`
	OSVersion       any              `json:"os_version"`
	KernelVersion   any              `json:"kernel_version"`
	Hostname        any              `json:"hostname"`
	CPUCores        any              `json:"cpu_cores"`
	CPUModel        any              `json:"cpu_model"`
	MemoryGB        any              `json:"memory_gb"`
	DiskTotalGB     any              `json:"disk_total_gb"`
	DiskUsedPercent any              `json:"disk_used_percent"`
	Architecture    any              `json:"architecture"`
	LastCollectTime *string          `json:"last_collect_time"`
}

func (handler *Handler) getHostDetail(ctx context.Context, host Host) (HostDetail, error) {
	detail := HostDetail{Host: host, Disks: []map[string]any{}, Monitors: []any{}, LastCollectTime: host.CollectTime}
	queries := db.New(handler.service.repository.pool)

	system, err := queries.GetHostSystem(ctx, host.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return detail, err
	}
	if err == nil {
		systemMap := map[string]any{"os_type": nullStringValue(system.OsType), "os_version": nullStringValue(system.OsVersion), "kernel_version": nullStringValue(system.KernelVersion), "hostname": nullStringValue(system.Hostname), "agent_version": nullStringValue(system.AgentVersion), "timezone_name": nullStringValue(system.TimezoneName), "utc_offset": nullStringValue(system.UtcOffset), "collector_source": nullStringValue(system.CollectorSource), "agent_last_seen_at": host.AgentOnlineTime, "agent_online": host.AgentOnline}
		detail.System = systemMap
		detail.OSType, detail.OSVersion = nullStringValue(system.OsType), nullStringValue(system.OsVersion)
		detail.KernelVersion, detail.Hostname = nullStringValue(system.KernelVersion), nullStringValue(system.Hostname)
	}

	hardware, err := queries.GetHostHardware(ctx, host.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return detail, err
	}
	if err == nil {
		hardwareMap := map[string]any{"cpu_cores": nullInt32Value(hardware.CpuCores), "cpu_model": nullStringValue(hardware.CpuModel), "memory_gb": nullFloatValue(hardware.MemoryGb), "disk_total_gb": nullFloatValue(hardware.DiskTotalGb), "architecture": nullStringValue(hardware.Architecture)}
		detail.Hardware = hardwareMap
		detail.CPUCores, detail.CPUModel = nullInt32Value(hardware.CpuCores), nullStringValue(hardware.CpuModel)
		detail.MemoryGB, detail.DiskTotalGB = nullFloatValue(hardware.MemoryGb), nullFloatValue(hardware.DiskTotalGb)
		detail.Architecture = nullStringValue(hardware.Architecture)
	}

	runtime, err := queries.GetHostRuntime(ctx, host.ID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return detail, err
	}
	if err == nil {
		detail.Runtime = map[string]any{"cpu_usage_percent": nullFloatValue(runtime.CpuUsagePercent), "cpu_times": hostJSONValue(runtime.CpuTimes, map[string]any{}), "memory_usage_percent": nullFloatValue(runtime.MemoryUsagePercent), "memory": hostJSONValue(runtime.Memory, map[string]any{}), "disk_io": hostJSONValue(runtime.DiskIo, []any{}), "os_uptime_seconds": nullIntValue(runtime.OsUptimeSeconds), "os_boot_time": nullTimeValue(runtime.OsBootTime), "metrics_sample_window_ms": nullInt32Value(runtime.MetricsSampleWindowMs), "collected_at": nullTimeValue(runtime.CollectedAt)}
	}

	diskRows, err := queries.ListHostDisks(ctx, host.ID)
	if err != nil {
		return detail, err
	}
	var total, used float64
	for _, disk := range diskRows {
		var usage any
		if disk.SizeGb.Valid && disk.SizeGb.Float64 > 0 && disk.UsedGb.Valid {
			usage = math.Round(disk.UsedGb.Float64/disk.SizeGb.Float64*10000) / 100
			total += disk.SizeGb.Float64
			used += disk.UsedGb.Float64
		}
		detail.Disks = append(detail.Disks, map[string]any{"device": disk.Device, "mount_point": nullStringValue(disk.MountPoint), "size_gb": nullFloatValue(disk.SizeGb), "used_gb": nullFloatValue(disk.UsedGb), "filesystem": nullStringValue(disk.Filesystem), "usage_percent": usage})
	}
	if total > 0 {
		detail.DiskUsedPercent = math.Round(used/total*10000) / 100
		if hardwareMap, ok := detail.Hardware.(map[string]any); ok {
			hardwareMap["disk_used_percent"] = detail.DiskUsedPercent
		}
	}

	monitors, err := handler.getHostMonitors(ctx, host.ID)
	if err != nil {
		return detail, err
	}
	detail.Monitors = monitors
	return detail, nil
}

// getHostMonitors 与 Django assets.serializer.HostDetailSerializer.get_monitors 保持字段一致，
// 前端"性能监控" tab 依赖 monitors[].name=="node_exporter" && enabled==true 判断是否展示。
func (handler *Handler) getHostMonitors(ctx context.Context, hostID int64) ([]any, error) {
	rows, err := db.New(handler.service.repository.pool).ListHostMonitors(ctx, hostID)
	if err != nil {
		return nil, err
	}
	monitors := make([]any, 0, len(rows))
	for _, row := range rows {
		status := row.InstallStatus
		if status == "" {
			status = "unknown"
		}
		monitors = append(monitors, map[string]any{
			"id":              row.ID,
			"name":            row.ExporterType,
			"port":            row.ScrapePort,
			"enabled":         row.ManagedEnabled,
			"install_status":  status,
			"install_message": nullStringValue(sql.NullString{String: row.InstallMessage, Valid: row.InstallMessage != ""}),
			"retry_count":     row.RetryCount,
			"update_time":     row.UpdateTime,
		})
	}
	return monitors, nil
}

func nullStringValue(value sql.NullString) any {
	if value.Valid {
		return value.String
	}
	return nil
}
func nullInt32Value(value sql.NullInt32) any {
	if value.Valid {
		return value.Int32
	}
	return nil
}
func nullIntValue(value sql.NullInt64) any {
	if value.Valid {
		return value.Int64
	}
	return nil
}
func nullFloatValue(value sql.NullFloat64) any {
	if value.Valid {
		return value.Float64
	}
	return nil
}
func nullTimeValue(value sql.NullTime) any {
	if value.Valid {
		return value.Time
	}
	return nil
}
func hostJSONValue(raw json.RawMessage, fallback any) any {
	if len(raw) == 0 {
		return fallback
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return fallback
	}
	return value
}
