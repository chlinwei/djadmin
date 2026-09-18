package monitor

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"
	"autoadmin/internal/logcollect"
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

// paginatedWith 在分页信封上追加页面级字段。
func paginatedWith(context *gin.Context, items []gin.H, count int64, page, size int, extra gin.H) {
	response.PaginatedWith(context, items, count, int32(page), int32(size), extra)
}

// logConfigStateUnknown 配置态无法计算时的占位（缺省未注入评估器、或没有启用的默认 ES 集群）。
const logConfigStateUnknown = "unknown"

// validConfigStateFilter 校验配置态筛选值。取值不合法直接 400，避免前端写错时静默返回全量。
func validConfigStateFilter(value string) bool {
	switch value {
	case logcollect.LogConfigSynced, logcollect.LogConfigDrift, logcollect.LogConfigNever, logConfigStateUnknown:
		return true
	}
	return false
}

// filterRowsByConfigState 按配置态保留主机。未纳管的主机没有日志目标，任何配置态筛选都不应命中它
// （否则"待下发"里会混进根本没纳管 Filebeat 的机器，与"从未下发"语义冲突）。
func filterRowsByConfigState(rows []db.ListMonitorHostsRow, states map[int64]logcollect.LogConfigState, want string) []db.ListMonitorHostsRow {
	filtered := make([]db.ListMonitorHostsRow, 0, len(rows))
	for _, row := range rows {
		if !row.LogTargetID.Valid {
			continue
		}
		state := states[row.ID]
		status := logConfigStateUnknown
		if state.HostID != 0 {
			status = state.Status
		}
		if status == want {
			filtered = append(filtered, row)
		}
	}
	return filtered
}

// pageRows 对已过滤的行做内存分页（筛选路径下 SQL 无法按状态分页）。
func pageRows(rows []db.ListMonitorHostsRow, page, size int) []db.ListMonitorHostsRow {
	start := (page - 1) * size
	if start >= len(rows) {
		return []db.ListMonitorHostsRow{}
	}
	end := start + size
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

// errorString 取错误文案（nil 时为空串），用于把"为何算不出配置态"带给前端。
func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
	if filebeatManaged := strings.TrimSpace(context.Query("filebeat_managed")); filebeatManaged == "true" || filebeatManaged == "false" {
		filter.FilebeatFilter = filebeatManaged
	}
	queries := db.New(handler.db)

	// 配置态筛选：状态是后端实时渲染算出来的，SQL 没法按它过滤。启用筛选时先把**全部**
	// 匹配主机取回（不按页取），批量评估后过滤，再在内存里分页——见计划 §2.4 的算力约束：
	// 筛选/统计场景允许一次全量渲染，展示场景只算当前页。
	stateFilter := strings.TrimSpace(context.Query("config_state"))
	if stateFilter != "" && !validConfigStateFilter(stateFilter) {
		response.BusinessError(context, 400, "config_state must be synced|drift|never|unknown", nil)
		return
	}

	count, err := queries.CountMonitorHosts(context, filter)
	if err != nil {
		response.Error(context, err)
		return
	}
	listLimit, listOffset := int32(size), int32((page-1)*size)
	if stateFilter != "" {
		listLimit, listOffset = int32(count), 0
	}
	rows := []db.ListMonitorHostsRow{}
	if listLimit > 0 {
		rows, err = queries.ListMonitorHosts(context, db.ListMonitorHostsParams{
			SearchPattern: filter.SearchPattern, GroupIds: filter.GroupIds, GroupFilter: filter.GroupFilter,
			ManagedFilter: filter.ManagedFilter, ExporterType: filter.ExporterType, FilebeatFilter: filter.FilebeatFilter,
			Limit: listLimit, Offset: listOffset,
		})
		if err != nil {
			response.Error(context, err)
			return
		}
	}

	// 一次评估覆盖本次取回的全部主机（本页或全量），不逐台查库/渲染。
	stateRefs := make([]logcollect.LogConfigTargetRef, 0, len(rows))
	for _, row := range rows {
		if row.LogTargetID.Valid {
			stateRefs = append(stateRefs, logcollect.LogConfigTargetRef{
				HostID: row.ID, AppliedFingerprint: row.ConfigFingerprint.String,
			})
		}
	}
	configStates, configStateErr := handler.evaluateConfigStates(context, stateRefs)

	if stateFilter != "" {
		rows = filterRowsByConfigState(rows, configStates, stateFilter)
		count = int64(len(rows))
		rows = pageRows(rows, page, size)
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
		filebeat := gin.H{"id": logIDValue, "host_id": hostID, "host_name": hostName.String, "host_ip": hostIP.String, "host_agent_online": online, "managed": logTargetID.Valid, "agent_installed": agentInstalled.Valid && agentInstalled.Bool, "agent_version": agentVersion.String, "runtime_status": runtimeStatus.String, "install_status": installStatus.String, "config_fingerprint": fingerprint.String, "last_applied_time": appliedValue, "last_error": lastError.String}
		// 配置态只对已纳管的目标有意义：未纳管主机没有日志目标，"从未下发"会误导。
		if logTargetID.Valid {
			state := configStates[hostID]
			status := logConfigStateUnknown
			if state.HostID != 0 {
				status = state.Status
			}
			filebeat["config_state"] = status
			filebeat["expected_fingerprint"] = state.ExpectedFingerprint
			filebeat["config_service_num"] = state.ServiceNum
			if len(state.Warnings) > 0 {
				filebeat["config_warnings"] = state.Warnings
			}
		}
		item := gin.H{"host_id": hostID, "host_name": hostName.String, "host_ip": hostIP.String, "group_id": groupValue, "group_name": groupName, "host_agent_online": online, "managed": len(typedExporters) > 0, "exporters": typedExporters, "filebeat": filebeat}
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
	// 评估失败不能让主机列表整体失败（该列表同时服务 exporter 纳管）：状态显示"未知"并回传原因。
	paginatedWith(context, results, count, page, size, gin.H{"config_state_error": errorString(configStateErr)})
}

// evaluateConfigStates 调用日志采集域注入的配置态评估。未注入（如未接线的测试）或没有入参时
// 返回空结果且不报错，调用方把状态渲染成"未知"。
func (handler *Handler) evaluateConfigStates(context *gin.Context, refs []logcollect.LogConfigTargetRef) (map[int64]logcollect.LogConfigState, error) {
	if handler.evaluateLogConfigStates == nil || len(refs) == 0 {
		return map[int64]logcollect.LogConfigState{}, nil
	}
	return handler.evaluateLogConfigStates(context, refs)
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
