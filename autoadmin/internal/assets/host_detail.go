package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
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
	var osType, osVersion, kernelVersion, hostname, agentVersion, timezoneName, utcOffset, collectorSource sql.NullString
	err := handler.service.repository.pool.QueryRowContext(ctx, `SELECT os_type,os_version,kernel_version,hostname,agent_version,timezone_name,utc_offset,collector_source FROM assets_hostsystem WHERE host_id=? LIMIT 1`, host.ID).Scan(&osType, &osVersion, &kernelVersion, &hostname, &agentVersion, &timezoneName, &utcOffset, &collectorSource)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return detail, err
	}
	if err == nil {
		detail.System = map[string]any{"os_type": nullStringValue(osType), "os_version": nullStringValue(osVersion), "kernel_version": nullStringValue(kernelVersion), "hostname": nullStringValue(hostname), "agent_version": nullStringValue(agentVersion), "timezone_name": nullStringValue(timezoneName), "utc_offset": nullStringValue(utcOffset), "collector_source": nullStringValue(collectorSource), "agent_last_seen_at": host.AgentOnlineTime, "agent_online": host.AgentOnline}
		detail.OSType, detail.OSVersion, detail.KernelVersion, detail.Hostname = nullStringValue(osType), nullStringValue(osVersion), nullStringValue(kernelVersion), nullStringValue(hostname)
	}
	var cpuCores sql.NullInt64
	var cpuModel, architecture sql.NullString
	var memoryGB, diskTotalGB sql.NullFloat64
	err = handler.service.repository.pool.QueryRowContext(ctx, `SELECT cpu_cores,cpu_model,memory_gb,disk_total_gb,architecture FROM assets_hosthardware WHERE host_id=? LIMIT 1`, host.ID).Scan(&cpuCores, &cpuModel, &memoryGB, &diskTotalGB, &architecture)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return detail, err
	}
	if err == nil {
		detail.Hardware = map[string]any{"cpu_cores": nullIntValue(cpuCores), "cpu_model": nullStringValue(cpuModel), "memory_gb": nullFloatValue(memoryGB), "disk_total_gb": nullFloatValue(diskTotalGB), "architecture": nullStringValue(architecture)}
		detail.CPUCores, detail.CPUModel, detail.MemoryGB, detail.DiskTotalGB, detail.Architecture = nullIntValue(cpuCores), nullStringValue(cpuModel), nullFloatValue(memoryGB), nullFloatValue(diskTotalGB), nullStringValue(architecture)
	}
	var cpuUsage, memoryUsage sql.NullFloat64
	var cpuTimes, memory, diskIO []byte
	var uptime sql.NullInt64
	var bootTime, collectedAt sql.NullTime
	var sampleWindow sql.NullInt64
	err = handler.service.repository.pool.QueryRowContext(ctx, `SELECT cpu_usage_percent,cpu_times,memory_usage_percent,memory,disk_io,os_uptime_seconds,os_boot_time,metrics_sample_window_ms,collected_at FROM assets_hostruntime WHERE host_id=? LIMIT 1`, host.ID).Scan(&cpuUsage, &cpuTimes, &memoryUsage, &memory, &diskIO, &uptime, &bootTime, &sampleWindow, &collectedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return detail, err
	}
	if err == nil {
		detail.Runtime = map[string]any{"cpu_usage_percent": nullFloatValue(cpuUsage), "cpu_times": hostJSONValue(cpuTimes, map[string]any{}), "memory_usage_percent": nullFloatValue(memoryUsage), "memory": hostJSONValue(memory, map[string]any{}), "disk_io": hostJSONValue(diskIO, []any{}), "os_uptime_seconds": nullIntValue(uptime), "os_boot_time": nullTimeValue(bootTime), "metrics_sample_window_ms": nullIntValue(sampleWindow), "collected_at": nullTimeValue(collectedAt)}
	}
	rows, err := handler.service.repository.pool.QueryContext(ctx, `SELECT device,mount_point,size_gb,used_gb,filesystem FROM assets_hostdisk WHERE host_id=? ORDER BY id`, host.ID)
	if err != nil {
		return detail, err
	}
	defer rows.Close()
	var total, used float64
	for rows.Next() {
		var device string
		var mountPoint, filesystem sql.NullString
		var sizeGB, usedGB sql.NullFloat64
		if err = rows.Scan(&device, &mountPoint, &sizeGB, &usedGB, &filesystem); err != nil {
			return detail, err
		}
		var usage any
		if sizeGB.Valid && sizeGB.Float64 > 0 && usedGB.Valid {
			usage = math.Round(usedGB.Float64/sizeGB.Float64*10000) / 100
			total += sizeGB.Float64
			used += usedGB.Float64
		}
		detail.Disks = append(detail.Disks, map[string]any{"device": device, "mount_point": nullStringValue(mountPoint), "size_gb": nullFloatValue(sizeGB), "used_gb": nullFloatValue(usedGB), "filesystem": nullStringValue(filesystem), "usage_percent": usage})
	}
	if total > 0 {
		detail.DiskUsedPercent = math.Round(used/total*10000) / 100
		if hardware, ok := detail.Hardware.(map[string]any); ok {
			hardware["disk_used_percent"] = detail.DiskUsedPercent
		}
	}
	if err = rows.Err(); err != nil {
		return detail, err
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
	rows, err := handler.service.repository.pool.QueryContext(ctx, `SELECT id,exporter_type,scrape_port,managed_enabled,install_status,install_message,retry_count,update_time FROM monitor_target WHERE host_id=? ORDER BY id DESC`, hostID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	monitors := []any{}
	for rows.Next() {
		var id int64
		var exporterType string
		var scrapePort int64
		var managedEnabled bool
		var installStatus, installMessage sql.NullString
		var retryCount int64
		var updateTime sql.NullTime
		if err = rows.Scan(&id, &exporterType, &scrapePort, &managedEnabled, &installStatus, &installMessage, &retryCount, &updateTime); err != nil {
			return nil, err
		}
		status := installStatus.String
		if status == "" {
			status = "unknown"
		}
		monitors = append(monitors, map[string]any{
			"id":              id,
			"name":            exporterType,
			"port":            scrapePort,
			"enabled":         managedEnabled,
			"install_status":  status,
			"install_message": nullStringValue(installMessage),
			"retry_count":     retryCount,
			"update_time":     nullTimeValue(updateTime),
		})
	}
	return monitors, rows.Err()
}

func nullStringValue(value sql.NullString) any {
	if value.Valid {
		return value.String
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
func hostJSONValue(raw []byte, fallback any) any {
	if len(raw) == 0 {
		return fallback
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return fallback
	}
	return value
}
