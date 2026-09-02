package monitor

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"

	"github.com/gin-gonic/gin"
)

var serviceNamePattern = regexp.MustCompile(`^[a-zA-Z0-9_.@-]+$`)

func (handler *Handler) UpdateTarget(context *gin.Context) {
	id := parseID(context.Param("id"))
	var input map[string]any
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "invalid request body", nil)
		return
	}
	sets, arguments := make([]string, 0), make([]any, 0)
	for _, field := range []string{"exporter_type", "scrape_port", "managed_enabled", "labels", "remark"} {
		value, exists := input[field]
		if !exists {
			continue
		}
		if field == "scrape_port" {
			port := intValue(value)
			if port < 1 || port > 65535 {
				response.BusinessError(context, 400, "scrape_port must be between 1 and 65535", nil)
				return
			}
		}
		if field == "exporter_type" && !serviceNamePattern.MatchString(strings.TrimSpace(stringValue(value))) {
			response.BusinessError(context, 400, "invalid exporter_type", nil)
			return
		}
		if field == "labels" {
			encoded, err := json.Marshal(value)
			if err != nil {
				response.BusinessError(context, 400, "labels must be valid JSON", nil)
				return
			}
			value = string(encoded)
		}
		sets = append(sets, field+"=?")
		arguments = append(arguments, value)
	}
	if len(sets) == 0 {
		response.BusinessError(context, 400, "no writable fields", nil)
		return
	}
	sets = append(sets, "update_time=?")
	arguments = append(arguments, time.Now().UTC(), id)
	result, err := handler.db.ExecContext(context, `UPDATE monitor_target SET `+strings.Join(sets, ",")+` WHERE id=?`, arguments...)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		response.BusinessError(context, 404, "monitor target not found", nil)
		return
	}
	handler.GetTarget(context)
}

func (handler *Handler) DeleteTarget(context *gin.Context) {
	id := parseID(context.Param("id"))
	var enabled bool
	var status string
	if err := handler.db.QueryRowContext(context, `SELECT managed_enabled,install_status FROM monitor_target WHERE id=?`, id).Scan(&enabled, &status); err != nil {
		response.Error(context, err)
		return
	}
	if enabled {
		response.BusinessError(context, 400, "disable the monitor target before deleting it", nil)
		return
	}
	if status == "pending" {
		response.BusinessError(context, 400, "wait for the uninstall task to finish before deleting", nil)
		return
	}
	if _, err := handler.db.ExecContext(context, `DELETE FROM monitor_target WHERE id=?`, id); err != nil {
		response.Error(context, err)
		return
	}
	response.Success(context, gin.H{"id": id})
}

