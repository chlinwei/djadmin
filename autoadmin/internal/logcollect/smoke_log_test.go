package logcollect

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// TestSmokeLogCollectQueriesAgainstRealDatabase 把日志采集/日志存储两域的 sqlc 语句在**真库**上跑一遍写路径。
// mock 用例全绿不等于方言可用，这里专门验只有真库能暴露的点：
//
//  1. "已纳管就跳过"的两种表达：MySQL 的 `INSERT IGNORE`（影响行数 0）vs PG 的
//     `ON CONFLICT DO NOTHING`（被跳过时 RETURNING 不返回行 → sql.ErrNoRows）；
//  2. 收尾语句里的 CASE + 整数标志（`CASE WHEN $1 = 1` 在 PG 里按参数首次出现定类型）；
//  3. 通用配置资源的整行写合并语义（只提交部分列时其余列必须保持原值）；
//  4. application 显式传 null 与传值的 narg 语义差异（COALESCE 表达不了"显式写空"）；
//  5. 应用层承担的时间/时长计算（原本是 TIMESTAMPDIFF 等方言函数）。
//
// 全程一个事务 + 回滚；会改动全局数据的语句（clearDefaultLogRetentionTier 会把别的行 is_default
// 清掉）也在事务内执行。
//
// 用法：
//
//	MONITOR_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/logcollect/ -run RealDatabase -v
//	MONITOR_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/logcollect/ -run RealDatabase -v
func TestSmokeLogCollectQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("MONITOR_SMOKE_DSN")
	if dsn == "" {
		t.Skip("MONITOR_SMOKE_DSN 未设置：跳过真库冒烟（说明见本函数注释）")
	}
	ctx := context.Background()
	pool, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	queries, now := db.New(tx), time.Now().UTC()
	suffix := now.Format("150405.000000")

	// 被监控的主机（日志采集目标与告警服务树归属都要外键指向真实主机）。
	hostResult, err := queries.CreateHost(ctx, db.CreateHostParams{
		CreateTime: now, UpdateTime: now, Status: "active", IsDeletedInCloud: false,
		InstanceName:  sql.NullString{String: "smoke-alert-log-" + suffix, Valid: true},
		Ip:            sql.NullString{String: "10.255.255.204", Valid: true},
		CollectStatus: "pending", CollectMessage: "", AgentOnline: false,
		WebsshDefaultUsername: "", WebsshLoginUsers: "",
	})
	if err != nil {
		t.Fatalf("建主机：%v", err)
	}
	hostID, err := hostResult.LastInsertId()
	if err != nil {
		t.Fatalf("主机主键：%v", err)
	}

	// ---- 日志采集目标 ----
	targetID, created, err := createLogCollectionTargetIfAbsent(ctx, queries, db.CreateLogCollectionTargetIfAbsentParams{
		CreateTime: now, UpdateTime: now, HostID: hostID,
	})
	if err != nil || !created || targetID <= 0 {
		t.Fatalf("首次纳管日志目标：id=%d created=%v err=%v", targetID, created, err)
	}
	if _, created, err = createLogCollectionTargetIfAbsent(ctx, queries, db.CreateLogCollectionTargetIfAbsentParams{
		CreateTime: now, UpdateTime: now, HostID: hostID,
	}); err != nil || created {
		t.Fatalf("重复纳管应跳过：created=%v err=%v", created, err)
	}
	targetRow, err := queries.GetLogTargetForAction(ctx, targetID)
	if err != nil || targetRow.HostID != hostID || targetRow.InstallStatus != "unknown" {
		t.Fatalf("读日志目标：%+v err=%v", targetRow, err)
	}
	if _, err := queries.GetLogTargetHostName(ctx, targetID); err != nil {
		t.Fatalf("读日志目标主机名：%v", err)
	}
	if err := queries.MarkLogTargetInstallPending(ctx, db.MarkLogTargetInstallPendingParams{
		UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("日志目标置 pending：%v", err)
	}
	historyID, err := queries.CreateLogTargetInstallHistory(ctx, db.CreateLogTargetInstallHistoryParams{
		CreateTime: now, UpdateTime: now, Action: "install",
		HostIDSnapshot:       sql.NullInt32{Int32: int32(hostID), Valid: true},
		HostNameSnapshot:     targetRow.InstanceName,
		HostIpSnapshot:       targetRow.Ip,
		ExporterTypeSnapshot: "filebeat", SummaryMessage: "",
		RequestedUserIDSnapshot: sql.NullInt32{}, RequestedUsernameSnapshot: "system",
		HostID:                sql.NullInt64{Int64: hostID, Valid: true},
		LogCollectionTargetID: sql.NullInt64{Int64: targetID, Valid: true},
	})
	if err != nil {
		t.Fatalf("建日志目标安装历史：%v", err)
	}
	pending, latest, err := logTargetPending(ctx, tx, targetID)
	if err != nil || !pending || latest.ID != historyID {
		t.Fatalf("日志目标在途判定：pending=%v latest=%+v err=%v", pending, latest, err)
	}
	// 收尾：时长由应用层算，CASE 里的 install_succeeded 用整数传（PG 按参数首次出现定类型）。
	finishedAt := time.Now().UTC()
	if affected, err := queries.FinishLogTargetInstallState(ctx, db.FinishLogTargetInstallStateParams{
		InstallStatus: "success", InstallMessage: "", InstallSucceeded: 1,
		AgentInstalled: sql.NullBool{Bool: true, Valid: true},
		UpdateTime:     finishedAt, ID: targetID,
	}); err != nil || affected != 1 {
		t.Fatalf("收尾日志目标：affected=%d err=%v", affected, err)
	}
	managed, err := queries.ListManagedLogTargetConfigs(ctx)
	if err != nil {
		t.Fatalf("读纳管日志目标配置：%v", err)
	}
	agentInstalled := false
	for _, item := range managed {
		if item.ID == targetID {
			agentInstalled = item.AgentInstalled
		}
	}
	if !agentInstalled {
		t.Fatal("安装成功后 agent_installed 应置 TRUE（否则链路体检误报未安装）")
	}
	if affected, err := queries.FinishLogTargetInstallHistory(ctx, db.FinishLogTargetInstallHistoryParams{
		Status: "success", SummaryMessage: "",
		EndTime:         sql.NullTime{Time: finishedAt, Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: finishedAt.Sub(now).Seconds(), Valid: true},
		UpdateTime:      finishedAt, ID: historyID,
	}); err != nil || affected != 1 {
		t.Fatalf("收尾日志历史：affected=%d err=%v", affected, err)
	}
	if affected, err := queries.FinishLogTargetInstallState(ctx, db.FinishLogTargetInstallStateParams{
		InstallStatus: "failed", InstallMessage: "重复收尾", InstallSucceeded: 0,
		UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil || affected != 0 {
		t.Fatalf("重复收尾应 0 行：affected=%d err=%v", affected, err)
	}
	if err := queries.SetLogTargetRuntimeStatus(ctx, db.SetLogTargetRuntimeStatusParams{
		RuntimeStatus: "running", UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("写日志目标运行态：%v", err)
	}
	// 指纹未变 / 变更两条路径。
	if _, err := queries.GetLogTargetConfigFingerprint(ctx, targetID); err != nil {
		t.Fatalf("读配置指纹：%v", err)
	}
	if err := queries.MarkLogTargetConfigApplied(ctx, db.MarkLogTargetConfigAppliedParams{
		LastAppliedTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		UpdateTime:      time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("刷新下发时间：%v", err)
	}
	if err := queries.MarkLogTargetConfigSynced(ctx, db.MarkLogTargetConfigSyncedParams{
		LastAppliedTime:   sql.NullTime{Time: time.Now().UTC(), Valid: true},
		ConfigFingerprint: "smoke-" + suffix, UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("写配置指纹：%v", err)
	}
	if !targetRow.ManagedEnabled {
		t.Fatalf("新建的日志目标应处于纳管状态")
	}

	// ---- Elasticsearch 集群（整行插入 / 整行更新 / 探测与同步状态 / 默认唯一）----
	clusterID, err := queries.CreateElasticsearchCluster(ctx, db.CreateElasticsearchClusterParams{
		CreateTime: now, UpdateTime: now, Name: "smoke-cluster-" + suffix, Hosts: "http://127.0.0.1:9200",
		Username: "admin", Password: "encrypted", VerifyTls: false, CaCert: "",
		IndexPrefix: "smoke-" + suffix, RequestTimeout: 10, Enabled: true, IsDefault: true, Remark: "冒烟",
	})
	if err != nil {
		t.Fatalf("建 Elasticsearch 集群：%v", err)
	}
	if count, err := queries.CountAllElasticsearchClusters(ctx); err != nil || count < 1 {
		t.Fatalf("集群计数：%d err=%v", count, err)
	}
	if err := queries.ClearDefaultElasticsearchCluster(ctx, db.ClearDefaultElasticsearchClusterParams{
		UpdateTime: time.Now().UTC(), ID: clusterID,
	}); err != nil {
		t.Fatalf("清理其它默认集群：%v", err)
	}
	cluster, err := queries.GetElasticsearchClusterConnection(ctx, clusterID)
	if err != nil || cluster.IndexPrefix != "smoke-"+suffix {
		t.Fatalf("读集群连接：%+v err=%v", cluster, err)
	}
	// 整行写：这里直接复用生产代码的"读回 + 合并"路径（未提交的列必须保持原值）。
	merged, err := mergedElasticsearchCluster(context.Background(), queries, clusterID, map[string]any{
		"hosts": "http://127.0.0.1:9201", "remark": "改过",
	})
	if err != nil {
		t.Fatalf("合并集群：%v", err)
	}
	if merged.Name != "smoke-cluster-"+suffix || merged.Password != "encrypted" {
		t.Fatalf("合并语义不符：%+v", merged)
	}
	if affected, err := queries.UpdateElasticsearchCluster(ctx, db.UpdateElasticsearchClusterParams{
		UpdateTime: time.Now().UTC(), Name: merged.Name, Hosts: merged.Hosts,
		Username: merged.Username, Password: merged.Password, VerifyTls: merged.VerifyTls,
		CaCert: merged.CaCert, IndexPrefix: merged.IndexPrefix, RequestTimeout: merged.RequestTimeout,
		Enabled: merged.Enabled, IsDefault: merged.IsDefault, Remark: merged.Remark, ID: clusterID,
	}); err != nil || affected != 1 {
		t.Fatalf("更新集群：affected=%d err=%v", affected, err)
	}
	if updated, err := queries.GetElasticsearchClusterTyped(ctx, clusterID); err != nil || updated.Password != "encrypted" || updated.Hosts != "http://127.0.0.1:9201" {
		t.Fatalf("合并后的集群：%+v err=%v", updated, err)
	}
	if err := queries.MarkElasticsearchClusterCheckFailed(ctx, db.MarkElasticsearchClusterCheckFailedParams{
		LastCheckTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, LastCheckMessage: "探测失败",
		UpdateTime: time.Now().UTC(), ID: clusterID,
	}); err != nil {
		t.Fatalf("写探测失败：%v", err)
	}
	if err := queries.MarkElasticsearchClusterCheckSuccess(ctx, db.MarkElasticsearchClusterCheckSuccessParams{
		LastCheckTime: sql.NullTime{Time: time.Now().UTC(), Valid: true}, LastCheckMessage: "ok",
		UpdateTime: time.Now().UTC(), ID: clusterID,
	}); err != nil {
		t.Fatalf("写探测成功：%v", err)
	}
	if err := queries.MarkClusterStorageSyncPending(ctx, db.MarkClusterStorageSyncPendingParams{
		UpdateTime: time.Now().UTC(), ID: clusterID,
	}); err != nil {
		t.Fatalf("写同步 pending：%v", err)
	}
	if err := queries.MarkClusterStorageSyncFailed(ctx, db.MarkClusterStorageSyncFailedParams{
		StorageSyncError: "模板下发失败", StorageSyncTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		UpdateTime: time.Now().UTC(), ID: clusterID,
	}); err != nil {
		t.Fatalf("写同步失败：%v", err)
	}
	if err := queries.MarkClusterStorageSyncSuccess(ctx, db.MarkClusterStorageSyncSuccessParams{
		StorageSyncTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		UpdateTime:      time.Now().UTC(), ID: clusterID,
	}); err != nil {
		t.Fatalf("写同步成功：%v", err)
	}
	if _, err := queries.ListEnabledElasticsearchClusterIDs(ctx); err != nil {
		t.Fatalf("列启用集群：%v", err)
	}
	if defaultCluster, err := queries.GetDefaultEnabledElasticsearchCluster(ctx); err != nil || defaultCluster.Hosts == "" {
		t.Fatalf("读默认启用集群：%+v err=%v", defaultCluster, err)
	}

	// ---- 保留档位与解析规则（通用配置资源：写路径按表分派 + 整行写）----
	tierID, err := createLogRetentionTier(ctx, tx, map[string]any{
		"code": "smoke-" + suffix, "name": "冒烟档位", "daily_size_gb": float64(2),
		"retention_days": float64(7), "rollover_min_index_age": "1d",
		"enabled": true, "is_default": true, "remark": "冒烟",
	})
	if err != nil {
		t.Fatalf("建保留档位：%v", err)
	}
	if err := updateLogRetentionTier(ctx, tx, tierID, map[string]any{"retention_days": float64(30)}); err != nil {
		t.Fatalf("更新保留档位：%v", err)
	}
	if tier, err := queries.GetLogRetentionTier(ctx, tierID); err != nil || int(tier.RetentionDays) != 30 || tier.Name != "冒烟档位" {
		// 关键断言：只提交了 retention_days，其余列必须保持原值（整行写的合并语义）。
		t.Fatalf("合并后的档位：%+v err=%v", tier, err)
	}
	if _, err := queries.CountRetentionTierServices(ctx, sql.NullInt64{Int64: tierID, Valid: true}); err != nil {
		t.Fatalf("档位占用计数：%v", err)
	}
	if _, err := queries.CountRetentionTierLogSettings(ctx, sql.NullInt64{Int64: tierID, Valid: true}); err != nil {
		t.Fatalf("档位日志设置计数：%v", err)
	}
	if err := clearDefaultLogRetentionTier(ctx, tx, tierID); err != nil {
		t.Fatalf("清理其它默认档位：%v", err)
	}

	// application_id 可空且带外键指向 assets_application：取一个真实存在的应用，没有就写 NULL。
	var realApplicationID int64
	applicationFound := tx.QueryRowContext(ctx, "SELECT id FROM assets_application ORDER BY id LIMIT 1").Scan(&realApplicationID) == nil
	var applicationValue any
	if applicationFound {
		applicationValue = float64(realApplicationID)
	}
	ruleID, err := createLogProcessingRule(ctx, tx, map[string]any{
		"cluster": float64(clusterID), "name": "smoke-rule-" + suffix, "description": "冒烟",
		"input_format": "json", "multiline_enabled": false, "start_pattern": "", "continuation_pattern": "",
		"flush_timeout": float64(5000), "pipeline_body": map[string]any{"processors": []any{}},
	})
	if err != nil {
		t.Fatalf("建解析规则：%v", err)
	}
	if err := updateLogProcessingRule(ctx, tx, ruleID, map[string]any{"description": "改过", "application": applicationValue}); err != nil {
		t.Fatalf("更新解析规则：%v", err)
	}
	if rule, err := queries.GetLogProcessingRule(ctx, ruleID); err != nil || rule.Description != "改过" || rule.Name != "smoke-rule-"+suffix || rule.ApplicationID.Valid != applicationFound {
		// application 显式传值 → application_id 落该值；显式传 null → 必须是 NULL
		//（narg 的语义，用 COALESCE 表达不了"显式写空"）。
		t.Fatalf("合并后的规则：%+v err=%v", rule, err)
	}
	if _, err := queries.CountLogDefinitionReferences(ctx, sql.NullInt64{Int64: ruleID, Valid: true}); err != nil {
		t.Fatalf("规则引用计数：%v", err)
	}
	if _, err := queries.ListProcessingRulesByCluster(ctx, clusterID); err != nil {
		t.Fatalf("按集群列解析规则：%v", err)
	}

	filterRuleID, err := createLogCollectionFilterRule(ctx, tx, map[string]any{
		"name": "smoke-filter-" + suffix, "description": "冒烟", "pattern": "password=.*",
		"enabled": true, "application": applicationValue,
	})
	if err != nil {
		t.Fatalf("建过滤规则：%v", err)
	}
	if err := updateLogCollectionFilterRule(ctx, tx, filterRuleID, map[string]any{"enabled": false}); err != nil {
		t.Fatalf("更新过滤规则：%v", err)
	}
	if rule, err := queries.GetLogCollectionFilterRule(ctx, filterRuleID); err != nil || rule.Enabled || rule.Pattern != "password=.*" {
		t.Fatalf("合并后的过滤规则：%+v err=%v", rule, err)
	}

	// ---- 只读：保留档位 / 日志目标对账 / 数据流维度 / 应用维度 ----
	if _, err := queries.ListEnabledRetentionTiers(ctx); err != nil {
		t.Fatalf("列启用档位：%v", err)
	}
	if _, err := queries.ListManagedLogTargetConfigs(ctx); err != nil {
		t.Fatalf("列纳管中的日志目标：%v", err)
	}
	if _, err := queries.ListInstalledLogTargetRuntime(ctx); err != nil {
		t.Fatalf("列已装 Filebeat 的目标：%v", err)
	}
	if _, err := queries.ListServiceStreamDims(ctx); err != nil {
		t.Fatalf("列服务流维度：%v", err)
	}
	if _, err := queries.ListEnabledProjects(ctx); err != nil {
		t.Fatalf("列启用项目：%v", err)
	}
	if _, err := queries.ListEnabledBusinessSystems(ctx); err != nil {
		t.Fatalf("列启用业务系统：%v", err)
	}
	if _, err := queries.ListEnabledBusinessEnvironments(ctx); err != nil {
		// `ORDER BY \`order\`,name` —— 保留字列名在 PG 侧必须由派生加成双引号。
		t.Fatalf("列启用环境：%v", err)
	}

	if _, err := queries.GetApplicationNameCode(ctx, realApplicationID); err != nil && err != sql.ErrNoRows {
		t.Fatalf("读应用名与编码：%v", err)
	}
	if _, err := queries.GetApplicationServiceCode(ctx, realApplicationID); err != nil && err != sql.ErrNoRows {
		t.Fatalf("读逻辑服务编码：%v", err)
	}

	// ---- 收尾：删除路径（含外键解绑）----
	if err := queries.DetachInstallHistoryFromLogTarget(ctx, sql.NullInt64{Int64: targetID, Valid: true}); err != nil {
		t.Fatalf("解绑安装历史：%v", err)
	}
	if err := deleteRowsAffected(queries.DeleteLogCollectionTarget(ctx, targetID)); err != nil {
		t.Fatalf("删日志目标：%v", err)
	}
	if err := deleteRowsAffected(queries.DeleteLogCollectionTarget(ctx, targetID)); err != sql.ErrNoRows {
		t.Fatalf("重复删应 ErrNoRows，实际 %v", err)
	}
	if err := deleteLogRetentionTier(ctx, tx, tierID); err != nil {
		t.Fatalf("删保留档位：%v", err)
	}
	if err := deleteLogProcessingRule(ctx, tx, ruleID); err != nil {
		t.Fatalf("删解析规则：%v", err)
	}
	if err := deleteLogCollectionFilterRule(ctx, tx, filterRuleID); err != nil {
		t.Fatalf("删过滤规则：%v", err)
	}
	if err := deleteRowsAffected(queries.DeleteElasticsearchCluster(ctx, clusterID)); err != nil {
		t.Fatalf("删集群：%v", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatalf("回滚：%v", err)
	}
}
