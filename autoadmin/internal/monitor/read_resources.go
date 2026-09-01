package monitor

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

func (handler *Handler) GetTarget(context *gin.Context) {
	rows, err := handler.db.QueryContext(context, `SELECT t.*, 'exporter' AS target_type, h.instance_name AS host_name, h.ip AS host_ip, h.agent_id FROM monitor_target t JOIN assets_host h ON h.id=t.host_id WHERE t.id=?`, context.Param("id"))
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil || len(items) == 0 {
		if err != nil {
			response.Error(context, err)
		} else {
			response.BusinessError(context, 404, "monitor target not found", nil)
		}
		return
	}
	items[0]["host_agent_online"] = handler.gateway != nil && handler.gateway.IsOnline(stringValue(items[0]["agent_id"]))
	delete(items[0], "agent_id")
	response.Success(context, items[0])
}

func (handler *Handler) ExporterOptions(context *gin.Context) {
	rows, err := handler.db.QueryContext(context, `SELECT name,MIN(default_port) AS default_port FROM monitor_software_package WHERE package_type='exporter' AND enabled=TRUE GROUP BY name ORDER BY name`)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, items)
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
		targetQuery := `SELECT id,exporter_type,scrape_port,managed_enabled,install_status,install_message,last_scrape_status FROM monitor_target WHERE host_id=?`
		targetArguments := []any{hostID}
		if exporterType != "" {
			targetQuery += " AND exporter_type=?"
			targetArguments = append(targetArguments, exporterType)
		}
		targetQuery += " ORDER BY exporter_type"
		targetRows, queryErr := handler.db.QueryContext(context, targetQuery, targetArguments...)
		if queryErr != nil {
			response.Error(context, queryErr)
			return
		}
		exporters, queryErr := scanRows(targetRows)
		if queryErr != nil {
			response.Error(context, queryErr)
			return
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
		item := gin.H{"host_id": hostID, "host_name": hostName.String, "host_ip": hostIP.String, "group_id": groupValue, "group_name": groupName.String, "host_agent_online": online, "managed": len(exporters) > 0, "exporters": exporters, "fluent_bit": fluentBit}
		if exporterType != "" && len(exporters) > 0 {
			for key, value := range exporters[0] {
				item[key] = value
			}
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

func (handler *Handler) ListInstallHistories(context *gin.Context) {
	handler.installHistories(context, 0)
}
func (handler *Handler) GetInstallHistory(context *gin.Context) {
	handler.installHistories(context, parseID(context.Param("id")))
}

func (handler *Handler) installHistories(context *gin.Context, id int64) {
	page, size := pagination(context)
	clauses, arguments := []string{"1=1"}, make([]any, 0)
	if id > 0 {
		clauses = append(clauses, "ih.id=?")
		arguments = append(arguments, id)
	}
	for queryName, column := range map[string]string{"target_id": "ih.target_id", "log_collection_target_id": "ih.log_collection_target_id", "action": "ih.action", "trigger_type": "ih.trigger_type", "status": "ih.status"} {
		if value := strings.TrimSpace(context.Query(queryName)); value != "" {
			clauses = append(clauses, column+"=?")
			arguments = append(arguments, value)
		}
	}
	if keyword := strings.TrimSpace(context.Query("keyword")); keyword != "" {
		pattern := "%" + keyword + "%"
		clauses = append(clauses, "(ih.host_name_snapshot LIKE ? OR ih.host_ip_snapshot LIKE ? OR ih.exporter_type_snapshot LIKE ? OR ih.summary_message LIKE ?)")
		arguments = append(arguments, pattern, pattern, pattern, pattern)
	}
	if value := strings.TrimSpace(context.Query("start_time")); value != "" {
		clauses = append(clauses, "ih.create_time>=?")
		arguments = append(arguments, value)
	}
	if value := strings.TrimSpace(context.Query("end_time")); value != "" {
		clauses = append(clauses, "ih.create_time<=?")
		arguments = append(arguments, value)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	base := ` FROM monitor_target_install_history ih LEFT JOIN assets_host h ON h.id=ih.host_id LEFT JOIN monitor_target mt ON mt.id=ih.target_id`
	count, err := queryCount(context, handler.db, "SELECT COUNT(*)"+base+where, arguments)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT ih.*,COALESCE(h.instance_name,ih.host_name_snapshot) AS host_name,COALESCE(h.ip,ih.host_ip_snapshot) AS host_ip,COALESCE(mt.exporter_type,ih.exporter_type_snapshot) AS target_exporter_type,COALESCE(ih.target_id,ih.log_collection_target_id) AS managed_target_id,CASE WHEN ih.log_collection_target_id IS NULL THEN 'exporter' ELSE 'fluent_bit' END AS target_type`+base+where+` ORDER BY ih.id DESC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	if id > 0 {
		if len(items) == 0 {
			response.BusinessError(context, 404, "install history not found", nil)
			return
		}
		response.Success(context, items[0])
		return
	}
	paginated(context, items, count, page, size)
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

func (handler *Handler) ListAlertHistories(context *gin.Context) { handler.alertHistories(context, 0) }
func (handler *Handler) GetAlertHistory(context *gin.Context) {
	handler.alertHistories(context, parseID(context.Param("id")))
}

func (handler *Handler) alertHistories(context *gin.Context, id int64) {
	page, size := pagination(context)
	clauses, arguments := []string{"1=1"}, make([]any, 0)
	if id > 0 {
		clauses = append(clauses, "ah.id=?")
		arguments = append(arguments, id)
	}
	for queryName, column := range map[string]string{"state": "ah.state", "severity": "ah.severity"} {
		if value := strings.TrimSpace(context.Query(queryName)); value != "" {
			clauses = append(clauses, column+"=?")
			arguments = append(arguments, value)
		}
	}
	if keyword := strings.TrimSpace(context.Query("keyword")); keyword != "" {
		pattern := "%" + keyword + "%"
		clauses = append(clauses, "(ah.alertname LIKE ? OR ah.instance LIKE ?)")
		arguments = append(arguments, pattern, pattern)
	}
	if value := strings.TrimSpace(context.Query("start_time")); value != "" {
		clauses = append(clauses, "ah.started_at>=?")
		arguments = append(arguments, value)
	}
	if value := strings.TrimSpace(context.Query("end_time")); value != "" {
		clauses = append(clauses, "ah.started_at<=?")
		arguments = append(arguments, value)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	count, err := queryCount(context, handler.db, `SELECT COUNT(*) FROM monitor_alert_history ah`+where, arguments)
	if err != nil {
		response.Error(context, err)
		return
	}
	queryArguments := append(append([]any{}, arguments...), size, (page-1)*size)
	rows, err := handler.db.QueryContext(context, `SELECT ah.*,(SELECT COUNT(*) FROM monitor_alert_notification_event ne WHERE ne.alert_id=ah.id) AS notification_count,(SELECT COUNT(*) FROM monitor_alert_notification_delivery nd JOIN monitor_alert_notification_event ne ON ne.id=nd.event_id WHERE ne.alert_id=ah.id) AS notification_delivery_count,(SELECT COUNT(*) FROM monitor_alert_notification_event ne WHERE ne.alert_id=ah.id AND ne.status='failed') AS notification_failed_count,(SELECT COUNT(*) FROM monitor_alert_notification_event ne WHERE ne.alert_id=ah.id AND ne.status IN ('pending','sending')) AS notification_active_count FROM monitor_alert_history ah`+where+` ORDER BY ah.started_at DESC,ah.id DESC LIMIT ? OFFSET ?`, queryArguments...)
	if err != nil {
		response.Error(context, err)
		return
	}
	items, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, item := range items {
		item["rule_details"] = item["rule_snapshot"]
		item["notification_status"] = notificationStatus(item)
		delete(item, "notification_failed_count")
		delete(item, "notification_active_count")
	}
	if id > 0 {
		if len(items) == 0 {
			response.BusinessError(context, 404, "alert history not found", nil)
			return
		}
		response.Success(context, items[0])
		return
	}
	paginated(context, items, count, page, size)
}

func notificationStatus(item gin.H) string {
	count := intValue(item["notification_count"])
	if count == 0 {
		return "none"
	}
	failed, active, delivery := intValue(item["notification_failed_count"]), intValue(item["notification_active_count"]), intValue(item["notification_delivery_count"])
	if failed > 0 || (active == 0 && delivery == 0) {
		return "failed"
	}
	if active > 0 {
		return "in_progress"
	}
	return "success"
}

func (handler *Handler) AlertNotificationStatus(context *gin.Context) {
	id := parseID(context.Param("id"))
	var alertname, instance string
	if err := handler.db.QueryRowContext(context, `SELECT alertname,instance FROM monitor_alert_history WHERE id=?`, id).Scan(&alertname, &instance); err != nil {
		response.Error(context, err)
		return
	}
	rows, err := handler.db.QueryContext(context, `SELECT * FROM monitor_alert_notification_event WHERE alert_id=? ORDER BY create_time DESC,id DESC`, id)
	if err != nil {
		response.Error(context, err)
		return
	}
	events, err := scanRows(rows)
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, event := range events {
		deliveryRows, queryErr := handler.db.QueryContext(context, `SELECT d.id,d.user_id,COALESCE(u.username,'-') AS username,d.media_id,COALESCE(m.name,'-') AS media_name,COALESCE(m.media_type,'-') AS media_type,d.address,d.status,d.attempt_count,d.error_message,d.sent_at,d.create_time FROM monitor_alert_notification_delivery d LEFT JOIN sys_user u ON u.id=d.user_id LEFT JOIN monitor_alert_media m ON m.id=d.media_id WHERE d.event_id=? ORDER BY d.id`, event["id"])
		if queryErr != nil {
			response.Error(context, queryErr)
			return
		}
		deliveries, queryErr := scanRows(deliveryRows)
		if queryErr != nil {
			response.Error(context, queryErr)
			return
		}
		event["deliveries"] = deliveries
		if len(deliveries) == 0 && stringValue(event["status"]) == "success" {
			event["status"] = "failed"
			event["error_message"] = "没有投递明细，无法确认实际接收用户、媒介和地址"
		}
	}
	response.Success(context, gin.H{"alert_id": id, "alertname": alertname, "instance": instance, "events": events})
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
