package baseline

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	"database/sql"

	db "autoadmin/internal/platform/database/generated"

	"github.com/gin-gonic/gin"
)

// 扫描编排：发起（解析项目/环境主机集合）→ 逐主机下发基线 OPA 策略检查 →
// 结果落 baseline_scan_result → 按主机聚合符合率。

type scanInput struct {
	MountType     string `json:"mount_type"`
	ProjectID     *int64 `json:"project_id"`
	EnvironmentID *int64 `json:"environment_id"`
}

type scanHost struct {
	HostID   int64
	HostName string
	HostIP   string
	AgentID  string
	Online   bool
}

// resolveHosts 复用 inspection 域的项目/环境主机集合查询（OS 基线只扫主机）。
func (handler *Handler) resolveHosts(ctx context.Context, input scanInput) ([]scanHost, error) {
	queries := db.New(handler.db)
	var rows []db.ListMountProjectHostsRow
	if input.MountType == "environment" {
		environmentRows, queryErr := queries.ListMountProjectEnvironmentHosts(ctx, db.ListMountProjectEnvironmentHostsParams{
			ProjectID:     sql.NullInt64{Int64: *input.ProjectID, Valid: true},
			EnvironmentID: sql.NullInt64{Int64: *input.EnvironmentID, Valid: true},
		})
		if queryErr != nil {
			return nil, queryErr
		}
		// 两个查询列集一致，统一到同一行类型处理。
		for _, row := range environmentRows {
			rows = append(rows, db.ListMountProjectHostsRow(row))
		}
	} else {
		projectRows, queryErr := queries.ListMountProjectHosts(ctx, sql.NullInt64{Int64: *input.ProjectID, Valid: true})
		if queryErr != nil {
			return nil, queryErr
		}
		rows = projectRows
	}
	hosts := make([]scanHost, 0, len(rows))
	for _, row := range rows {
		hosts = append(hosts, scanHost{HostID: row.ID, HostName: row.InstanceName, HostIP: row.Ip, AgentID: row.AgentID, Online: row.AgentOnline})
	}
	return hosts, nil
}

