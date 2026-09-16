package monitor

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

func queryCount(context *gin.Context, database *sql.DB, query string, arguments []any) (int64, error) {
	var count int64
	err := database.QueryRowContext(context, query, arguments...).Scan(&count)
	return count, err
}

func paginated(context *gin.Context, items []gin.H, count int64, page, size int) {
	response.Paginated(context, items, count, int32(page), int32(size))
}

func (handler *Handler) GetTarget(context *gin.Context) {
	row, err := db.New(handler.db).GetMonitorTarget(context, parseID(context.Param("id")))
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "monitor target not found", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	item := monitorTargetResponseFrom(db.ListMonitorTargetsRow{
		ID: row.ID, CreateTime: row.CreateTime, UpdateTime: row.UpdateTime, Remark: row.Remark,
		ExporterType: row.ExporterType, ManagedEnabled: row.ManagedEnabled,
		InstallStatus: row.InstallStatus, InstallMessage: row.InstallMessage,
		LastScrapeStatus: row.LastScrapeStatus, LastScrapeAt: row.LastScrapeAt,
		Labels: row.Labels, HostID: row.HostID, RetryCount: row.RetryCount,
		LastDispatchManual: row.LastDispatchManual, ScrapePort: row.ScrapePort,
		TargetType: row.TargetType, HostName: row.HostName, HostIp: row.HostIp,
		HostAgentOnline: row.HostAgentOnline,
	})
	item.HostAgentOnline = handler.gateway != nil && handler.gateway.IsOnline(row.HostName.String)
	response.Success(context, item)
}