func (handler *Handler) BatchCreateTargets(context *gin.Context) {
	var input struct {
		HostIDs      []int64 `json:"host_ids"`
		ExporterType string  `json:"exporter_type"`
		ScrapePort   int64   `json:"scrape_port"`
		InstallNow   bool    `json:"install_now"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.HostIDs) == 0 {
		response.BusinessError(context, 400, "host_ids must be a non-empty array", nil)
		return
	}
	input.ExporterType = strings.TrimSpace(input.ExporterType)
	if !serviceNamePattern.MatchString(input.ExporterType) {
		response.BusinessError(context, 400, "invalid exporter_type", nil)
		return
	}
	var defaultPort int64
	if err := handler.db.QueryRowContext(context, `SELECT default_port FROM monitor_software_package WHERE name=? AND package_type='exporter' AND enabled=TRUE ORDER BY id LIMIT 1`, input.ExporterType).Scan(&defaultPort); err != nil {
		response.BusinessError(context, 400, "no enabled exporter package found", nil)
		return
	}
	if input.ScrapePort == 0 {
		input.ScrapePort = defaultPort
	}
	if input.ScrapePort < 1 || input.ScrapePort > 65535 {
		response.BusinessError(context, 400, "scrape_port must be between 1 and 65535", nil)
		return
	}
	results := make([]gin.H, 0, len(input.HostIDs))
	success := 0
	for _, hostID := range input.HostIDs {
		var name, ip string
		var deleted bool
		if err := handler.db.QueryRowContext(context, `SELECT COALESCE(instance_name,''),COALESCE(ip,''),is_deleted_in_cloud FROM assets_host WHERE id=?`, hostID).Scan(&name, &ip, &deleted); err != nil || deleted {
			results = append(results, gin.H{"host_id": hostID, "host": name, "ok": false, "message": "host not found"})
			continue
		}
		label := name
		if label == "" {
			label = ip
		}
		now := time.Now().UTC()
		result, err := handler.db.ExecContext(context, `INSERT IGNORE INTO monitor_target(create_time,update_time,remark,host_id,exporter_type,scrape_port,managed_enabled,install_status,install_message,retry_count,last_scrape_status,labels,last_dispatch_manual) VALUES(?,?,?,?,?,?,TRUE,'unknown','',0,'unknown','{}',FALSE)`, now, now, nil, hostID, input.ExporterType, input.ScrapePort)
		if err != nil {
			results = append(results, gin.H{"host_id": hostID, "host": label, "ok": false, "message": err.Error()})
			continue
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			results = append(results, gin.H{"host_id": hostID, "host": label, "ok": false, "message": "target already managed"})
			continue
		}
		success++
		message := ""
		if input.InstallNow {
			message = "target created; use retry to dispatch installation"
		}
		results = append(results, gin.H{"host_id": hostID, "host": label, "ok": true, "message": message})
	}
	response.Success(context, gin.H{"total": len(results), "success": success, "failed": len(results) - success, "results": results})
}

func (handler *Handler) CancelTarget(context *gin.Context) {
	id := parseID(context.Param("id"))
	var historyID int64
	var status string
	var start sql.NullTime
	err := handler.db.QueryRowContext(context, `SELECT id,status,start_time FROM monitor_target_install_history WHERE target_id=? ORDER BY id DESC LIMIT 1`, id).Scan(&historyID, &status, &start)
	if err != nil || (status != "pending" && status != "running") {
		response.BusinessError(context, 400, "current task has ended and does not need cancellation", nil)
		return
	}
	now := time.Now().UTC()
	var duration any
	if start.Valid {
		duration = now.Sub(start.Time).Seconds()
	}
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(context, `UPDATE monitor_target_install_history SET status='cancelled',summary_message='任务已取消',error_message_snapshot='任务已由用户取消',end_time=?,duration_seconds=?,update_time=? WHERE id=?`, now, duration, now, historyID); err != nil {
		response.Error(context, err)
		return
	}
	if _, err = tx.ExecContext(context, `UPDATE monitor_target SET install_status='failed',install_message='安装/卸载任务已取消',update_time=? WHERE id=?`, now, id); err != nil {
		response.Error(context, err)
		return
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	handler.GetTarget(context)
}

func (handler *Handler) CheckTargetService(context *gin.Context) {
	handler.controlTargetService(context, "status")
}
func (handler *Handler) StartTargetService(context *gin.Context) {
	handler.controlTargetService(context, "start")
}
func (handler *Handler) StopTargetService(context *gin.Context) {
	handler.controlTargetService(context, "stop")
}

func (handler *Handler) controlTargetService(context *gin.Context, action string) {
	id := parseID(context.Param("id"))
	result, err := handler.dispatchTargetServiceControl(context, id, action)
	if err != nil {
		response.BusinessError(context, 400, err.Error(), nil)
		return
	}
	response.Success(context, result)
}

// dispatchTargetServiceControl 是 controlTargetService 与批量接口共用的下发核心，
// 避免批量版本和单台版本的 systemctl 命令拼接逻辑各写一份、后续改一处漏一处。
func (handler *Handler) dispatchTargetServiceControl(context *gin.Context, id int64, action string) (gin.H, error) {
	var agentID, exporterType string
	if err := handler.db.QueryRowContext(context, `SELECT COALESCE(h.agent_id,''),t.exporter_type FROM monitor_target t JOIN assets_host h ON h.id=t.host_id WHERE t.id=?`, id).Scan(&agentID, &exporterType); err != nil {
		return nil, fmt.Errorf("monitor target not found")
	}
	if handler.gateway == nil || !handler.gateway.IsOnline(agentID) {
		return nil, fmt.Errorf("host agent is offline")
	}
	if !serviceNamePattern.MatchString(exporterType) {
		return nil, fmt.Errorf("invalid exporter service name")
	}
	command := fmt.Sprintf("sudo systemctl %s %s.service", action, exporterType)
	params, _ := json.Marshal(gin.H{"command": command})
	result, err := handler.gateway.Execute(context, agentID, &pb.AutomationExecuteRequest{JobId: fmt.Sprintf("monitor-service-%d", time.Now().UnixNano()), Type: "custom", Action: "run_shell", ParamsJson: string(params), TimeoutSeconds: 30})
	if err != nil {
		return nil, err
	}
	return gin.H{"job_id": result.JobId, "status": result.Status, "exit_code": result.ExitCode, "stdout": result.Stdout, "stderr": result.Stderr, "error_message": result.ErrorMessage}, nil
}

// targetHostLabel 取一个 monitor_target 对应的主机展示名，批量接口的结果列表要按主机报告成功/失败。
func targetHostLabel(context *gin.Context, db *sql.DB, id int64) string {
	var name, ip sql.NullString
	if err := db.QueryRowContext(context, `SELECT h.instance_name,h.ip FROM monitor_target t JOIN assets_host h ON h.id=t.host_id WHERE t.id=?`, id).Scan(&name, &ip); err != nil {
		return fmt.Sprintf("target-%d", id)
	}
	if name.Valid && name.String != "" {
		return name.String
	}
	if ip.Valid && ip.String != "" {
		return ip.String
	}
	return fmt.Sprintf("target-%d", id)
}

func batchTargetIDs(context *gin.Context) ([]int64, bool) {
	var input struct {
		IDs []int64 `json:"ids"`
	}
	if context.ShouldBindJSON(&input) != nil || len(input.IDs) == 0 {
		response.BusinessError(context, 400, "ids must be a non-empty array", nil)
		return nil, false
	}
	return input.IDs, true
}

func (handler *Handler) BatchDeleteTargets(context *gin.Context) {
	ids, ok := batchTargetIDs(context)
	if !ok {
		return
	}
	results := make([]gin.H, 0, len(ids))
	success := 0
	for _, id := range ids {
		label := targetHostLabel(context, handler.db, id)
		var enabled bool
		var status string
		if err := handler.db.QueryRowContext(context, `SELECT managed_enabled,install_status FROM monitor_target WHERE id=?`, id).Scan(&enabled, &status); err != nil {
			results = append(results, gin.H{"id": id, "host": label, "ok": false, "message": "monitor target not found"})
			continue
		}
		if enabled {
			results = append(results, gin.H{"id": id, "host": label, "ok": false, "message": "disable the monitor target before deleting it"})
			continue
		}
		if status == "pending" {
			results = append(results, gin.H{"id": id, "host": label, "ok": false, "message": "wait for the uninstall task to finish before deleting"})
			continue
		}
		if _, err := handler.db.ExecContext(context, `DELETE FROM monitor_target WHERE id=?`, id); err != nil {
			results = append(results, gin.H{"id": id, "host": label, "ok": false, "message": err.Error()})
			continue
		}
		success++
		results = append(results, gin.H{"id": id, "host": label, "ok": true, "message": ""})
	}
	response.Success(context, gin.H{"total": len(results), "success": success, "failed": len(results) - success, "results": results})
}

func (handler *Handler) BatchStartTargetService(context *gin.Context) {
	handler.batchControlTargetService(context, "start")
}
func (handler *Handler) BatchStopTargetService(context *gin.Context) {
	handler.batchControlTargetService(context, "stop")
}

func (handler *Handler) batchControlTargetService(context *gin.Context, action string) {
	ids, ok := batchTargetIDs(context)
	if !ok {
		return
	}
	results := make([]gin.H, 0, len(ids))
	success := 0
	for _, id := range ids {
		label := targetHostLabel(context, handler.db, id)
		detail, err := handler.dispatchTargetServiceControl(context, id, action)
		if err != nil {
			results = append(results, gin.H{"id": id, "host": label, "ok": false, "message": err.Error()})
			continue
		}
		success++
		results = append(results, gin.H{"id": id, "host": label, "ok": true, "message": "", "detail": detail})
	}
	response.Success(context, gin.H{"total": len(results), "success": success, "failed": len(results) - success, "results": results})
}
