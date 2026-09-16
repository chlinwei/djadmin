package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// TestSmokeHostDomainQueriesAgainstRealDatabase 把主机域迁到 sqlc 的语句在**真库**上跑一遍
// （建主机 → 采集结果落库（3 个 UPSERT）→ 详情读取 → 磁盘整表重建 → 采集状态两态 → 身份唯一性计数）。
//
// 为什么需要它：这组查询里有几处 mock 验不了的东西——
//   - **UPSERT**：MySQL 侧是 `ON DUPLICATE KEY UPDATE`，PG 侧由派生脚本改写成
//     `ON CONFLICT (host_id) DO UPDATE SET …`（冲突目标来自源里的 `-- conflict:` 注释）。
//     改写是否真的落在 host_id 上、第二次写是否走 UPDATE 分支，只有真库能验；
//   - **可空列**：采集快照里 os_type/cpu_cores/disk 各列大量可空，写入 NULL 与读出 NULL 的往返；
//   - `collect_time = COALESCE(?, collect_time)`：失败时保留上次采集时间的语义。
//
// 全程一个事务 + 回滚（这些语句都不自带事务），不往库里留数据，也不需要库里预先有主机。
//
// 用法：
//
//	ASSETS_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/assets/ -run RealDatabase -v
//	ASSETS_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/assets/ -run RealDatabase -v
func TestSmokeHostDomainQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("ASSETS_SMOKE_DSN")
	if dsn == "" {
		t.Skip("ASSETS_SMOKE_DSN 未设置：跳过真库冒烟（两个方言各跑一次的说明见本函数注释）")
	}
	ctx := context.Background()
	pool, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("connect(%s): %v", redactSmokeDSN(dsn), err)
	}
	defer pool.Close()
	tx, err := pool.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()
	queries, now := db.New(tx), time.Now().UTC()
	suffix := now.Format("150405.000000")

	// 建主机（instance_name 有唯一键，用冒烟专用名）。
	created, err := queries.CreateHost(ctx, db.CreateHostParams{
		CreateTime: now, UpdateTime: now, Status: "active", IsDeletedInCloud: false,
		InstanceName:  sql.NullString{String: "smoke-host-" + suffix, Valid: true},
		Ip:            sql.NullString{String: "10.255.255.201", Valid: true},
		CollectStatus: "pending", CollectMessage: "", AgentOnline: false,
		WebsshDefaultUsername: "", WebsshLoginUsers: "",
	})
	if err != nil {
		t.Fatalf("建主机：%v", err)
	}
	hostID, err := created.LastInsertId()
	if err != nil {
		t.Fatalf("取主机主键（PG 侧说明 INSERT 未派生为 :one + RETURNING）：%v", err)
	}

	// 采集结果落库：先插入（无既有行），再写第二次验证 UPSERT 走 UPDATE 分支。
	if _, err = queries.GetHostRuntimeFingerprint(ctx, hostID); err != sql.ErrNoRows {
		t.Fatalf("未采集时读指纹应 ErrNoRows，实得 %v", err)
	}
	upsertRuntime := func(fingerprint string, cpuUsage float64) {
		t.Helper()
		if err = queries.UpsertHostRuntime(ctx, db.UpsertHostRuntimeParams{
			CreateTime: now, UpdateTime: time.Now().UTC(), HostID: hostID,
			CpuUsagePercent:       sql.NullFloat64{Float64: cpuUsage, Valid: true},
			CpuTimes:              json.RawMessage(`{"user":1}`),
			MemoryUsagePercent:    sql.NullFloat64{Float64: 20.5, Valid: true},
			Memory:                json.RawMessage(`{"total":1024}`),
			DiskIo:                json.RawMessage(`[]`),
			OsUptimeSeconds:       sql.NullInt64{Int64: 3600, Valid: true},
			OsBootTime:            sql.NullTime{Time: now, Valid: true},
			MetricsSampleWindowMs: sql.NullInt32{Int32: 15000, Valid: true},
			StaticFingerprint:     fingerprint,
			CollectedAt:           sql.NullTime{Time: now, Valid: true},
		}); err != nil {
			t.Fatalf("UPSERT hostruntime(%s)：%v", fingerprint, err)
		}
	}
	upsertRuntime("fp-1", 10)
	if fingerprint, err := queries.GetHostRuntimeFingerprint(ctx, hostID); err != nil || fingerprint != "fp-1" {
		t.Fatalf("指纹 = %q, %v，期望 fp-1", fingerprint, err)
	}
	upsertRuntime("fp-2", 42)
	fingerprint, err := queries.GetHostRuntimeFingerprint(ctx, hostID)
	if err != nil || fingerprint != "fp-2" {
		t.Fatalf("二次写入后指纹 = %q, %v，期望 fp-2（UPSERT 应走 UPDATE 分支）", fingerprint, err)
	}
	runtime, err := queries.GetHostRuntime(ctx, hostID)
	if err != nil {
		t.Fatalf("读 runtime：%v", err)
	}
	if !runtime.CpuUsagePercent.Valid || runtime.CpuUsagePercent.Float64 != 42 || runtime.MetricsSampleWindowMs.Int32 != 15000 {
		t.Fatalf("runtime 未按第二次写入更新：%+v", runtime)
	}

	// 系统与硬件快照：同样两态。
	upsertSystem := func(hostname string) {
		t.Helper()
		if err = queries.UpsertHostSystem(ctx, db.UpsertHostSystemParams{
			CreateTime: now, UpdateTime: time.Now().UTC(), HostID: hostID,
			OsType:    sql.NullString{String: "linux", Valid: true},
			OsVersion: sql.NullString{String: "24.04", Valid: true},
			OsID:      sql.NullString{String: "ubuntu", Valid: true},
			OsIDLike:  sql.NullString{String: "debian", Valid: true},
			Hostname:  sql.NullString{String: hostname, Valid: true},
			// 其余列留 NULL：验证可空列的写入路径。
			CollectorSource: sql.NullString{String: "agent", Valid: true},
			CollectedAt:     sql.NullTime{Time: now, Valid: true},
		}); err != nil {
			t.Fatalf("UPSERT hostsystem(%s)：%v", hostname, err)
		}
	}
	upsertSystem("smoke-a")
	upsertSystem("smoke-b")
	system, err := queries.GetHostSystem(ctx, hostID)
	if err != nil {
		t.Fatalf("读 system：%v", err)
	}
	if system.Hostname.String != "smoke-b" || !system.OsType.Valid || system.KernelVersion.Valid {
		t.Fatalf("system 未按第二次写入更新或可空列往返有误：%+v", system)
	}

	upsertHardware := func(cores int32) {
		t.Helper()
		if err = queries.UpsertHostHardware(ctx, db.UpsertHostHardwareParams{
			CreateTime: now, UpdateTime: time.Now().UTC(), HostID: hostID,
			CpuCores: sql.NullInt32{Int32: cores, Valid: true},
			CpuModel: sql.NullString{String: "smoke-cpu", Valid: true},
			MemoryGb: sql.NullFloat64{Float64: 16, Valid: true},
			// disk_total_gb / architecture 留 NULL。
			CollectedAt: sql.NullTime{Time: now, Valid: true},
		}); err != nil {
			t.Fatalf("UPSERT hosthardware(%d)：%v", cores, err)
		}
	}
	upsertHardware(4)
	upsertHardware(8)
	hardware, err := queries.GetHostHardware(ctx, hostID)
	if err != nil {
		t.Fatalf("读 hardware：%v", err)
	}
	if hardware.CpuCores.Int32 != 8 || hardware.DiskTotalGb.Valid || hardware.Architecture.Valid {
		t.Fatalf("hardware 未按第二次写入更新：%+v", hardware)
	}

	// 磁盘：整表重建（先删后插），列表顺序按 id。
	if err = queries.CreateHostDisk(ctx, db.CreateHostDiskParams{
		HostID: hostID, Device: "/dev/sda1", MountPoint: sql.NullString{String: "/", Valid: true},
		SizeGb: sql.NullFloat64{Float64: 100, Valid: true}, UsedGb: sql.NullFloat64{Float64: 40, Valid: true},
		Filesystem: sql.NullString{String: "ext4", Valid: true},
	}); err != nil {
		t.Fatalf("写磁盘：%v", err)
	}
	if err = queries.CreateHostDisk(ctx, db.CreateHostDiskParams{
		HostID: hostID, Device: "/dev/sdb1", // mount_point / size / used / filesystem 留 NULL
	}); err != nil {
		t.Fatalf("写磁盘（可空列为 NULL）：%v", err)
	}
	disks, err := queries.ListHostDisks(ctx, hostID)
	if err != nil || len(disks) != 2 {
		t.Fatalf("磁盘 = %d 行, %v，期望 2", len(disks), err)
	}
	if disks[0].Device != "/dev/sda1" || !disks[0].SizeGb.Valid || disks[1].SizeGb.Valid {
		t.Fatalf("磁盘字段不符：%+v", disks)
	}
	if err = queries.DeleteHostDisks(ctx, hostID); err != nil {
		t.Fatalf("清空磁盘：%v", err)
	}
	if disks, err = queries.ListHostDisks(ctx, hostID); err != nil || len(disks) != 0 {
		t.Fatalf("清空后磁盘 = %d 行, %v", len(disks), err)
	}

	// 采集状态两态：成功写 collect_time，失败保留上次的值（COALESCE 分支）。
	if err = queries.MarkHostCollected(ctx, db.MarkHostCollectedParams{
		CollectStatus: "success", CollectMessage: "", CollectTime: sql.NullTime{Time: now, Valid: true},
		UpdateTime: now, ID: hostID,
	}); err != nil {
		t.Fatalf("采集成功置位：%v", err)
	}
	host, err := queries.GetHost(ctx, hostID)
	if err != nil {
		t.Fatalf("读主机：%v", err)
	}
	if host.CollectStatus != "success" || !host.CollectTime.Valid {
		t.Fatalf("采集成功未落库：%+v", host)
	}
	if err = queries.MarkHostCollected(ctx, db.MarkHostCollectedParams{
		CollectStatus: "failed", CollectMessage: "冒烟失败", CollectTime: sql.NullTime{},
		UpdateTime: now, ID: hostID,
	}); err != nil {
		t.Fatalf("采集失败置位：%v", err)
	}
	host, err = queries.GetHost(ctx, hostID)
	if err != nil {
		t.Fatalf("读主机（失败态）：%v", err)
	}
	if host.CollectStatus != "failed" || host.CollectMessage != "冒烟失败" || !host.CollectTime.Valid {
		t.Fatalf("失败态应保留上次 collect_time：%+v", host)
	}

	// 身份唯一性计数：排除自己应为 0，不排除应为 1。
	if count, err := queries.CountOtherHostsByInstanceName(ctx, db.CountOtherHostsByInstanceNameParams{
		InstanceName: host.InstanceName, ExcludeID: hostID,
	}); err != nil || count != 0 {
		t.Fatalf("排除自己后实例名计数 = %d, %v，期望 0", count, err)
	}
	if count, err := queries.CountOtherHostsByInstanceName(ctx, db.CountOtherHostsByInstanceNameParams{
		InstanceName: host.InstanceName, ExcludeID: 0,
	}); err != nil || count != 1 {
		t.Fatalf("实例名计数 = %d, %v，期望 1", count, err)
	}
	if count, err := queries.CountOtherHostsByIP(ctx, db.CountOtherHostsByIPParams{
		Ip: host.Ip, ExcludeID: hostID,
	}); err != nil || count != 0 {
		t.Fatalf("排除自己后 IP 计数 = %d, %v，期望 0", count, err)
	}

	// 主机监控目标与不存在的 host_id：只验语句在两方言上都可执行。
	if _, err = queries.ListHostMonitors(ctx, hostID); err != nil {
		t.Fatalf("读主机监控目标：%v", err)
	}
	if _, err = queries.GetHostSystem(ctx, hostID+1_000_000); err != sql.ErrNoRows {
		t.Fatalf("不存在主机的 system 应 ErrNoRows，实得 %v", err)
	}
	if _, err = queries.GetHostRuntime(ctx, hostID+1_000_000); err != sql.ErrNoRows {
		t.Fatalf("不存在主机的 runtime 应 ErrNoRows，实得 %v", err)
	}
	if _, err = queries.GetHostHardware(ctx, hostID+1_000_000); err != sql.ErrNoRows {
		t.Fatalf("不存在主机的 hardware 应 ErrNoRows，实得 %v", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatalf("回滚：%v", err)
	}
	// 回滚后不应留下冒烟主机（用池上的新查询集，事务已结束）。
	if count, err := db.New(pool).CountOtherHostsByInstanceName(ctx, db.CountOtherHostsByInstanceNameParams{
		InstanceName: sql.NullString{String: "smoke-host-" + suffix, Valid: true}, ExcludeID: 0,
	}); err != nil || count != 0 {
		t.Fatalf("回滚后仍能查到冒烟主机：count=%d err=%v", count, err)
	}
}

// redactSmokeDSN 只保留主机与库名，避免把口令写进测试日志。
func redactSmokeDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return "***" + dsn[at:]
	}
	return dsn
}