func (handler *Handler) ExporterOptions(context *gin.Context) {
	rows, err := db.New(handler.db).ListExporterPackagePorts(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	type exporterOption struct {
		Name        string `json:"name"`
		DefaultPort uint32 `json:"default_port"`
	}
	options := make([]exporterOption, 0)
	positions := make(map[string]int)
	for _, row := range rows {
		if position, ok := positions[row.Name]; ok {
			if row.DefaultPort < options[position].DefaultPort {
				options[position].DefaultPort = row.DefaultPort
			}
			continue
		}
		positions[row.Name] = len(options)
		options = append(options, exporterOption{Name: row.Name, DefaultPort: row.DefaultPort})
	}
	response.Success(context, options)
}

type monitorGroup struct {
	ID           int64
	Name         string
	ParentID     sql.NullInt64
	HostCount    int64
	ManagedCount int64
}

func (handler *Handler) HostGroupTree(context *gin.Context) {
	queries := db.New(handler.db)
	rows, err := queries.ListMonitorHostGroupTree(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	groups := make([]monitorGroup, 0, len(rows))
	for _, row := range rows {
		groups = append(groups, monitorGroup{ID: row.ID, Name: row.Name, ParentID: row.ParentID,
			HostCount: row.HostCount, ManagedCount: row.ManagedCount})
	}
	totals, err := queries.CountMonitorHostTotals(context)
	if err != nil {
		response.Error(context, err)
		return
	}
	totalHosts, totalManaged, ungrouped := totals.Count, totals.ManagedTotal, totals.Ungrouped
	children := make(map[int64][]monitorGroup)
	for _, item := range groups {
		parentID := int64(0)
		if item.ParentID.Valid {
			parentID = item.ParentID.Int64
		}
		children[parentID] = append(children[parentID], item)
	}
	var build func(int64, map[int64]bool) []gin.H
	build = func(parentID int64, ancestors map[int64]bool) []gin.H {
		result := make([]gin.H, 0, len(children[parentID]))
		for _, item := range children[parentID] {
			if ancestors[item.ID] {
				continue
			}
			next := make(map[int64]bool, len(ancestors)+1)
			for id := range ancestors {
				next[id] = true
			}
			next[item.ID] = true
			var parent any
			if item.ParentID.Valid {
				parent = item.ParentID.Int64
			}
			result = append(result, gin.H{"id": item.ID, "name": item.Name, "parent_id": parent, "host_count": item.HostCount, "managed_count": item.ManagedCount, "children": build(item.ID, next)})
		}
		return result
	}
	response.Success(context, gin.H{"groups": build(0, map[int64]bool{}), "total_host_count": totalHosts, "total_managed_count": totalManaged, "ungrouped_host_count": ungrouped})
}

func (handler *Handler) HostOverview(context *gin.Context) {
	page, size := pagination(context)
	// 过滤条件全部走参数（原来的四种组合是运行时拼 WHERE）：
	// 搜索空串 / 分组为空 / 纳管状态与日志采集状态为 NULL 都表示"不过滤"。
	// GroupFilter 是"空串表示不过滤"的开关：必须显式传空串而不是 nil，
	// 否则 SQL 里 `$n = ''` 变成 `NULL = ''`（NULL 不是 true），整条过滤会把所有主机排除。
	filter := db.CountMonitorHostsParams{
		ExporterType: strings.TrimSpace(context.Query("exporter_type")),
		GroupFilter:  "",
	}
	// 搜索同样是"空串表示不过滤"：必须显式传空串（传 NULL 会让 `? = ''` 求值为 NULL，
	// 整条 AND 变 NULL，所有主机都被排除）。
	pattern := strings.TrimSpace(context.Query("search"))
	if pattern != "" {
		pattern = "%" + pattern + "%"
	}
	filter.SearchPattern = sql.NullString{String: pattern, Valid: true}
	if groupID := strings.TrimSpace(context.Query("group_id")); groupID != "" {
		groupIDs, err := handler.descendantGroupIDs(context, groupID)
		if err != nil {
			response.Error(context, err)
			return
		}
		if len(groupIDs) > 0 {
			filter.GroupFilter = groupID
			filter.GroupIds = make([]sql.NullInt64, 0, len(groupIDs))
			for _, id := range groupIDs {
				filter.GroupIds = append(filter.GroupIds, sql.NullInt64{Int64: id, Valid: true})
			}
		}
	}
	managedFilter := strings.TrimSpace(context.Query("exporter_managed"))
	if managedFilter == "" {
		managedFilter = strings.TrimSpace(context.Query("managed"))
	}
	if managedFilter == "true" || managedFilter == "false" {
		filter.ManagedFilter = managedFilter
	}
	if fluentManaged := strings.TrimSpace(context.Query("fluent_bit_managed")); fluentManaged == "true" || fluentManaged == "false" {
		filter.FluentFilter = fluentManaged
	}
	queries := db.New(handler.db)
	count, err := queries.CountMonitorHosts(context, filter)
	if err != nil {
		response.Error(context, err)
		return
	}
	rows, err := queries.ListMonitorHosts(context, db.ListMonitorHostsParams{
		SearchPattern: filter.SearchPattern, GroupIds: filter.GroupIds, GroupFilter: filter.GroupFilter,
		ManagedFilter: filter.ManagedFilter, ExporterType: filter.ExporterType, FluentFilter: filter.FluentFilter,
		Limit: int32(size), Offset: int32((page - 1) * size),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	results := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		hostID := row.ID
		hostName, hostIP, groupName := row.InstanceName, row.Ip, row.GroupName
		groupID, logTargetID := row.GroupID, row.LogTargetID
		agentInstalled := row.AgentInstalled
		agentVersion, runtimeStatus, installStatus := row.AgentVersion, row.RuntimeStatus, row.InstallStatus
		fingerprint, lastError, lastApplied := row.ConfigFingerprint, row.LastError, row.LastAppliedTime
		exporters, queryErr := queries.ListMonitorTargetsByHost(context, db.ListMonitorTargetsByHostParams{
			HostID:       hostID,
			ExporterType: optionalStringParam(context, "exporter_type"),
		})
		if queryErr != nil {
			response.Error(context, queryErr)
			return
		}
		typedExporters := make([]exporterTargetResponse, 0, len(exporters))
		for _, target := range exporters {
			typedExporters = append(typedExporters, exporterTargetResponseFrom(target))
		}
		online := handler.gateway != nil && handler.gateway.IsOnline(hostName.String)
		var groupValue, logIDValue, appliedValue any
		if groupID.Valid {
			groupValue = groupID.Int64
		}
		if logTargetID.Valid {
			logIDValue = logTargetID.Int64
		}
		if lastApplied.Valid {
			appliedValue = lastApplied.Time
		}
		fluentBit := gin.H{"id": logIDValue, "host_id": hostID, "host_name": hostName.String, "host_ip": hostIP.String, "host_agent_online": online, "managed": logTargetID.Valid, "agent_installed": agentInstalled.Valid && agentInstalled.Bool, "agent_version": agentVersion.String, "runtime_status": runtimeStatus.String, "install_status": installStatus.String, "config_fingerprint": fingerprint.String, "last_applied_time": appliedValue, "last_error": lastError.String}
		item := gin.H{"host_id": hostID, "host_name": hostName.String, "host_ip": hostIP.String, "group_id": groupValue, "group_name": groupName, "host_agent_online": online, "managed": len(typedExporters) > 0, "exporters": typedExporters, "fluent_bit": fluentBit}
		if filter.ExporterType != "" && len(typedExporters) > 0 {
			first := typedExporters[0]
			item["id"] = first.ID
			item["exporter_type"] = first.ExporterType
			item["scrape_port"] = first.ScrapePort
			item["managed_enabled"] = first.ManagedEnabled
			item["install_status"] = first.InstallStatus
			item["install_message"] = first.InstallMessage
			item["last_scrape_status"] = first.LastScrapeStatus
		}
		results = append(results, item)
	}
	paginated(context, results, count, page, size)
}

func (handler *Handler) descendantGroupIDs(context *gin.Context, root string) ([]int64, error) {
	rootID, err := strconv.ParseInt(root, 10, 64)
	if err != nil || rootID <= 0 {
		return nil, nil
	}
	rows, err := db.New(handler.db).ListMonitorHostGroupParents(context)
	if err != nil {
		return nil, err
	}
	children := make(map[int64][]int64)
	for _, row := range rows {
		id, parent := row.ID, row.ParentID
		if parent.Valid {
			children[parent.Int64] = append(children[parent.Int64], id)
		}
	}
	result, pending, seen := make([]int64, 0), []int64{rootID}, map[int64]bool{}
	for len(pending) > 0 {
		id := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[id] {
			continue
		}
		seen[id] = true
		result = append(result, id)
		pending = append(pending, children[id]...)
	}
	return result, nil
}

func (handler *Handler) CancelInstallHistory(context *gin.Context) {
	id := parseID(context.Param("id"))
	if id == 0 {
		response.BusinessError(context, 400, "invalid id", nil)
		return
	}
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	txQueries := db.New(tx)
	history, err := txQueries.GetInstallHistoryForUpdate(context, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	status, targetID, logTargetID, start := history.Status, history.TargetID, history.LogCollectionTargetID, history.StartTime
	if status != "pending" && status != "running" {
		response.BusinessError(context, 400, "current task has ended and cannot be cancelled", nil)
		return
	}
	now := time.Now().UTC()
	duration := sql.NullFloat64{}
	if start.Valid {
		duration = sql.NullFloat64{Float64: now.Sub(start.Time).Seconds(), Valid: true}
	}
	if err = txQueries.CancelInstallHistory(context, db.CancelInstallHistoryParams{
		EndTime: sql.NullTime{Time: now, Valid: true}, DurationSeconds: duration,
		UpdateTime: now, ID: id,
	}); err != nil {
		response.Error(context, err)
		return
	}
	// 取消的是监控目标还是日志采集目标，决定更新哪张表（原实现拼表名，这里按类型分派）。
	switch {
	case targetID.Valid:
		err = txQueries.CancelMonitorTargetInstallState(context, db.CancelMonitorTargetInstallStateParams{
			UpdateTime: now, ID: targetID.Int64,
		})
	case logTargetID.Valid:
		err = txQueries.CancelLogTargetInstallState(context, db.CancelLogTargetInstallStateParams{
			UpdateTime: now, ID: logTargetID.Int64,
		})
	default:
		response.BusinessError(context, 400, "task has no managed target", nil)
		return
	}
	if err != nil {
		response.Error(context, err)
		return
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	handler.installHistories(context, id)
}

func parseID(value string) int64 {
	id, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return id
}
func intValue(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		result, _ := strconv.ParseInt(stringValue(value), 10, 64)
		return result
	}
}
