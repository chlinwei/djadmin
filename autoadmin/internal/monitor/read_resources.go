package monitor

import (
	"database/sql"
	"fmt"
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
	item.HostAgentOnline = handler.gateway != nil && handler.gateway.IsOnline(row.AgentID.String)
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
	rows, err := handler.db.QueryContext(context, `SELECT g.id,g.name,g.parent_id,COUNT(h.id) AS host_count,COALESCE(SUM(CASE WHEN h.id IS NOT NULL AND (EXISTS(SELECT 1 FROM monitor_target t WHERE t.host_id=h.id) OR EXISTS(SELECT 1 FROM monitor_log_collection_target l WHERE l.host_id=h.id)) THEN 1 ELSE 0 END),0) AS managed_count FROM assets_hostgroup g LEFT JOIN assets_host h ON h.group_id=g.id AND h.is_deleted_in_cloud=FALSE GROUP BY g.id,g.name,g.parent_id ORDER BY g.name,g.id`)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	groups := make([]monitorGroup, 0)
	for rows.Next() {
		var item monitorGroup
		if err = rows.Scan(&item.ID, &item.Name, &item.ParentID, &item.HostCount, &item.ManagedCount); err != nil {
			response.Error(context, err)
			return
		}
		groups = append(groups, item)
	}
	if err = rows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	var totalHosts, totalManaged, ungrouped int64
	err = handler.db.QueryRowContext(context, `SELECT COUNT(*),COALESCE(SUM(EXISTS(SELECT 1 FROM monitor_target t WHERE t.host_id=h.id) OR EXISTS(SELECT 1 FROM monitor_log_collection_target l WHERE l.host_id=h.id)),0),COALESCE(SUM(h.group_id IS NULL),0) FROM assets_host h WHERE h.is_deleted_in_cloud=FALSE`).Scan(&totalHosts, &totalManaged, &ungrouped)
	if err != nil {
		response.Error(context, err)
		return
	}
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
	clauses := []string{"h.is_deleted_in_cloud=FALSE"}
	arguments := make([]any, 0)
	if search := strings.TrimSpace(context.Query("search")); search != "" {
		clauses = append(clauses, "(h.instance_name LIKE ? OR h.ip LIKE ?)")
		arguments = append(arguments, "%"+search+"%", "%"+search+"%")
	}
	if groupID := strings.TrimSpace(context.Query("group_id")); groupID != "" {
		groupIDs, err := handler.descendantGroupIDs(context, groupID)
		if err != nil {
			response.Error(context, err)
			return
		}
		if len(groupIDs) > 0 {
			placeholders := make([]string, len(groupIDs))
			for index, id := range groupIDs {
				placeholders[index] = "?"
				arguments = append(arguments, id)
			}
			clauses = append(clauses, "h.group_id IN ("+strings.Join(placeholders, ",")+")")
		}
	}
	exporterType := strings.TrimSpace(context.Query("exporter_type"))
	managedFilter := strings.TrimSpace(context.Query("exporter_managed"))
	if managedFilter == "" {
		managedFilter = strings.TrimSpace(context.Query("managed"))
	}
	if managedFilter == "true" || managedFilter == "false" {
		exists := "EXISTS(SELECT 1 FROM monitor_target mt WHERE mt.host_id=h.id"
		if exporterType != "" {
			exists += " AND mt.exporter_type=?"
			arguments = append(arguments, exporterType)
		}
		exists += ")"
		if managedFilter == "false" {
			exists = "NOT " + exists
		}
		clauses = append(clauses, exists)
	} else if exporterType != "" {
		// 单独选中某个 Exporter、不搭配"已纳管/未纳管"时，也要把主机列表收窄到"纳管过这个 exporter"，
		// 否则下拉只是摆设——选了也不过滤，用户看到的还是全部主机。
		clauses = append(clauses, "EXISTS(SELECT 1 FROM monitor_target mt WHERE mt.host_id=h.id AND mt.exporter_type=?)")
		arguments = append(arguments, exporterType)
	}
	if fluentManaged := strings.TrimSpace(context.Query("fluent_bit_managed")); fluentManaged == "true" {
		clauses = append(clauses, "EXISTS(SELECT 1 FROM monitor_log_collection_target lc WHERE lc.host_id=h.id AND lc.agent_installed=TRUE)")
	} else if fluentManaged == "false" {
		clauses = append(clauses, "NOT EXISTS(SELECT 1 FROM monitor_log_collection_target lc WHERE lc.host_id=h.id AND lc.agent_installed=TRUE)")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM assets_host h`+where, arguments)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT h.id,h.instance_name,h.ip,h.group_id,COALESCE(g.name,''),h.agent_id,lc.id,lc.agent_installed,lc.agent_version,lc.runtime_status,lc.install_status,lc.config_fingerprint,lc.last_applied_time,lc.last_error FROM assets_host h LEFT JOIN assets_hostgroup g ON g.id=h.group_id LEFT JOIN monitor_log_collection_target lc ON lc.host_id=h.id`+where+` ORDER BY h.instance_name,h.id LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	results := make([]gin.H, 0)
	for rows.Next() {
		var hostID int64
		var hostName, hostIP, groupName, agentID sql.NullString
		var groupID, logTargetID sql.NullInt64
		var agentInstalled sql.NullBool
		var agentVersion, runtimeStatus, installStatus, fingerprint, lastError sql.NullString
		var lastApplied sql.NullTime
		if err = rows.Scan(&hostID, &hostName, &hostIP, &groupID, &groupName, &agentID, &logTargetID, &agentInstalled, &agentVersion, &runtimeStatus, &installStatus, &fingerprint, &lastApplied, &lastError); err != nil {
			response.Error(context, err)
			return
		}
		exporters, queryErr := db.New(handler.db).ListMonitorTargetsByHost(context, db.ListMonitorTargetsByHostParams{
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
		online := handler.gateway != nil && handler.gateway.IsOnline(agentID.String)
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
		item := gin.H{"host_id": hostID, "host_name": hostName.String, "host_ip": hostIP.String, "group_id": groupValue, "group_name": groupName.String, "host_agent_online": online, "managed": len(typedExporters) > 0, "exporters": typedExporters, "fluent_bit": fluentBit}
		if exporterType != "" && len(typedExporters) > 0 {
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
	if err = rows.Err(); err != nil {
		response.Error(context, err)
		return
	}
	paginated(context, results, count, page, size)
}

func (handler *Handler) descendantGroupIDs(context *gin.Context, root string) ([]int64, error) {
	rootID, err := strconv.ParseInt(root, 10, 64)
	if err != nil || rootID <= 0 {
		return nil, nil
	}
	rows, err := handler.db.QueryContext(context, `SELECT id,parent_id FROM assets_hostgroup`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	children := make(map[int64][]int64)
	for rows.Next() {
		var id int64
		var parent sql.NullInt64
		if err = rows.Scan(&id, &parent); err != nil {
			return nil, err
		}
		if parent.Valid {
			children[parent.Int64] = append(children[parent.Int64], id)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, err
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
	var status string
	var targetID, logTargetID sql.NullInt64
	var start sql.NullTime
	if err = tx.QueryRowContext(context, `SELECT status,target_id,log_collection_target_id,start_time FROM monitor_target_install_history WHERE id=? FOR UPDATE`, id).Scan(&status, &targetID, &logTargetID, &start); err != nil {
		response.Error(context, err)
		return
	}
	if status != "pending" && status != "running" {
		response.BusinessError(context, 400, "current task has ended and cannot be cancelled", nil)
		return
	}
	now := time.Now().UTC()
	var duration any
	if start.Valid {
		duration = now.Sub(start.Time).Seconds()
	}
	if _, err = tx.ExecContext(context, `UPDATE monitor_target_install_history SET status='cancelled',summary_message='任务已取消',error_message_snapshot='任务已由用户取消',end_time=?,duration_seconds=?,update_time=? WHERE id=?`, now, duration, now, id); err != nil {
		response.Error(context, err)
		return
	}
	table, target := `monitor_target`, targetID
	if logTargetID.Valid {
		table, target = `monitor_log_collection_target`, logTargetID
	}
	if !target.Valid {
		response.BusinessError(context, 400, "task has no managed target", nil)
		return
	}
	if _, err = tx.ExecContext(context, fmt.Sprintf(`UPDATE %s SET install_status='unknown',install_message='安装/卸载任务已取消',update_time=? WHERE id=?`, table), now, target.Int64); err != nil {
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
