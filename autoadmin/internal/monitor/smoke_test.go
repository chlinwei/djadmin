package monitor

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// TestSmokeTargetDomainQueriesAgainstRealDatabase 把监控目标域迁到 sqlc 的语句在**真库**上跑一遍。
//
// 重点验三处只有真库能验的东西：
//  1. **"已纳管就跳过"的两种表达**：MySQL 的 `INSERT IGNORE`（影响行数 0）vs PG 的
//     `ON CONFLICT DO NOTHING`（被跳过时 RETURNING 不返回行 → sql.ErrNoRows），
//     两条路径都必须报告"未新建"；
//  2. **宿主总览的 CASE 过滤**（有/无指定 exporter、日志已装/未装），PG 在解析期按参数
//     首次出现定类型，这类写法只有真 PG 会暴露问题；
//  3. 安装历史的状态流转与取消（含可空列与时长写入）。
//
// 全程一个事务 + 回滚，不依赖库里预先有数据（测试自己建项目/业务系统/主机链路）。
//
// 用法：
//
//	MONITOR_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/monitor/ -run RealDatabase -v
//	MONITOR_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/monitor/ -run RealDatabase -v
func TestSmokeTargetDomainQueriesAgainstRealDatabase(t *testing.T) {
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

	hostResult, err := queries.CreateHost(ctx, db.CreateHostParams{
		CreateTime: now, UpdateTime: now, Status: "active", IsDeletedInCloud: false,
		InstanceName:  sql.NullString{String: "smoke-monitor-" + suffix, Valid: true},
		Ip:            sql.NullString{String: "10.255.255.203", Valid: true},
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

	// 1) 首次建立 → 新建；再次建立（同 host_id + exporter_type）→ 跳过。
	createParams := db.CreateMonitorTargetIfAbsentParams{
		CreateTime: now, UpdateTime: now, HostID: hostID, ExporterType: "node_exporter", ScrapePort: 9100,
	}
	targetID, created, err := createMonitorTargetIfAbsent(ctx, queries, createParams)
	if err != nil || !created || targetID <= 0 {
		t.Fatalf("首次建立目标：id=%d created=%v err=%v", targetID, created, err)
	}
	if _, created, err = createMonitorTargetIfAbsent(ctx, queries, createParams); err != nil || created {
		t.Fatalf("重复建立应跳过：created=%v err=%v", created, err)
	}

	// 2) PATCH 式的整行更新（应用层合并后的写入）与状态读取。
	if err = queries.UpdateMonitorTargetPatch(ctx, db.UpdateMonitorTargetPatchParams{
		ExporterType: "node_exporter", ScrapePort: 9101, ManagedEnabled: true,
		Labels: []byte(`{"env":"smoke"}`), Remark: sql.NullString{String: "冒烟", Valid: true},
		UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("更新目标：%v", err)
	}
	state, err := queries.GetMonitorTargetState(ctx, targetID)
	if err != nil || !state.ManagedEnabled || state.InstallStatus != "unknown" {
		t.Fatalf("目标状态：%+v err=%v", state, err)
	}

	// 3) 宿主总览的 CASE 过滤（不过滤 / 已纳管 / 未纳管 / 日志两态）。
	// 注意：搜索/分组过滤的"不过滤"必须传空串而不是 NULL（`? = ''` 遇到 NULL 会让整条 AND 变 NULL）。
	for name, filter := range map[string]db.CountMonitorHostsParams{
		"不过滤":   {SearchPattern: sql.NullString{String: "", Valid: true}, GroupFilter: ""},
		"指定已纳管": {SearchPattern: sql.NullString{String: "", Valid: true}, GroupFilter: "", ExporterType: "node_exporter", ManagedFilter: "true"},
		"指定未纳管": {SearchPattern: sql.NullString{String: "", Valid: true}, GroupFilter: "", ExporterType: "node_exporter", ManagedFilter: "false"},
		"按名字收窄": {SearchPattern: sql.NullString{String: "", Valid: true}, GroupFilter: "", ExporterType: "node_exporter"},
		"日志未纳管": {SearchPattern: sql.NullString{String: "", Valid: true}, GroupFilter: "", FluentFilter: "false"},
		"日志已纳管": {SearchPattern: sql.NullString{String: "", Valid: true}, GroupFilter: "", FluentFilter: "true"},
	} {
		if _, err = queries.CountMonitorHosts(ctx, filter); err != nil {
			t.Fatalf("宿主过滤 %s：%v", name, err)
		}
	}
	// 刚建的主机在"纳管 node_exporter"的过滤下应命中，在"未纳管"下应不命中。
	managed, err := queries.CountMonitorHosts(ctx, db.CountMonitorHostsParams{
		SearchPattern: sql.NullString{String: "%smoke-monitor-" + suffix + "%", Valid: true},
		GroupFilter:   "", ExporterType: "node_exporter", ManagedFilter: "true",
	})
	if err != nil || managed != 1 {
		t.Fatalf("按 exporter 过滤已纳管 = %d, %v，期望 1", managed, err)
	}
	if unmanaged, err := queries.CountMonitorHosts(ctx, db.CountMonitorHostsParams{
		SearchPattern: sql.NullString{String: "%smoke-monitor-" + suffix + "%", Valid: true},
		GroupFilter:   "", ExporterType: "node_exporter", ManagedFilter: "false",
	}); err != nil || unmanaged != 0 {
		t.Fatalf("未纳管过滤 = %d, %v，期望 0", unmanaged, err)
	}
	hostRows, err := queries.ListMonitorHosts(ctx, db.ListMonitorHostsParams{
		SearchPattern: sql.NullString{String: "%smoke-monitor-" + suffix + "%", Valid: true},
		GroupFilter:   "", Limit: 10, Offset: 0,
	})
	if err != nil || len(hostRows) != 1 {
		t.Fatalf("宿主列表 = %d 行, %v，期望 1", len(hostRows), err)
	}
	if hostRows[0].InstanceName.String == "" || hostRows[0].LogTargetID.Valid {
		t.Fatalf("宿主行字段不符：%+v", hostRows[0])
	}

	// 4) 安装历史：建 → 读最新 → 收尾（时长由应用层写入）→ 取消。
	historyID, err := queries.CreateTargetInstallHistory(ctx, db.CreateTargetInstallHistoryParams{
		CreateTime: now, UpdateTime: now, Action: "install",
		HostIDSnapshot: sql.NullInt32{Int32: int32(hostID), Valid: true}, HostNameSnapshot: "smoke",
		HostIpSnapshot: "10.255.255.203", ExporterTypeSnapshot: "node_exporter", SummaryMessage: "已下发安装任务",
		RequestedUsernameSnapshot: "system", StartTime: sql.NullTime{Time: now, Valid: true},
		HostID: sql.NullInt64{Int64: hostID, Valid: true}, TargetID: sql.NullInt64{Int64: targetID, Valid: true},
	})
	if err != nil {
		t.Fatalf("建安装历史：%v", err)
	}
	if latest, err := queries.GetLatestTargetInstallHistory(ctx, sql.NullInt64{Int64: targetID, Valid: true}); err != nil || latest.ID != historyID {
		t.Fatalf("最新历史 = %+v, %v", latest, err)
	}
	if affected, err := queries.FinishTargetInstallHistory(ctx, db.FinishTargetInstallHistoryParams{
		Status: "success", SummaryMessage: "", EndTime: sql.NullTime{Time: time.Now().UTC(), Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: 1.25, Valid: true},
		UpdateTime:      time.Now().UTC(), ID: historyID,
	}); err != nil || affected != 1 {
		t.Fatalf("收尾历史：affected=%d err=%v", affected, err)
	}
	// 已终态的历史再收尾应为 0 行，取消同样应能读到 target_id（用于按类型分派目标状态）。
	if affected, err := queries.FinishTargetInstallHistory(ctx, db.FinishTargetInstallHistoryParams{
		Status: "failed", SummaryMessage: "", EndTime: sql.NullTime{}, DurationSeconds: sql.NullFloat64{},
		UpdateTime: time.Now().UTC(), ID: historyID,
	}); err != nil || affected != 0 {
		t.Fatalf("重复收尾应 0 行：affected=%d err=%v", affected, err)
	}
	locked, err := queries.GetInstallHistoryForUpdate(ctx, historyID)
	if err != nil || !locked.TargetID.Valid || locked.Status != "success" {
		t.Fatalf("FOR UPDATE 读历史：%+v err=%v", locked, err)
	}
	if err = queries.CancelMonitorTargetInstallState(ctx, db.CancelMonitorTargetInstallStateParams{
		UpdateTime: time.Now().UTC(), ID: targetID,
	}); err != nil {
		t.Fatalf("取消目标状态：%v", err)
	}
	if _, err = queries.ListMonitorTargetsByHost(ctx, db.ListMonitorTargetsByHostParams{
		HostID: hostID, ExporterType: sql.NullString{String: "node_exporter", Valid: true},
	}); err != nil {
		t.Fatalf("按主机列目标：%v", err)
	}
	if _, err = queries.GetTargetInstallContext(ctx, targetID); err != nil {
		t.Fatalf("读目标安装上下文：%v", err)
	}
	if _, err = queries.GetTargetServiceContext(ctx, targetID); err != nil {
		t.Fatalf("读目标服务上下文：%v", err)
	}
	// 主机组树与总览统计（全库聚合，只要能在两方言上跑通）。
	if _, err = queries.ListMonitorHostGroupTree(ctx); err != nil {
		t.Fatalf("主机组树：%v", err)
	}
	if _, err = queries.CountMonitorHostTotals(ctx); err != nil {
		t.Fatalf("总览统计：%v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("回滚：%v", err)
	}
	_ = strings.TrimSpace
}
