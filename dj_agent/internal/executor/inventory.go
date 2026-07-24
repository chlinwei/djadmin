package executor

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// collectStaticInventory 采集主机静态资产信息（OS 发行版/内核/CPU 型号/内存容量/磁盘容量）。
// 这些字段原先由后端 SSH(paramiko) 周期采集，现改为 agent 本地读取 /proc 与 /etc/os-release，
// 供 get_host_info 一次性返回，避免后端再持有主机凭据做远程采集。
func collectStaticInventory() map[string]any {
	data := map[string]any{}

	if osType, osVersion := readOSRelease(); osType != "" || osVersion != "" {
		if osType != "" {
			data["os_type"] = osType
		}
		if osVersion != "" {
			data["os_version"] = osVersion
		}
	}

	if kernel := readKernelVersion(); kernel != "" {
		data["kernel_version"] = kernel
	}

	if model := readCPUModel(); model != "" {
		data["cpu_model"] = model
	}

	if totalGB, ok := readMemoryTotalGB(); ok {
		data["memory_total_gb"] = totalGB
	}

	if disks := collectDiskUsage(); len(disks) > 0 {
		data["disks"] = disks
	}

	return data
}

// readOSRelease 解析 /etc/os-release，返回发行版名称(NAME)与版本(VERSION 优先，回退 VERSION_ID)。
func readOSRelease() (osType string, osVersion string) {
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "", ""
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		values[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	osType = values["NAME"]
	osVersion = values["VERSION"]
	if osVersion == "" {
		osVersion = values["VERSION_ID"]
	}
	return osType, osVersion
}

// readKernelVersion 读取内核版本（等价于 uname -r）。
func readKernelVersion() string {
	content, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(content))
}

// readCPUModel 从 /proc/cpuinfo 读取 CPU 型号（首个 model name）。
func readCPUModel() string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		if strings.TrimSpace(key) == "model name" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// readMemoryTotalGB 从 /proc/meminfo 读取 MemTotal 并换算为 GB（二进制 GiB，保留一位小数）。
func readMemoryTotalGB() (float64, bool) {
	content, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, false
		}
		kb, convErr := strconv.ParseUint(fields[1], 10, 64)
		if convErr != nil || kb == 0 {
			return 0, false
		}
		gib := float64(kb) / (1024.0 * 1024.0)
		// 保留一位小数
		return float64(int64(gib*10+0.5)) / 10, true
	}
	return 0, false
}

// collectDiskUsage 读取 /proc/mounts 并对真实块设备(/dev/*)执行 statfs，
// 返回每个挂载点的容量/已用/使用率信息。忽略 tmpfs、proc、cgroup 等伪文件系统。
func collectDiskUsage() []map[string]any {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		return nil
	}
	defer file.Close()

	disks := make([]map[string]any, 0)
	seen := map[string]bool{} // 按设备去重，避免 bind mount 重复统计

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		device := fields[0]
		mountPoint := unescapeMountField(fields[1])
		fsType := fields[2]

		// 仅统计真实块设备，跳过 tmpfs/proc/sysfs/cgroup/overlay 等伪文件系统
		if !strings.HasPrefix(device, "/dev/") {
			continue
		}
		// squashfs 常见于 snap 只读镜像挂载，不属于业务磁盘分区，采集时直接过滤。
		if strings.EqualFold(strings.TrimSpace(fsType), "squashfs") {
			continue
		}
		if seen[device] {
			continue
		}

		var stat syscall.Statfs_t
		if statErr := syscall.Statfs(mountPoint, &stat); statErr != nil {
			continue
		}
		if stat.Blocks == 0 {
			continue
		}

		blockSize := uint64(stat.Bsize)
		const bytesPerGiB = 1024.0 * 1024.0 * 1024.0
		totalGB := float64(stat.Blocks*blockSize) / bytesPerGiB
		freeGB := float64(stat.Bfree*blockSize) / bytesPerGiB
		availGB := float64(stat.Bavail*blockSize) / bytesPerGiB
		usedGB := totalGB - freeGB

		// 使用率按 df 语义计算：used / (used + available)
		usagePercent := 0.0
		if denom := usedGB + availGB; denom > 0 {
			usagePercent = usedGB / denom * 100
		}

		seen[device] = true
		disks = append(disks, map[string]any{
			"device":        device,
			"mount_point":   mountPoint,
			"filesystem":    fsType,
			"size_gb":       roundTo1(totalGB),
			"used_gb":       roundTo1(usedGB),
			"usage_percent": roundTo1(usagePercent),
		})
	}

	return disks
}

// unescapeMountField 还原 /proc/mounts 中被转义的路径（空格为 \040 等八进制序列）。
func unescapeMountField(field string) string {
	if !strings.Contains(field, `\`) {
		return field
	}
	var builder strings.Builder
	for i := 0; i < len(field); i++ {
		if field[i] == '\\' && i+3 < len(field) {
			if code, convErr := strconv.ParseUint(field[i+1:i+4], 8, 8); convErr == nil {
				builder.WriteByte(byte(code))
				i += 3
				continue
			}
		}
		builder.WriteByte(field[i])
	}
	return builder.String()
}

// roundTo1 将浮点数四舍五入保留一位小数。
func roundTo1(value float64) float64 {
	if value < 0 {
		value = 0
	}
	return float64(int64(value*10+0.5)) / 10
}
