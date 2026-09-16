package baseline

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// TestSmokeBaselineQueriesAgainstRealDatabase 把 baseline 域的全部 sqlc 查询在**真库**上按业务流程跑一遍。
//
// 为什么需要它：sqlmock（其余用例）只验证调用形态，验证不了驱动层的参数类型与方言行为——
// 实测教训是 `:execlastid` 在 MySQL 上正常、在 PostgreSQL 上必然失败（pgx 不实现 LastInsertId），
// 两个 tag 的 mock 用例全绿也照样漏掉；decimal 列（compliance_rate）两侧都是 string、
// 需要应用层换算，也只有真库能验。**每个包迁 sqlc 后都应在两个方言上各跑一次。**
//
// 用法（DB 里要有 db/schema 对应的表；全程一个事务，最后回滚，不留数据）：
//
//	BASELINE_SMOKE_DSN='root:pwd@tcp(host:3306)/db?parseTime=true&loc=UTC' go test ./internal/baseline/ -run Smoke -v
//	BASELINE_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/baseline/ -run Smoke -v
func TestSmokeBaselineQueriesAgainstRealDatabase(t *testing.T) {
	dsn := os.Getenv("BASELINE_SMOKE_DSN")
	if dsn == "" {
		t.Skip("BASELINE_SMOKE_DSN 未设置：跳过真库冒烟（两个方言各跑一次的说明见本函数注释）")
	}
	ctx := context.Background()
	connection := smokeConnect(t, ctx, dsn)
	defer connection.Close()
	tx, err := connection.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer tx.Rollback()

	queries := db.New(tx)
	now := time.Now().UTC()
	suffix := now.Format("150405.000000")
	var baselineID, categoryID, itemID, scanID, targetRowID int64

	// step 每条语句套一个 SAVEPOINT：失败后能继续跑完（PG 里一条语句失败就会毁掉整个事务），
	// 一次就能看全所有问题。
	step := func(name string, run func() error) {
		t.Helper()
		if _, err := tx.ExecContext(ctx, "SAVEPOINT s"); err != nil {
			t.Fatalf("savepoint: %v", err)
		}
		if err := run(); err != nil {
			t.Errorf("%s: %v", name, err)
			tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT s")
			return
		}
		tx.ExecContext(ctx, "RELEASE SAVEPOINT s")
	}

	step("CreateBaseline", func() error {
		id, err := queries.CreateBaseline(ctx, db.CreateBaselineParams{CreateTime: now, UpdateTime: now,
			Name: "smoke-" + suffix, Version: "v1", Description: "冒烟", Enabled: true})
		baselineID = id
		if err == nil && id == 0 {
			return fmt.Errorf("自增主键未取回（PG 侧 :execlastid 未派生为 RETURNING 时的典型症状）")
		}
		return err
	})
	step("MaxBaselineCategorySort", func() error {
		max, err := queries.MaxBaselineCategorySort(ctx, baselineID)
		if err != nil {
			return err
		}
		if max != -1 {
			return fmt.Errorf("空基线应得 -1，实得 %d", max)
		}
		return nil
	})
	step("CreateBaselineCategory", func() error {
		id, err := queries.CreateBaselineCategory(ctx, db.CreateBaselineCategoryParams{
			CreateTime: now, UpdateTime: now, Name: "类目A", Sort: 0, BaselineID: baselineID})
		categoryID = id
		return err
	})
	step("MaxBaselineItemSort", func() error {
		max, err := queries.MaxBaselineItemSort(ctx, categoryID)
		if err != nil {
			return err
		}
		if max != -1 {
			return fmt.Errorf("空类目应得 -1，实得 %d", max)
		}
		return nil
	})
	step("CreateBaselineItem", func() error {
		id, err := queries.CreateBaselineItem(ctx, db.CreateBaselineItemParams{
			CreateTime: now, UpdateTime: now, BaselineID: baselineID, CategoryID: categoryID, Sort: 0,
			Name: "密码长度", Description: "至少 8 位", Config: json.RawMessage(`{"policy":"x"}`), Severity: "high"})
		itemID = id
		return err
	})
	step("CountBaselines(NULL 表示不过滤)", func() error {
		count, err := queries.CountBaselines(ctx, db.CountBaselinesParams{})
		if err != nil {
			return err
		}
		if count < 1 {
			return fmt.Errorf("不过滤应至少命中刚建的 1 条，实得 %d", count)
		}
		return nil
	})
	step("CountBaselines(带 pattern)", func() error {
		count, err := queries.CountBaselines(ctx, db.CountBaselinesParams{
			Pattern: sql.NullString{String: "%smoke-" + suffix + "%", Valid: true}})
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("按名字精确搜应命中 1 条，实得 %d", count)
		}
		return nil
	})
	step("ListBaselines(聚合列)", func() error {
		rows, err := queries.ListBaselines(ctx, db.ListBaselinesParams{Pattern: sql.NullString{}, Limit: 100, Offset: 0})
		if err != nil {
			return err
		}
		for _, row := range rows {
			if row.ID == baselineID {
				if row.ItemCount != 1 || row.ScanCount != 0 {
					return fmt.Errorf("聚合列不符：item_count=%d scan_count=%d", row.ItemCount, row.ScanCount)
				}
				return nil
			}
		}
		return fmt.Errorf("列表里找不到刚建的基线")
	})
	step("GetBaseline", func() error {
		row, err := queries.GetBaseline(ctx, baselineID)
		if err != nil {
			return err
		}
		if row.Name != "smoke-"+suffix || !row.Enabled {
			return fmt.Errorf("行内容不符: %+v", row)
		}
		return nil
	})
	step("ListBaselineCategories / GetBaselineCategory", func() error {
		rows, err := queries.ListBaselineCategories(ctx, baselineID)
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			return fmt.Errorf("类目数应为 1，实得 %d", len(rows))
		}
		row, err := queries.GetBaselineCategory(ctx, categoryID)
		if err != nil {
			return err
		}
		if row.BaselineID != baselineID {
			return fmt.Errorf("baseline_id 不符: %+v", row)
		}
		return nil
	})
	step("ListBaselineItems(带类目名)", func() error {
		rows, err := queries.ListBaselineItems(ctx, baselineID)
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0].Category != "类目A" {
			return fmt.Errorf("条目/类目名不符: %+v", rows)
		}
		return nil
	})
	step("GetBaselineItem / UpdateBaselineItem", func() error {
		row, err := queries.GetBaselineItem(ctx, itemID)
		if err != nil {
			return err
		}
		if row.BaselineID != baselineID {
			return fmt.Errorf("baseline_id 不符: %+v", row)
		}
		_, err = queries.UpdateBaselineItem(ctx, db.UpdateBaselineItemParams{CategoryID: categoryID, Name: "密码长度v2",
			Description: "至少 12 位", Config: json.RawMessage(`{"policy":"y"}`), Severity: "medium", UpdateTime: now, ID: itemID})
		return err
	})
	step("UpdateBaseline / GetBaselineForScan / CountBaselineItems", func() error {
		if err := queries.UpdateBaseline(ctx, db.UpdateBaselineParams{UpdateTime: now, Name: "smoke-" + suffix,
			Version: "v2", Description: "改过", Enabled: true, ID: baselineID}); err != nil {
			return err
		}
		if _, err := queries.GetBaselineForScan(ctx, baselineID); err != nil {
			return err
		}
		count, err := queries.CountBaselineItems(ctx, baselineID)
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("条目数应为 1，实得 %d", count)
		}
		return nil
	})
	step("RenameBaselineCategory / UpdateBaselineCategorySort", func() error {
		if _, err := queries.RenameBaselineCategory(ctx, db.RenameBaselineCategoryParams{
			Name: "类目A2", UpdateTime: now, ID: categoryID}); err != nil {
			return err
		}
		_, err := queries.UpdateBaselineCategorySort(ctx, db.UpdateBaselineCategorySortParams{Sort: 5, UpdateTime: now, ID: categoryID})
		return err
	})
	step("CreateBaselineScan", func() error {
		id, err := queries.CreateBaselineScan(ctx, db.CreateBaselineScanParams{CreateTime: now, UpdateTime: now,
			BaselineID: baselineID, MountType: "project", ProjectID: sql.NullInt64{Int64: 1, Valid: true},
			RequestedUsername: "smoke"})
		scanID = id
		return err
	})
	step("CreateBaselineScanTargets / GetBaselineScanTarget", func() error {
		if err := queries.CreateBaselineScanTargets(ctx, db.CreateBaselineScanTargetsParams{
			ScanID: scanID, HostID: 1, HostName: "node-1", HostIp: "10.0.0.5", InstanceNameSnapshot: "node-1"}); err != nil {
			return err
		}
		row, err := queries.GetBaselineScanTarget(ctx, db.GetBaselineScanTargetParams{ScanID: scanID, HostID: 1})
		if err != nil {
			return err
		}
		targetRowID = row.ID
		if row.Status != "pending" {
			return fmt.Errorf("初始状态应为 pending，实得 %q", row.Status)
		}
		return nil
	})
	step("ClaimBaselineScan(只抢占 pending)", func() error {
		affected, err := queries.ClaimBaselineScan(ctx, db.ClaimBaselineScanParams{
			StartTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: scanID})
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("应抢占 1 行，实得 %d", affected)
		}
		second, err := queries.ClaimBaselineScan(ctx, db.ClaimBaselineScanParams{
			StartTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: scanID})
		if err != nil {
			return err
		}
		if second != 0 {
			return fmt.Errorf("第二次抢占应命中 0 行（已 running），实得 %d", second)
		}
		return nil
	})
	step("SetBaselineScanTargetStatus / FinishBaselineScanTarget", func() error {
		if err := queries.SetBaselineScanTargetStatus(ctx, db.SetBaselineScanTargetStatusParams{
			Status: "running", ErrorMessage: "冒烟", ID: targetRowID}); err != nil {
			return err
		}
		return queries.FinishBaselineScanTarget(ctx, db.FinishBaselineScanTargetParams{
			Status: "failed", PassedItems: 2, FailedItems: 1, ComplianceRate: decimalString(66.666666), ErrorMessage: "", ID: targetRowID})
	})
	step("CreateBaselineScanResults(含 NULL remediation)", func() error {
		if err := queries.CreateBaselineScanResults(ctx, db.CreateBaselineScanResultsParams{
			ScanID: scanID, HostID: 1, ItemID: itemID, ItemName: "密码长度", Chapter: "身份鉴别", Severity: "high",
			Status: "fail", ExpectedValue: json.RawMessage(`{"min":8}`), ActualValue: json.RawMessage(`{"actual":4}`),
			Message: "不满足", Remediation: sql.NullString{String: "改 /etc/login.defs", Valid: true}}); err != nil {
			return err
		}
		return queries.CreateBaselineScanResults(ctx, db.CreateBaselineScanResultsParams{
			ScanID: scanID, HostID: 1, ItemID: itemID, ItemName: "空 remediation", Chapter: "身份鉴别", Severity: "low",
			Status: "pass", ExpectedValue: json.RawMessage(`null`), ActualValue: json.RawMessage(`null`), Message: ""})
	})
	step("CountScanTargetsByStatus", func() error {
		counts, err := queries.CountScanTargetsByStatus(ctx, scanID)
		if err != nil {
			return err
		}
		if counts.Failed != 1 || counts.Success != 0 || counts.Skipped != 0 {
			return fmt.Errorf("计数不符: %+v", counts)
		}
		return nil
	})
	step("CountScansByType / ListScans / GetScanHeader", func() error {
		if _, err := queries.CountScansByType(ctx, "baseline"); err != nil {
			return err
		}
		rows, err := queries.ListScans(ctx, db.ListScansParams{ScanType: "baseline", Limit: 100, Offset: 0})
		if err != nil {
			return err
		}
		found := false
		for _, row := range rows {
			if row.ID == scanID {
				found = row.Baseline == "smoke-"+suffix
			}
		}
		if !found {
			return fmt.Errorf("列表里找不到刚建的扫描或 baseline 名不对")
		}
		header, err := queries.GetScanHeader(ctx, scanID)
		if err != nil {
			return err
		}
		if header.BaselineID != baselineID || len(header.Summary) == 0 {
			return fmt.Errorf("扫描头不符: %+v", header)
		}
		return nil
	})
	step("ListScanTargets(decimal 文本)", func() error {
		rows, err := queries.ListScanTargets(ctx, scanID)
		if err != nil {
			return err
		}
		if len(rows) != 1 || decimalValue(rows[0].ComplianceRate) != 66.67 {
			return fmt.Errorf("目标行不符（compliance_rate 应为 66.67）: %+v", rows)
		}
		return nil
	})
	step("ListBaselineScanResults(fail 排前)", func() error {
		rows, err := queries.ListBaselineScanResults(ctx, scanID)
		if err != nil {
			return err
		}
		if len(rows) != 2 || rows[0].Status != "fail" {
			return fmt.Errorf("明细应为 2 条且 fail 在前: %+v", rows)
		}
		return nil
	})
	step("GetScanStatusForUpdate / CancelBaselineScan / CancelBaselineScanTargets", func() error {
		row, err := queries.GetScanStatusForUpdate(ctx, scanID)
		if err != nil {
			return err
		}
		if row.Status != "running" {
			return fmt.Errorf("状态应为 running，实得 %q", row.Status)
		}
		affected, err := queries.CancelBaselineScan(ctx, db.CancelBaselineScanParams{
			Summary: mergeCanceledSummary(row.Summary), EndTime: sql.NullTime{Time: now, Valid: true},
			UpdateTime: now, ID: scanID})
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("取消应更新 1 行，实得 %d", affected)
		}
		return queries.CancelBaselineScanTargets(ctx, scanID)
	})
	step("FinishBaselineScan", func() error {
		return queries.FinishBaselineScan(ctx, db.FinishBaselineScanParams{Status: "success",
			Summary: json.RawMessage(`{"total":1}`), EndTime: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: scanID})
	})
	step("CountBaselineItemsByCategory / DeleteBaselineItem(带基线归属)", func() error {
		count, err := queries.CountBaselineItemsByCategory(ctx, categoryID)
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("类目下条目数应为 1，实得 %d", count)
		}
		// 跨基线的 id 必须删不掉（row 受影响为 0）。
		foreign, err := queries.DeleteBaselineItem(ctx, db.DeleteBaselineItemParams{ID: itemID, BaselineID: baselineID + 1})
		if err != nil {
			return err
		}
		if foreign != 0 {
			return fmt.Errorf("跨基线删除应命中 0 行，实得 %d", foreign)
		}
		affected, err := queries.DeleteBaselineItem(ctx, db.DeleteBaselineItemParams{ID: itemID, BaselineID: baselineID})
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("应删 1 行，实得 %d", affected)
		}
		return nil
	})
	step("级联删除（明细→目标→记录→条目→类目→基线）", func() error {
		if err := queries.DeleteBaselineScanResults(ctx, baselineID); err != nil {
			return err
		}
		if err := queries.DeleteBaselineScanTargets(ctx, baselineID); err != nil {
			return err
		}
		if err := queries.DeleteBaselineScans(ctx, baselineID); err != nil {
			return err
		}
		if err := queries.DeleteBaselineItems(ctx, baselineID); err != nil {
			return err
		}
		if _, err := queries.DeleteBaselineCategory(ctx, categoryID); err != nil {
			return err
		}
		affected, err := queries.DeleteBaseline(ctx, baselineID)
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("应删 1 行，实得 %d", affected)
		}
		return nil
	})

	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if t.Failed() {
		t.Fatalf("冒烟失败：DSN=%s", redactDSN(dsn))
	}
}

// smokeConnect 按当前构建标签连库：不带 tag 连 MySQL、-tags postgres 连 PostgreSQL。
func smokeConnect(t *testing.T, ctx context.Context, dsn string) *sql.DB {
	t.Helper()
	connection, err := openSmokeDatabase(ctx, dsn)
	if err != nil {
		t.Fatalf("connect(%s): %v", redactDSN(dsn), err)
	}
	return connection
}

// redactDSN 只保留主机与库名，避免把口令写进测试日志。
func redactDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return "***" + dsn[at:]
	}
	return dsn
}
