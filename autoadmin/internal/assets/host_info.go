package assets

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	db "autoadmin/internal/platform/database/generated"
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
// persists the result, mirroring the Django-era refresh_host_info + persist_host_info.
// Requires the caller to have already called applyAgentPresence(&host) for a fresh online check.
func (h *Handler) refreshHostAgentInfo(ctx context.Context, host Host) hostInfoOutcome {
	outcome := hostInfoOutcome{HostID: host.ID}

	instanceName := ""
	if host.InstanceName != nil {
		instanceName = strings.TrimSpace(*host.InstanceName)
	}
	if instanceName == "" {
		outcome.Skipped = true
		outcome.Error = "主机未配置实例名，无法定位 agent"
		return outcome
	}
	if !host.AgentOnline {
		outcome.Skipped = true
		outcome.Error = "agent 离线"
		return outcome
	}

	response, err := h.gateway.Execute(ctx, instanceName, &pb.AutomationExecuteRequest{
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

// RefreshHostInfoByID 供其他域（如 monitor 的 exporter 目标安装）在缺少主机平台/架构信息时
// 主动补采一次资产信息并落库：同步向 agent 下发 get_host_info，成功后 hardware/system 等
// 平台字段即可用于选包。返回采集失败原因，供调用方决定是否继续或提示。
func (h *Handler) RefreshHostInfoByID(ctx context.Context, hostID int64) error {
	item, err := h.service.GetHost(ctx, hostID)
	if err != nil {
		return err
	}
	h.applyAgentPresence(&item)
	outcome := h.refreshHostAgentInfo(ctx, item)
	if outcome.Error != "" {
		return errors.New(outcome.Error)
	}
	return nil
}

// persistHostInfo writes one get_host_info result into assets_host/hostsystem/hosthardware/
// hostruntime/hostdisk, matching Django's persist_host_info() field-for-field.
func (s *Service) persistHostInfo(ctx context.Context, hostID int64, status string, resultData map[string]any, errorMessage string) (bool, error) {
	pool := s.repository.pool
	now := time.Now().UTC()
	isSuccess := status == "success"

	queries := db.New(pool)
	// 成功时写 collect_time；失败时传 NULL 保留上次采集时间（原实现是两条 UPDATE）。
	collectTime := sql.NullTime{}
	if isSuccess {
		collectTime = sql.NullTime{Time: now, Valid: true}
	}
	if err := queries.MarkHostCollected(ctx, db.MarkHostCollectedParams{
		CollectStatus: status, CollectMessage: errorMessage, CollectTime: collectTime,
		UpdateTime: now, ID: hostID,
	}); err != nil {
		return false, err
	}
	if !isSuccess || len(resultData) == 0 {
		return false, nil
	}

	disks := normalizeHostDisks(resultData["disks"])
	fingerprint := buildStaticFingerprint(resultData, disks)

	previousFingerprint, err := queries.GetHostRuntimeFingerprint(ctx, hostID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	cpuTimes := jsonOrDefault(resultData["cpu_times"], map[string]any{})
	memory := jsonOrDefault(resultData["memory"], map[string]any{})
	diskIO := jsonOrDefault(resultData["disk_io"], []any{})

	if err = queries.UpsertHostRuntime(ctx, db.UpsertHostRuntimeParams{
		CreateTime: now, UpdateTime: now, HostID: hostID,
		CpuUsagePercent:       nullFloat64Of(floatOrNil(resultData["cpu_usage_percent"])),
		CpuTimes:              json.RawMessage(cpuTimes),
		MemoryUsagePercent:    nullFloat64Of(floatOrNil(resultData["memory_usage_percent"])),
		Memory:                json.RawMessage(memory),
		DiskIo:                json.RawMessage(diskIO),
		OsUptimeSeconds:       nullInt64Of(intOrNil(resultData["os_uptime_seconds"])),
		OsBootTime:            nullTimeOf(timeOrNil(resultData["os_boot_time"])),
		MetricsSampleWindowMs: nullInt32Of(intOrNil(resultData["metrics_sample_window_ms"])),
		StaticFingerprint:     fingerprint,
		CollectedAt:           sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return false, err
	}

	if previousFingerprint != "" && previousFingerprint == fingerprint {
		// Static assets unchanged: dynamic runtime snapshot above is already fresh, skip the rest.
		return true, nil
	}

	osType := stringOrEmpty(resultData["os_type"])
	if osType == "" {
		osType = stringOrEmpty(resultData["os"])
	}
	if err = queries.UpsertHostSystem(ctx, db.UpsertHostSystemParams{
		CreateTime: now, UpdateTime: now, HostID: hostID,
		OsType:          nullableStrField(osType),
		OsVersion:       nullableStrField(stringOrEmpty(resultData["os_version"])),
		OsID:            nullableStrField(strings.ToLower(stringOrEmpty(resultData["os_id"]))),
		OsIDLike:        nullableStrField(strings.ToLower(stringOrEmpty(resultData["os_id_like"]))),
		OsVersionID:     nullableStrField(stringOrEmpty(resultData["os_version_id"])),
		KernelVersion:   nullableStrField(stringOrEmpty(resultData["kernel_version"])),
		Hostname:        nullableStrField(stringOrEmpty(resultData["hostname"])),
		AgentVersion:    nullableStrField(stringOrEmpty(resultData["agent_version"])),
		TimezoneName:    nullableStrField(stringOrEmpty(resultData["os_timezone"])),
		UtcOffset:       nullableStrField(stringOrEmpty(resultData["os_utc_offset"])),
		CollectorSource: sql.NullString{String: "agent", Valid: true},
		CollectedAt:     sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return false, err
	}

	diskTotalGB := sql.NullFloat64{}
	total := 0.0
	for _, disk := range disks {
		if size, ok := disk["size_gb"].(float64); ok {
			total += size
		}
	}
	if total > 0 {
		diskTotalGB = sql.NullFloat64{Float64: float64(int(total*10+0.5)) / 10, Valid: true}
	}

	if err = queries.UpsertHostHardware(ctx, db.UpsertHostHardwareParams{
		CreateTime: now, UpdateTime: now, HostID: hostID,
		CpuCores:     nullInt32Of(intOrNil(resultData["cpu_count"])),
		CpuModel:     nullableStrField(stringOrEmpty(resultData["cpu_model"])),
		MemoryGb:     nullFloat64Of(floatOrNil(resultData["memory_total_gb"])),
		DiskTotalGb:  diskTotalGB,
		Architecture: nullableStrField(stringOrEmpty(resultData["arch"])),
		CollectedAt:  sql.NullTime{Time: now, Valid: true},
	}); err != nil {
		return false, err
	}

	// Disk table has no unique key per device: rebuild fully to drop unmounted/removed partitions.
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	txQueries := db.New(tx)
	if err = txQueries.DeleteHostDisks(ctx, hostID); err != nil {
		tx.Rollback()
		return false, err
	}
	for _, disk := range disks {
		device, _ := disk["device"].(string)
		if device == "" {
			continue
		}
		mountPoint, filesystem := sql.NullString{}, sql.NullString{}
		if v, ok := disk["mount_point"].(string); ok && v != "" {
			mountPoint = sql.NullString{String: v, Valid: true}
		}
		if v, ok := disk["filesystem"].(string); ok && v != "" {
			filesystem = sql.NullString{String: v, Valid: true}
		}
		sizeGB, usedGB := sql.NullFloat64{}, sql.NullFloat64{}
		if v, ok := disk["size_gb"].(float64); ok {
			sizeGB = sql.NullFloat64{Float64: v, Valid: true}
		}
		if v, ok := disk["used_gb"].(float64); ok {
			usedGB = sql.NullFloat64{Float64: v, Valid: true}
		}
		if err = txQueries.CreateHostDisk(ctx, db.CreateHostDiskParams{
			HostID: hostID, Device: device, MountPoint: mountPoint, SizeGb: sizeGB,
			UsedGb: usedGB, Filesystem: filesystem,
		}); err != nil {
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

// nullableStrField 把"空串即 NULL"的字段钉成 sqlc 的 sql.NullString
// （与 nullableStr 同语义，只是返回具体类型给参数结构体用）。
func nullableStrField(value string) sql.NullString {
	if value == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: value, Valid: true}
}

// nullInt32Of / nullInt64Of / nullFloat64Of / nullTimeOf 把 resultData 里的 any 形态值
// （agent 上报的 JSON）转成 sqlc 需要的具体可空类型，nil 一律落 NULL。
func nullInt32Of(value any) sql.NullInt32 {
	switch typed := value.(type) {
	case int64:
		return sql.NullInt32{Int32: int32(typed), Valid: true}
	case int:
		return sql.NullInt32{Int32: int32(typed), Valid: true}
	case float64:
		return sql.NullInt32{Int32: int32(typed), Valid: true}
	default:
		return sql.NullInt32{}
	}
}

func nullInt64Of(value any) sql.NullInt64 {
	switch typed := value.(type) {
	case int64:
		return sql.NullInt64{Int64: typed, Valid: true}
	case int:
		return sql.NullInt64{Int64: int64(typed), Valid: true}
	case float64:
		return sql.NullInt64{Int64: int64(typed), Valid: true}
	default:
		return sql.NullInt64{}
	}
}

func nullFloat64Of(value any) sql.NullFloat64 {
	if typed, ok := value.(float64); ok {
		return sql.NullFloat64{Float64: typed, Valid: true}
	}
	return sql.NullFloat64{}
}

func nullTimeOf(value any) sql.NullTime {
	if typed, ok := value.(time.Time); ok {
		return sql.NullTime{Time: typed, Valid: true}
	}
	return sql.NullTime{}
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
