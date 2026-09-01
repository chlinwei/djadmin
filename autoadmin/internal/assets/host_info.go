package assets

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
)

// hostInfoOutcome mirrors Django's refresh_host_info() return contract so the frontend's
// existing {result, host} handling keeps working unchanged.
type hostInfoOutcome struct {
	HostID  int64  `json:"host_id"`
	Updated bool   `json:"updated"`
	Skipped bool   `json:"skipped"`
	Error   string `json:"error"`
}

// refreshHostAgentInfo dispatches a synchronous get_host_info job to the host's agent and
// persists the result, mirroring Django's refresh_host_info + persist_host_info (host_info.py).
// Requires the caller to have already called applyAgentPresence(&host) for a fresh online check.
func (h *Handler) refreshHostAgentInfo(ctx context.Context, host Host) hostInfoOutcome {
	outcome := hostInfoOutcome{HostID: host.ID}

	agentID := ""
	if host.AgentID != nil {
		agentID = strings.TrimSpace(*host.AgentID)
	}
	if agentID == "" {
		outcome.Skipped = true
		outcome.Error = "主机未配置 agent_id，无法定位 agent"
		return outcome
	}
	if !host.AgentOnline {
		outcome.Skipped = true
		outcome.Error = "agent 离线"
		return outcome
	}

	response, err := h.gateway.Execute(ctx, agentID, &pb.AutomationExecuteRequest{
		JobId:          fmt.Sprintf("host-info-%d-%d", host.ID, time.Now().UnixNano()),
		Type:           "inventory",
		Action:         "get_host_info",
		ParamsJson:     "{}",
		TimeoutSeconds: 15,
	})
	if err != nil {
		if _, persistErr := h.service.persistHostInfo(ctx, host.ID, "failed", nil, err.Error()); persistErr != nil {
			outcome.Error = persistErr.Error()
			return outcome
		}
		outcome.Error = err.Error()
		return outcome
	}

	var resultData map[string]any
	if response.ResultDataJson != "" {
		_ = json.Unmarshal([]byte(response.ResultDataJson), &resultData)
	}
	updated, err := h.service.persistHostInfo(ctx, host.ID, response.Status, resultData, response.ErrorMessage)
	if err != nil {
		outcome.Error = err.Error()
		return outcome
	}
	outcome.Updated = updated
	if !updated && response.ErrorMessage != "" {
		outcome.Error = response.ErrorMessage
	}
	return outcome
}

