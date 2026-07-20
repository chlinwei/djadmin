package executor

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const defaultMetricsSampleWindow = 200 * time.Millisecond
const minMetricsSampleWindow = 100 * time.Millisecond
const maxMetricsSampleWindow = 2 * time.Second

// cpuStatSnapshot 表示 /proc/stat 中的 CPU 统计快照
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

// total 计算 CPU 统计的总时间
func (s cpuStatSnapshot) total() uint64 {
	return s.user + s.system + s.nice + s.idle + s.iowait + s.hirq + s.sirq + s.steal
}

// diskStatSnapshot 表示 /proc/diskstats 中的磁盘 IO 统计快照
type diskStatSnapshot struct {
	readSectors  uint64
	writeSectors uint64
}

// collectCPUMetrics 采集 CPU 指标（使用指定采样窗口）
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

// readCPUStatSnapshot 从 /proc/stat 读取 CPU 统计快照
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

// collectMemoryMetrics 采集内存指标
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

// formatKBToHuman 将 KB 转换为可读格式（GiB）
func formatKBToHuman(kb uint64) string {
	const kbPerGiB = 1024.0 * 1024.0
	giB := float64(kb) / kbPerGiB
	if giB >= 10 {
		return fmt.Sprintf("%.0fGi", giB)
	}
	return fmt.Sprintf("%.1fGi", giB)
}

// collectOSUptimeMetrics 采集操作系统启动时间指标
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

// collectDiskIOMetrics 采集磁盘 IO 指标（使用指定采样窗口）
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

// readDiskStatSnapshots 从 /proc/diskstats 读取所有磁盘 IO 统计快照
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

// shouldSkipDiskDevice 判断是否应该跳过该磁盘设备（虚拟设备等）
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

// resolveDiskStatAliases 获取磁盘设备的别名（如 dm-* 对应的 mapper 名称）
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

// formatBytesPerSecond 将字节/秒转换为可读格式
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

// resolveMetricsSampleWindow 从环境变量解析指标采样窗口（毫秒）
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