func (handler *Handler) StartScan(context *gin.Context) {
	var input scanInput
	if err := context.ShouldBindJSON(&input); err != nil {
		response.BusinessError(context, 400, "请求参数无效", nil)
		return
	}
	if input.MountType != "project" && input.MountType != "environment" {
		response.BusinessError(context, 400, "基线扫描只支持按项目或项目×环境选择主机", nil)
		return
	}
	if input.ProjectID == nil || *input.ProjectID <= 0 {
		response.BusinessError(context, 400, "必须选择项目", nil)
		return
	}
	if input.MountType == "environment" && (input.EnvironmentID == nil || *input.EnvironmentID <= 0) {
		response.BusinessError(context, 400, "按环境扫描必须选择环境", nil)
		return
	}
	baselineID := pathID(context)
	var enabled bool
	if err := handler.db.QueryRowContext(context, `SELECT enabled FROM baseline WHERE id=?`, baselineID).Scan(&enabled); err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "基线不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	if !enabled {
		response.BusinessError(context, 400, "基线已禁用", nil)
		return
	}
	var itemCount int
	if err := handler.db.QueryRowContext(context, `SELECT COUNT(*) FROM baseline_item WHERE baseline_id=?`, baselineID).Scan(&itemCount); err != nil || itemCount == 0 {
		response.BusinessError(context, 400, "基线没有检查条目", nil)
		return
	}
	hosts, err := handler.resolveHosts(context, input)
	if err != nil {
		response.Error(context, err)
		return
	}
	if len(hosts) == 0 {
		response.BusinessError(context, 400, "所选范围没有解析到任何主机", nil)
		return
	}
	username := ""
	if claims, _ := identity.ClaimsFromContext(context); claims != nil {
		username = claims.Username
	}
	tx, err := handler.db.BeginTx(context, nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(context, `INSERT INTO security_scan(create_time,update_time,scan_type,baseline_id,mount_type,project_id,environment_id,status,summary,requested_username) VALUES(NOW(6),NOW(6),'baseline',?,?,?,?, 'pending', JSON_OBJECT(), ?)`,
		baselineID, input.MountType, nullInt64FromPtr(input.ProjectID), nullInt64FromPtr(input.EnvironmentID), username)
	if err != nil {
		response.Error(context, err)
		return
	}
	scanID, err := result.LastInsertId()
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, host := range hosts {
		if _, err = tx.ExecContext(context, `INSERT INTO security_scan_target(scan_id,host_id,host_name,host_ip,agent_id,status,error_message) VALUES(?,?,?,?,?,'pending','')`,
			scanID, host.HostID, host.HostName, host.HostIP, host.AgentID); err != nil {
			response.Error(context, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	itemRows, err := db.New(handler.db).ListBaselineItems(context, baselineID)
	if err != nil {
		response.Error(context, err)
		return
	}
	go handler.runScan(scanID, itemRows, hosts)
	response.Success(context, gin.H{"scan_id": scanID, "targets": len(hosts), "status": "pending"})
}

// runScan 逐主机并发执行基线检查（每主机一次 OPA 策略下发），完成后聚合 summary。
func (handler *Handler) runScan(scanID int64, items []db.ListBaselineItemsRow, hosts []scanHost) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	if _, err := handler.db.Exec(`UPDATE security_scan SET status='running',update_time=NOW(6) WHERE id=? AND status='pending'`, scanID); err != nil {
		return
	}
	var waitGroup sync.WaitGroup
	semaphore := make(chan struct{}, 20)
	for _, host := range hosts {
		waitGroup.Add(1)
		go func(host scanHost) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			handler.runScanHost(ctx, scanID, items, host)
		}(host)
	}
	waitGroup.Wait()
	handler.finishScan(ctx, scanID, len(hosts))
}

// runScanHost 对单台主机执行全部基线条目（编译为一次 OPA 检查下发）。
func (handler *Handler) runScanHost(ctx context.Context, scanID int64, items []db.ListBaselineItemsRow, host scanHost) {
	var targetRowID int64
	if err := handler.db.QueryRowContext(ctx, `SELECT id FROM security_scan_target WHERE scan_id=? AND host_id=?`, scanID, host.HostID).Scan(&targetRowID); err != nil {
		return
	}
	setFailed := func(message string) {
		handler.db.ExecContext(ctx, `UPDATE security_scan_target SET status='failed',error_message=? WHERE id=?`, message, targetRowID)
	}
	if host.AgentID == "" || !handler.gateway.IsOnline(host.AgentID) {
		handler.db.ExecContext(ctx, `UPDATE security_scan_target SET status='skipped',error_message='Agent 离线，未执行扫描' WHERE id=?`, targetRowID)
		return
	}
	target := hostContext(host)
	agentChecks := make([]gin.H, 0, len(items))
	for index, item := range items {
		var config map[string]any
		_ = json.Unmarshal(item.Config, &config)
		// 唯一执行器 OPA：check_plan 不再携带 executor 字段。
		check := gin.H{
			"key":  fmt.Sprintf("baseline:%d:%d", scanID, index),
			"name": item.Name, "requires_running": false,
			"run_user": firstNonEmpty(stringFrom(config["run_user"]), "root"),
			"host_ip":  host.HostIP, "host_name": host.HostName,
			"config": gin.H{
				"input_commands": expandOpaInputs(config["input_commands"], target),
				"input_files":    expandOpaInputs(config["input_files"], target),
				"policy":         config["policy"],
			},
		}
		agentChecks = append(agentChecks, check)
	}
	paramsJSON := jsonBytes(gin.H{"check_plan": gin.H{"schema_version": 1, "checks": agentChecks}})
	execCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	responseData, execErr := handler.gateway.Execute(execCtx, host.AgentID, &pb.AutomationExecuteRequest{
		JobId: fmt.Sprintf("baseline-%d-%d", scanID, targetRowID), Type: "custom",
		Action: "check_application_baseline", ParamsJson: string(paramsJSON), TimeoutSeconds: 540,
	})
	cancel()
	if execErr != nil {
		setFailed(execErr.Error())
		return
	}
	var decoded struct {
		Checks []struct {
			Key, Status, Message string
			Actual               any
		} `json:"checks"`
	}
	_ = json.Unmarshal([]byte(responseData.ResultDataJson), &decoded)
	statusByKey := make(map[string]string, len(decoded.Checks))
	messageByKey := make(map[string]string, len(decoded.Checks))
	actualByKey := make(map[string]any, len(decoded.Checks))
	for _, check := range decoded.Checks {
		statusByKey[check.Key] = check.Status
		messageByKey[check.Key] = check.Message
		actualByKey[check.Key] = check.Actual
	}
	passed, failed := 0, 0
	results := make([]gin.H, 0)
	for index, item := range items {
		key := fmt.Sprintf("baseline:%d:%d", scanID, index)
		checkStatus, exists := statusByKey[key]
		if !exists {
			continue
		}
		resultStatus := "fail"
		if checkStatus == "pass" {
			resultStatus, passed = "pass", passed+1
		} else {
			failed++
		}
		results = append(results, gin.H{"item_id": item.ID, "item_name": item.Name, "chapter": item.Category,
			"severity": item.Severity, "status": resultStatus, "message": messageByKey[key], "actual": actualByKey[key]})
	}
	compliance := 0.0
	if passed+failed > 0 {
		compliance = float64(passed) / float64(passed+failed) * 100
	}
	status := "failed"
	if failed == 0 && passed > 0 {
		status = "success"
	}
	handler.db.ExecContext(ctx, `UPDATE security_scan_target SET status=?,passed_items=?,failed_items=?,compliance_rate=?,error_message='' WHERE id=?`,
		status, passed, failed, compliance, targetRowID)
	flushResults(ctx, handler.db, scanID, host.HostID, results)
}

// finishScan 聚合全部目标结果并关闭扫描。
func (handler *Handler) finishScan(ctx context.Context, scanID int64, totalHosts int) {
	var success, failed, skipped int
	handler.db.QueryRowContext(ctx, `SELECT SUM(status='success'),SUM(status='failed'),SUM(status='skipped') FROM security_scan_target WHERE scan_id=?`, scanID).Scan(&success, &failed, &skipped)
	status := "success"
	if failed > 0 {
		status = "failed"
	} else if success == 0 && skipped > 0 {
		status = "skipped"
	} else if success == 0 {
		status = "failed"
	}
	summary := jsonBytes(gin.H{"total": totalHosts, "success": success, "failed": failed, "skipped": skipped})
	handler.db.ExecContext(ctx, `UPDATE security_scan SET status=?,summary=?,end_time=NOW(6),update_time=NOW(6) WHERE id=?`, status, summary, scanID)
}

// flushResults 批量落基线条目结果（100 行/批）。
func flushResults(ctx context.Context, dbWriter dbExecutor, scanID, hostID int64, results []gin.H) {
	const batchSize = 100
	for start := 0; start < len(results); start += batchSize {
		end := min(start+batchSize, len(results))
		var builder strings.Builder
		builder.WriteString(`INSERT INTO baseline_scan_result(scan_id,host_id,item_id,item_name,chapter,severity,status,expected_value,actual_value,message) VALUES `)
		arguments := make([]any, 0, (end-start)*7)
		for index := start; index < end; index++ {
			if index > start {
				builder.WriteString(",")
			}
			builder.WriteString("(?,?,?,?,?,?,?,?,?,?)")
			item := results[index]
			arguments = append(arguments, scanID, hostID, item["item_id"], item["item_name"], item["chapter"], item["severity"], item["status"], jsonBytes(item["expected"]), jsonBytes(item["actual"]), item["message"])
		}
		if _, err := dbWriter.ExecContext(ctx, builder.String(), arguments...); err != nil {
			return
		}
	}
}

type dbExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// hostContext 提供 OS 基线可用的主机上下文变量（HOST_IP/HOST_NAME）。
func hostContext(host scanHost) map[string]string {
	return map[string]string{"HOST_IP": host.HostIP, "HOST_NAME": host.HostName}
}

// expandHostVars 展开 OS 基线条目里的主机变量；其余变量保持字面量。
func expandHostVars(value string, context map[string]string) string {
	for key, replacement := range context {
		value = strings.ReplaceAll(value, "${"+key+"}", replacement)
	}
	return value
}

// expandOpaInputs 展开 OPA 采集条目 exec/path 里的主机变量，其余字段透传。
func expandOpaInputs(raw any, context map[string]string) []any {
	items, ok := raw.([]any)
	if !ok {
		return []any{}
	}
	resolved := make([]any, 0, len(items))
	for _, item := range items {
		entry, valid := item.(map[string]any)
		if !valid {
			continue
		}
		clone := gin.H{}
		for key, value := range entry {
			if key == "exec" || key == "path" {
				clone[key] = expandHostVars(stringFrom(value), context)
			} else {
				clone[key] = value
			}
		}
		resolved = append(resolved, clone)
	}
	return resolved
}

func stringFrom(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func nullInt64FromPtr(value *int64) sql.NullInt64 {
	if value == nil || *value <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *value, Valid: true}
}

// ListScans 扫描记录（按类型过滤，默认基线）。
func (handler *Handler) ListScans(context *gin.Context) {
	scanType := strings.TrimSpace(context.Query("type"))
	if scanType == "" {
		scanType = "baseline"
	}
	rows, err := handler.db.QueryContext(context, `SELECT s.id, s.scan_type, b.name, s.mount_type, s.status, s.summary, s.requested_username, s.start_time, s.end_time, s.create_time
FROM security_scan s JOIN baseline b ON b.id = s.baseline_id
WHERE s.scan_type = ? ORDER BY s.id DESC LIMIT 100`, scanType)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer rows.Close()
	items := make([]gin.H, 0)
	for rows.Next() {
		var id int64
		var scanTypeValue, baselineName, mountType, status, requestedUsername string
		var summary []byte
		var startTime, endTime, createTime sql.NullTime
		if err := rows.Scan(&id, &scanTypeValue, &baselineName, &mountType, &status, &summary, &requestedUsername, &startTime, &endTime, &createTime); err != nil {
			response.Error(context, err)
			return
		}
		var summaryDecoded any
		_ = json.Unmarshal(summary, &summaryDecoded)
		items = append(items, gin.H{"id": id, "scan_type": scanTypeValue, "baseline": baselineName, "mount_type": mountType,
			"status": status, "summary": summaryDecoded, "requested_username": requestedUsername,
			"start_time": nullTimeString(startTime), "end_time": nullTimeString(endTime), "create_time": createTime})
	}
	response.Success(context, gin.H{"results": items})
}

// GetScan 扫描详情：概要 + 每主机符合率 + 不符合条目清单。
func (handler *Handler) GetScan(context *gin.Context) {
	var scanID int64
	fmt.Sscanf(strings.TrimSpace(context.Param("id")), "%d", &scanID)
	var scan gin.H
	var summary []byte
	err := handler.db.QueryRowContext(context, `SELECT JSON_OBJECT('id',s.id,'scan_type',s.scan_type,'baseline',b.name,'baseline_id',s.baseline_id,'mount_type',s.mount_type,'status',s.status,'requested_username',s.requested_username,'start_time',s.start_time,'end_time',s.end_time), s.summary
FROM security_scan s JOIN baseline b ON b.id=s.baseline_id WHERE s.id=?`, scanID).Scan(&scan, &summary)
	if err != nil {
		if err == sql.ErrNoRows {
			response.BusinessError(context, 404, "扫描记录不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	var summaryDecoded any
	_ = json.Unmarshal(summary, &summaryDecoded)
	scan["summary"] = summaryDecoded

	targets := make([]gin.H, 0)
	targetRows, err := handler.db.QueryContext(context, `SELECT host_name,host_ip,status,passed_items,failed_items,compliance_rate,error_message FROM security_scan_target WHERE scan_id=? ORDER BY id`, scanID)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer targetRows.Close()
	for targetRows.Next() {
		var hostName, hostIP, status, errorMessage string
		var passed, failed int32
		var compliance float64
		if err := targetRows.Scan(&hostName, &hostIP, &status, &passed, &failed, &compliance, &errorMessage); err != nil {
			response.Error(context, err)
			return
		}
		targets = append(targets, gin.H{"host_name": hostName, "host_ip": hostIP, "status": status,
			"passed_items": passed, "failed_items": failed, "compliance_rate": compliance, "error_message": errorMessage})
	}

	failures := make([]gin.H, 0)
	failureRows, err := handler.db.QueryContext(context, `SELECT host_id,item_name,chapter,severity,status,message FROM baseline_scan_result WHERE scan_id=? AND status='fail' ORDER BY host_id,severity,id`, scanID)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer failureRows.Close()
	for failureRows.Next() {
		var hostID int64
		var itemName, chapter, severity, status, message string
		if err := failureRows.Scan(&hostID, &itemName, &chapter, &severity, &status, &message); err != nil {
			response.Error(context, err)
			return
		}
		failures = append(failures, gin.H{"host_id": hostID, "item_name": itemName, "chapter": chapter, "severity": severity, "status": status, "message": message})
	}
	response.Success(context, gin.H{"scan": scan, "targets": targets, "failures": failures})
}

func nullTimeString(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