// persistHostInfo writes one get_host_info result into assets_host/hostsystem/hosthardware/
// hostruntime/hostdisk, matching Django's persist_host_info() field-for-field.
func (s *Service) persistHostInfo(ctx context.Context, hostID int64, status string, resultData map[string]any, errorMessage string) (bool, error) {
	pool := s.repository.pool
	now := time.Now().UTC()
	isSuccess := status == "success"

	if isSuccess {
		if _, err := pool.ExecContext(ctx, `UPDATE assets_host SET collect_status=?,collect_message=?,collect_time=?,update_time=? WHERE id=?`, "success", errorMessage, now, now, hostID); err != nil {
			return false, err
		}
	} else {
		if _, err := pool.ExecContext(ctx, `UPDATE assets_host SET collect_status=?,collect_message=?,update_time=? WHERE id=?`, "failed", errorMessage, now, hostID); err != nil {
			return false, err
		}
	}
	if !isSuccess || len(resultData) == 0 {
		return false, nil
	}

	disks := normalizeHostDisks(resultData["disks"])
	fingerprint := buildStaticFingerprint(resultData, disks)

	var previousFingerprint sql.NullString
	err := pool.QueryRowContext(ctx, `SELECT static_fingerprint FROM assets_hostruntime WHERE host_id=? LIMIT 1`, hostID).Scan(&previousFingerprint)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}

	cpuTimes := jsonOrDefault(resultData["cpu_times"], map[string]any{})
	memory := jsonOrDefault(resultData["memory"], map[string]any{})
	diskIO := jsonOrDefault(resultData["disk_io"], []any{})

	_, err = pool.ExecContext(ctx, `INSERT INTO assets_hostruntime(create_time,update_time,remark,host_id,cpu_usage_percent,cpu_times,memory_usage_percent,memory,disk_io,os_uptime_seconds,os_boot_time,metrics_sample_window_ms,static_fingerprint,collected_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),cpu_usage_percent=VALUES(cpu_usage_percent),cpu_times=VALUES(cpu_times),memory_usage_percent=VALUES(memory_usage_percent),memory=VALUES(memory),disk_io=VALUES(disk_io),os_uptime_seconds=VALUES(os_uptime_seconds),os_boot_time=VALUES(os_boot_time),metrics_sample_window_ms=VALUES(metrics_sample_window_ms),static_fingerprint=VALUES(static_fingerprint),collected_at=VALUES(collected_at)`,
		now, now, "", hostID, floatOrNil(resultData["cpu_usage_percent"]), cpuTimes, floatOrNil(resultData["memory_usage_percent"]), memory, diskIO, intOrNil(resultData["os_uptime_seconds"]), timeOrNil(resultData["os_boot_time"]), intOrNil(resultData["metrics_sample_window_ms"]), fingerprint, now)
	if err != nil {
		return false, err
	}

	if previousFingerprint.Valid && previousFingerprint.String != "" && previousFingerprint.String == fingerprint {
		// Static assets unchanged: dynamic runtime snapshot above is already fresh, skip the rest.
		return true, nil
	}

	osType := stringOrEmpty(resultData["os_type"])
	if osType == "" {
		osType = stringOrEmpty(resultData["os"])
	}
	_, err = pool.ExecContext(ctx, `INSERT INTO assets_hostsystem(create_time,update_time,remark,host_id,os_type,os_version,os_id,os_id_like,os_version_id,kernel_version,hostname,agent_version,timezone_name,utc_offset,collector_source,collected_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),os_type=VALUES(os_type),os_version=VALUES(os_version),os_id=VALUES(os_id),os_id_like=VALUES(os_id_like),os_version_id=VALUES(os_version_id),kernel_version=VALUES(kernel_version),hostname=VALUES(hostname),agent_version=VALUES(agent_version),timezone_name=VALUES(timezone_name),utc_offset=VALUES(utc_offset),collector_source=VALUES(collector_source),collected_at=VALUES(collected_at)`,
		now, now, "", hostID, nullableStr(osType), nullableStr(stringOrEmpty(resultData["os_version"])), nullableStr(strings.ToLower(stringOrEmpty(resultData["os_id"]))), nullableStr(strings.ToLower(stringOrEmpty(resultData["os_id_like"]))), nullableStr(stringOrEmpty(resultData["os_version_id"])), nullableStr(stringOrEmpty(resultData["kernel_version"])), nullableStr(stringOrEmpty(resultData["hostname"])), nullableStr(stringOrEmpty(resultData["agent_version"])), nullableStr(stringOrEmpty(resultData["os_timezone"])), nullableStr(stringOrEmpty(resultData["os_utc_offset"])), "agent", now)
	if err != nil {
		return false, err
	}

	var diskTotalGB any
	total := 0.0
	for _, disk := range disks {
		if size, ok := disk["size_gb"].(float64); ok {
			total += size
		}
	}
	if total > 0 {
		diskTotalGB = float64(int(total*10+0.5)) / 10
	}

	_, err = pool.ExecContext(ctx, `INSERT INTO assets_hosthardware(create_time,update_time,remark,host_id,cpu_cores,cpu_model,memory_gb,disk_total_gb,architecture,collected_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON DUPLICATE KEY UPDATE update_time=VALUES(update_time),cpu_cores=VALUES(cpu_cores),cpu_model=VALUES(cpu_model),memory_gb=VALUES(memory_gb),disk_total_gb=VALUES(disk_total_gb),architecture=VALUES(architecture),collected_at=VALUES(collected_at)`,
		now, now, "", hostID, intOrNil(resultData["cpu_count"]), nullableStr(stringOrEmpty(resultData["cpu_model"])), floatOrNil(resultData["memory_total_gb"]), diskTotalGB, nullableStr(stringOrEmpty(resultData["arch"])), now)
	if err != nil {
		return false, err
	}

	// Disk table has no unique key per device: rebuild fully to drop unmounted/removed partitions.
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM assets_hostdisk WHERE host_id=?`, hostID); err != nil {
		tx.Rollback()
		return false, err
	}
	for _, disk := range disks {
		device, _ := disk["device"].(string)
		if device == "" {
			continue
		}
		var mountPoint, filesystem any
		if v, ok := disk["mount_point"].(string); ok && v != "" {
			mountPoint = v
		}
		if v, ok := disk["filesystem"].(string); ok && v != "" {
			filesystem = v
		}
		var sizeGB, usedGB any
		if v, ok := disk["size_gb"].(float64); ok {
			sizeGB = v
		}
		if v, ok := disk["used_gb"].(float64); ok {
			usedGB = v
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO assets_hostdisk(host_id,device,mount_point,size_gb,used_gb,filesystem) VALUES(?,?,?,?,?,?)`, hostID, device, mountPoint, sizeGB, usedGB, filesystem); err != nil {
			tx.Rollback()
			return false, err
		}
	}
	if err = tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// normalizeHostDisks filters out unnamed/squashfs entries and sorts deterministically, matching
