package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"autoadmin/internal/agent"
	"autoadmin/internal/agent/pb"
	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"

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
	// InstanceName 是主机 instance_name（= assets_host.instance_name），
	// 也是 gRPC 网关会话的路由 key。
	InstanceName string
	Online       bool
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
		hosts = append(hosts, scanHost{HostID: row.ID, HostName: row.InstanceName, HostIP: row.Ip, InstanceName: row.InstanceName, Online: row.AgentOnline})
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
	queries := db.New(handler.db)
	baselineRow, err := queries.GetBaselineForScan(context, baselineID)
	if err != nil {
		if isNoRows(err) {
			response.BusinessError(context, 404, "基线不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	if !baselineRow.Enabled {
		response.BusinessError(context, 400, "基线已禁用", nil)
		return
	}
	itemCount, err := queries.CountBaselineItems(context, baselineID)
	if err != nil {
		response.Error(context, err)
		return
	}
	if itemCount == 0 {
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
	now := time.Now().UTC()
	scanID, err := db.New(tx).CreateBaselineScan(context, db.CreateBaselineScanParams{
		CreateTime: now, UpdateTime: now, BaselineID: baselineID, MountType: input.MountType,
		ProjectID: nullInt64FromPtr(input.ProjectID), EnvironmentID: nullInt64FromPtr(input.EnvironmentID),
		RequestedUsername: username,
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	for _, host := range hosts {
		if err = db.New(tx).CreateBaselineScanTargets(context, db.CreateBaselineScanTargetsParams{
			ScanID: scanID, HostID: host.HostID, HostName: host.HostName, HostIp: host.HostIP,
			InstanceNameSnapshot: host.InstanceName,
		}); err != nil {
			response.Error(context, err)
			return
		}
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	itemRows, err := queries.ListBaselineItems(context, baselineID)
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
	now := time.Now().UTC()
	if _, err := db.New(handler.db).ClaimBaselineScan(ctx, db.ClaimBaselineScanParams{
		StartTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: scanID,
	}); err != nil {
		return
	}
	var waitGroup sync.WaitGroup
	semaphore := make(chan struct{}, 20)
	for _, host := range hosts {
		if handler.isCanceled(scanID) {
			break // 已取消：剩余目标的 target 行已被 CancelScan 置 canceled
		}
		waitGroup.Add(1)
		go func(host scanHost) {
			defer waitGroup.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			if handler.isCanceled(scanID) {
				return
			}
			handler.runScanHost(ctx, scanID, items, host)
		}(host)
	}
	waitGroup.Wait()
	if handler.isCanceled(scanID) {
		return // 终态已由 CancelScan 落库（canceled），不聚合覆盖
	}
	handler.finishScan(ctx, scanID, len(hosts))
}

// runScanHost 对单台主机执行全部基线条目（编译为一次 OPA 检查下发）。
func (handler *Handler) runScanHost(ctx context.Context, scanID int64, items []db.ListBaselineItemsRow, host scanHost) {
	queries := db.New(handler.db)
	targetRow, err := queries.GetBaselineScanTarget(ctx, db.GetBaselineScanTargetParams{ScanID: scanID, HostID: host.HostID})
	if err != nil {
		return
	}
	targetRowID := targetRow.ID
	setFailed := func(message string) {
		queries.SetBaselineScanTargetStatus(ctx, db.SetBaselineScanTargetStatusParams{Status: "failed", ErrorMessage: message, ID: targetRowID})
	}
	if host.InstanceName == "" || !handler.gateway.IsOnline(host.InstanceName) {
		queries.SetBaselineScanTargetStatus(ctx, db.SetBaselineScanTargetStatusParams{Status: "skipped", ErrorMessage: "Agent 离线，未执行扫描", ID: targetRowID})
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
	responseData, execErr := handler.gateway.Execute(execCtx, host.InstanceName, &pb.AutomationExecuteRequest{
		JobId: fmt.Sprintf("baseline-%d-%d", scanID, targetRowID), Type: "custom",
		Action: "check_application_baseline", ParamsJson: string(paramsJSON), TimeoutSeconds: 540,
	})
	cancel()
	if handler.isCanceled(scanID) {
		return // 取消后丢弃迟到响应（target 终态已由 CancelScan 落库）
	}
	if execErr != nil {
		// 离线是"未执行"而非"检查失败"：下发瞬间会话可能已断（IsOnline 仍看得到半死会话），
		// 此时必须与前置检查一致地置 skipped，否则会把离线机器算作"存在不符合"。
		if errors.Is(execErr, agent.ErrAgentOffline) {
			queries.SetBaselineScanTargetStatus(ctx, db.SetBaselineScanTargetStatusParams{
				Status: "skipped", ErrorMessage: "Agent 离线，未执行扫描", ID: targetRowID,
			})
			return
		}
		setFailed(execErr.Error())
		return
	}
	var decoded struct {
		Checks []struct {
			Key, Status, Message string
			Expected, Actual     any
		} `json:"checks"`
	}
	_ = json.Unmarshal([]byte(responseData.ResultDataJson), &decoded)
	statusByKey := make(map[string]string, len(decoded.Checks))
	messageByKey := make(map[string]string, len(decoded.Checks))
	expectedByKey := make(map[string]any, len(decoded.Checks))
	actualByKey := make(map[string]any, len(decoded.Checks))
	for _, check := range decoded.Checks {
		statusByKey[check.Key] = check.Status
		messageByKey[check.Key] = check.Message
		expectedByKey[check.Key] = check.Expected
		actualByKey[check.Key] = check.Actual
	}
	passed, failed := 0, 0
	results := make([]scanResultRow, 0, len(items))
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
		var config map[string]any
		_ = json.Unmarshal(item.Config, &config)
		results = append(results, scanResultRow{itemID: item.ID, itemName: item.Name, chapter: item.Category,
			severity: item.Severity, status: resultStatus, message: messageByKey[key],
			expected: expectedByKey[key], actual: actualByKey[key],
			remediation: stringFrom(config["remediation"])})
	}
	compliance := 0.0
	if passed+failed > 0 {
		compliance = float64(passed) / float64(passed+failed) * 100
	}
	status := "failed"
	if failed == 0 && passed > 0 {
		status = "success"
	}
	queries.FinishBaselineScanTarget(ctx, db.FinishBaselineScanTargetParams{
		Status: status, PassedItems: int32(passed), FailedItems: int32(failed),
		ComplianceRate: decimalString(compliance), ErrorMessage: "", ID: targetRowID,
	})
	flushResults(ctx, handler.db, scanID, host.HostID, results)
}

// finishScan 聚合全部目标结果并关闭扫描。
func (handler *Handler) finishScan(ctx context.Context, scanID int64, totalHosts int) {
	// 聚合失败不能把扫描卡在 running：拿不到计数就按 0 处理（与原实现一致，那里也忽略了 scan 错误）。
	counts, _ := db.New(handler.db).CountScanTargetsByStatus(ctx, scanID)
	status := "success"
	if counts.Failed > 0 {
		status = "failed"
	} else if counts.Success == 0 && counts.Skipped > 0 {
		status = "skipped"
	} else if counts.Success == 0 {
		status = "failed"
	}
	summary := jsonBytes(gin.H{"total": totalHosts, "success": counts.Success, "failed": counts.Failed, "skipped": counts.Skipped})
	now := time.Now().UTC()
	db.New(handler.db).FinishBaselineScan(ctx, db.FinishBaselineScanParams{
		Status: status, Summary: summary, EndTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: scanID,
	})
}

// CancelScan 取消进行中的扫描（对齐巡检 CancelExecution）：
// 仅 pending/running 可取消；DB 原子置 canceled + end_time，未开始的目标同步置 canceled；
// 内存镜像让扫描 goroutine 丢弃迟到结果并不再聚合。Agent 侧已下发的执行无法远程终止。
func (handler *Handler) CancelScan(context *gin.Context) {
	scanID := int64(0)
	fmt.Sscanf(strings.TrimSpace(context.Param("id")), "%d", &scanID)
	if scanID < 1 {
		response.BusinessError(context, 400, "scan id 无效", nil)
		return
	}
	tx, err := handler.db.BeginTx(context.Request.Context(), nil)
	if err != nil {
		response.Error(context, err)
		return
	}
	defer tx.Rollback()
	queries := db.New(tx)
	// FOR UPDATE 锁行后由应用层读改写 summary（JSON_MERGE_PATCH 是 MySQL 专有函数）。
	current, err := queries.GetScanStatusForUpdate(context.Request.Context(), scanID)
	if err != nil {
		if isNoRows(err) {
			response.BusinessError(context, 404, "扫描不存在", nil)
			return
		}
		response.Error(context, err)
		return
	}
	if current.Status != "pending" && current.Status != "running" {
		response.BusinessError(context, 400, "扫描已结束，无法取消", nil)
		return
	}
	now := time.Now().UTC()
	if _, err = queries.CancelBaselineScan(context.Request.Context(), db.CancelBaselineScanParams{
		Summary: mergeCanceledSummary(current.Summary), EndTime: sql.NullTime{Time: now, Valid: true},
		UpdateTime: now, ID: scanID,
	}); err != nil {
		response.Error(context, err)
		return
	}
	if err = queries.CancelBaselineScanTargets(context.Request.Context(), scanID); err != nil {
		response.Error(context, err)
		return
	}
	if err = tx.Commit(); err != nil {
		response.Error(context, err)
		return
	}
	handler.markCanceled(scanID)
	response.Success(context, gin.H{"id": scanID, "status": "canceled"})
}

// mergeCanceledSummary 等价于原来的 JSON_MERGE_PATCH(summary, '{"canceled":true}')：
// 对象则合并键，非对象（JSON_MERGE_PATCH 的语义是整体替换）则重建为只带标记的对象。
func mergeCanceledSummary(raw json.RawMessage) json.RawMessage {
	merged := map[string]any{}
	_ = json.Unmarshal(raw, &merged)
	if merged == nil {
		merged = map[string]any{}
	}
	merged["canceled"] = true
	return jsonBytes(merged)
}

// scanResultRow 是一条待落库的条目结果。用结构体而不是 gin.H：这里的所有字段都要落库，
// 结构体让字段名/类型由编译器钉住（原来从 map 里取 item_id 要做类型断言，断言失败会静默落 0）。
type scanResultRow struct {
	itemID      int64
	itemName    string
	chapter     string
	severity    string
	status      string
	message     string
	expected    any
	actual      any
	remediation string
}

// flushResults 逐条落基线条目结果。
//
// 原实现按 100 行/批拼一条多行 INSERT：占位符个数随入参变化，是 sqlc 表达不了、只能内联的形状
// （SQL_DESIGN §1），而拼出来的文本用 `?` 占位符——PG 变体（pgx）不接受 `?`，那条路径在 PG 上
// 根本跑不通。这里改成走 sqlc 的单行 INSERT：目标是每主机几十到几百条，摊在按主机并发、
// 单机耗时以远端命令执行为主的扫描里可以忽略；真要回到批量插入，就按方言各写一份。
func flushResults(ctx context.Context, executor db.DBTX, scanID, hostID int64, results []scanResultRow) {
	queries := db.New(executor)
	for _, item := range results {
		if err := queries.CreateBaselineScanResults(ctx, db.CreateBaselineScanResultsParams{
			ScanID: scanID, HostID: hostID, ItemID: item.itemID,
			ItemName: item.itemName, Chapter: item.chapter, Severity: item.severity, Status: item.status,
			ExpectedValue: jsonBytes(item.expected), ActualValue: jsonBytes(item.actual),
			Message:     item.message,
			Remediation: sql.NullString{String: item.remediation, Valid: item.remediation != ""},
		}); err != nil {
			return
		}
	}
}

// decimalString 把数值写成 decimal(5,2) 列能接受的文本：sqlc 两侧都把该列生成为 string
// （MySQL 驱动把 DECIMAL 当文本返回，PG 的 numeric 同理），写入也就必须给文本。
func decimalString(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

// decimalValue 把取回的 decimal(5,2) 文本还原成 API 契约里的数值。列 NOT NULL，解析失败退 0。
func decimalValue(text string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
	if err != nil {
		return 0
	}
	return value
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

// ListScans 扫描记录（按类型过滤，默认基线；page/page_size 分页，page_size 默认 10 上限 100）。
func (handler *Handler) ListScans(context *gin.Context) {
	scanType := strings.TrimSpace(context.Query("type"))
	if scanType == "" {
		scanType = "baseline"
	}
	page, pageSize := 1, 10
	fmt.Sscanf(strings.TrimSpace(context.Query("page")), "%d", &page)
	fmt.Sscanf(strings.TrimSpace(context.Query("page_size")), "%d", &pageSize)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}
	queries := db.New(handler.db)
	total, err := queries.CountScansByType(context, scanType)
	if err != nil {
		response.Error(context, err)
		return
	}
	scanRows, err := queries.ListScans(context, db.ListScansParams{
		ScanType: scanType, Limit: int32(pageSize), Offset: int32((page - 1) * pageSize),
	})
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(scanRows))
	for _, row := range scanRows {
		var summaryDecoded any
		_ = json.Unmarshal(row.Summary, &summaryDecoded)
		items = append(items, gin.H{"id": row.ID, "scan_type": row.ScanType, "baseline": row.Baseline, "mount_type": row.MountType,
			"status": row.Status, "summary": summaryDecoded, "requested_username": row.RequestedUsername,
			"start_time": nullTimeString(row.StartTime), "end_time": nullTimeString(row.EndTime), "create_time": row.CreateTime})
	}
	response.Success(context, gin.H{"count": total, "results": items})
}

// GetScan 扫描详情：概要 + 每主机符合率 + 不符合条目清单。
func (handler *Handler) GetScan(context *gin.Context) {
	var scanID int64
	fmt.Sscanf(strings.TrimSpace(context.Param("id")), "%d", &scanID)
	queries := db.New(handler.db)
	// 响应体在应用层组装：原来用 JSON_OBJECT 在库里拼，PG 没有该函数。
	header, err := queries.GetScanHeader(context, scanID)
	if err != nil {
		if isNoRows(err) {
			response.BusinessError(context, 404, "扫描记录不存在", nil)
		} else {
			response.Error(context, err)
		}
		return
	}
	var summaryDecoded any
	_ = json.Unmarshal(header.Summary, &summaryDecoded)
	scan := gin.H{"id": header.ID, "scan_type": header.ScanType, "baseline": header.Baseline, "baseline_id": header.BaselineID,
		"mount_type": header.MountType, "status": header.Status, "summary": summaryDecoded,
		"requested_username": header.RequestedUsername,
		// 时间列从 JSON_OBJECT 的库内文本变成本地 time.Time：格式从
		// "2026-09-16 21:00:00.000000" 变成 RFC3339，与列表接口的表示一致。
		"start_time": nullTimeString(header.StartTime), "end_time": nullTimeString(header.EndTime)}

	targetRows, err := queries.ListScanTargets(context, scanID)
	if err != nil {
		response.Error(context, err)
		return
	}
	targets := make([]gin.H, 0, len(targetRows))
	for _, row := range targetRows {
		targets = append(targets, gin.H{"host_name": row.HostName, "host_ip": row.HostIp, "status": row.Status,
			"passed_items": row.PassedItems, "failed_items": row.FailedItems,
			"compliance_rate": decimalValue(row.ComplianceRate), "error_message": row.ErrorMessage})
	}

	itemRows, err := queries.ListBaselineScanResults(context, scanID)
	if err != nil {
		response.Error(context, err)
		return
	}
	items := make([]gin.H, 0, len(itemRows))
	for _, row := range itemRows {
		items = append(items, gin.H{"host_id": row.HostID, "host_name": row.HostName, "host_ip": row.HostIp,
			"item_name": row.ItemName, "chapter": row.Chapter, "severity": row.Severity, "status": row.Status,
			"expected": row.ExpectedValue, "actual": row.ActualValue, "message": row.Message,
			"remediation": row.Remediation})
	}
	response.Success(context, gin.H{"scan": scan, "targets": targets, "items": items})
}

func nullTimeString(value sql.NullTime) any {
	if !value.Valid {
		return nil
	}
	return value.Time
}