// Django's _normalize_disks() so the static fingerprint is stable regardless of report order.
func normalizeHostDisks(raw any) []map[string]any {
	items, _ := raw.([]any)
	normalized := make([]map[string]any, 0, len(items))
	for _, rawItem := range items {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		device := strings.TrimSpace(stringOrEmpty(item["device"]))
		if device == "" {
			continue
		}
		filesystem := strings.TrimSpace(stringOrEmpty(item["filesystem"]))
		if strings.EqualFold(filesystem, "squashfs") {
			continue
		}
		entry := map[string]any{"device": device, "mount_point": strings.TrimSpace(stringOrEmpty(item["mount_point"]))}
		if filesystem != "" {
			entry["filesystem"] = filesystem
		}
		if v := floatOrNil(item["size_gb"]); v != nil {
			entry["size_gb"] = v
		}
		if v := floatOrNil(item["used_gb"]); v != nil {
			entry["used_gb"] = v
		}
		normalized = append(normalized, entry)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i]["device"] != normalized[j]["device"] {
			return fmt.Sprint(normalized[i]["device"]) < fmt.Sprint(normalized[j]["device"])
		}
		return fmt.Sprint(normalized[i]["mount_point"]) < fmt.Sprint(normalized[j]["mount_point"])
	})
	return normalized
}

// buildStaticFingerprint hashes the fields that HostSystem/HostHardware/HostDisk care about, so
// unchanged static assets can skip those writes on every periodic refresh (see Django's
// _build_static_fingerprint). The hash only needs to be stable within this process/table, not
// cross-language identical to Django's.
func buildStaticFingerprint(resultData map[string]any, disks []map[string]any) string {
	osType := stringOrEmpty(resultData["os_type"])
	if osType == "" {
		osType = stringOrEmpty(resultData["os"])
	}
	payload := map[string]any{
		"os_type":         nullableStr(osType),
		"os_version":      nullableStr(stringOrEmpty(resultData["os_version"])),
		"os_id":           nullableStr(strings.ToLower(stringOrEmpty(resultData["os_id"]))),
		"os_id_like":      nullableStr(strings.ToLower(stringOrEmpty(resultData["os_id_like"]))),
		"os_version_id":   nullableStr(stringOrEmpty(resultData["os_version_id"])),
		"kernel_version":  nullableStr(stringOrEmpty(resultData["kernel_version"])),
		"hostname":        nullableStr(stringOrEmpty(resultData["hostname"])),
		"agent_version":   nullableStr(stringOrEmpty(resultData["agent_version"])),
		"cpu_count":       intOrNil(resultData["cpu_count"]),
		"cpu_model":       nullableStr(stringOrEmpty(resultData["cpu_model"])),
		"memory_total_gb": floatOrNil(resultData["memory_total_gb"]),
		"arch":            nullableStr(stringOrEmpty(resultData["arch"])),
		"os_timezone":     nullableStr(stringOrEmpty(resultData["os_timezone"])),
		"os_utc_offset":   nullableStr(stringOrEmpty(resultData["os_utc_offset"])),
		"disks":           disks,
	}
	// encoding/json marshals map keys in sorted order, giving a stable hash input.
	encoded, _ := json.Marshal(payload)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func stringOrEmpty(value any) string {
	s, _ := value.(string)
	return strings.TrimSpace(s)
}

func nullableStr(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func floatOrNil(value any) any {
	switch v := value.(type) {
	case float64:
		return v
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil
		}
		return parsed
	default:
		return nil
	}
}

func intOrNil(value any) any {
	switch v := value.(type) {
	case float64:
		return int64(v)
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil
		}
		return parsed
	default:
		return nil
	}
}

func timeOrNil(value any) any {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil
	}
	return parsed.UTC()
}

func jsonOrDefault(value any, fallback any) []byte {
	if value == nil {
		encoded, _ := json.Marshal(fallback)
		return encoded
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		encoded, _ = json.Marshal(fallback)
	}
	return encoded
}
