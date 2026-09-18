package logcollect

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"autoadmin/internal/job"
	db "autoadmin/internal/platform/database/generated"
)

// TestSmokeLogBatchJobAgainstRealDatabase 把批量作业执行器的整套语句在**真库**上跑一遍流程。
// mock 用例全绿不等于方言可用；这条流程里有几处只有真库能暴露的坑：
//
//  1. `ClaimLogBatchJob` 的"是否抢到"依赖 UPDATE 的影响行数 —— MySQL 默认返回**实际改变的行数**
//     （未开 CLIENT_FOUND_ROWS），把已经是 running 的行再写一遍同样值会返回 0 行。
//     早期版本用 `status IN ('pending','running')` 做认领，真库上分片续跑因此恒被误判成"抢不到"
//     （作业永久停在 running）。现在认领只认 pending，续跑靠先读状态，这里断言这个语义。
//  2. `RefreshLogBatchJobProgress` 是带相关子查询的 UPDATE（两侧都要真跑过）。
//  3. 失联对账的整条链路：心跳过期 → 回落作业与明细 → 重投消息。
//
// 造的是**自己的行**（作业与明细），不碰任何主机：item 的 target_id 故意用不存在的 id，
// 因此执行路径不会调用 agent。收尾按 id 精确删除自己造的行（仓库纪律，见 SQL_DESIGN §6.3）。
//
// 用法：
//
//	MONITOR_SMOKE_DSN='root:pwd@tcp(host:3306)/djadmin?parseTime=true&loc=UTC' go test ./internal/logcollect/ -run LogBatchJob -v
//	MONITOR_SMOKE_DSN='postgres://user@host:5432/db?sslmode=disable&TimeZone=UTC' go test -tags postgres ./internal/logcollect/ -run LogBatchJob -v
func TestSmokeLogBatchJobAgainstRealDatabase(t *testing.T) {
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

	queries := db.New(pool)
	now := time.Now().UTC()
	jobID, err := queries.CreateLogBatchJob(ctx, db.CreateLogBatchJobParams{
		CreateTime: now, UpdateTime: now, Action: LogBatchActionApply, TotalCount: 2,
		Concurrency: 1, RequestedUsername: "smoke",
	})
	if err != nil {
		t.Fatalf("建作业：%v", err)
	}
	defer func() {
		// 按 id 精确删除自己造的行。
		if _, err := pool.ExecContext(context.Background(), "DELETE FROM monitor_log_batch_job_item WHERE batch_job_id = ?", jobID); err != nil {
			t.Errorf("清理明细失败：%v", err)
		}
		if _, err := pool.ExecContext(context.Background(), "DELETE FROM monitor_log_batch_job WHERE id = ?", jobID); err != nil {
			t.Errorf("清理作业失败：%v", err)
		}
	}()
	// 目标 id 不存在 → 每台都以"日志采集目标已不存在"失败，不会调用 agent。
	for index := int64(1); index <= 2; index++ {
		if err := queries.CreateLogBatchJobItem(ctx, db.CreateLogBatchJobItemParams{
			CreateTime: now, UpdateTime: now, BatchJobID: jobID,
			TargetID: 900000000 + index, HostID: index, HostName: "smoke-batch", HostIp: "10.255.255.253",
		}); err != nil {
			t.Fatalf("建明细：%v", err)
		}
	}

	publisher := &fakeLogBatchPublisher{}
	runner := newLogBatchRunner(t, pool, publisher, time.Hour)
	if err := runner.Handle(ctx, job.Message{
		SchemaVersion: job.SchemaVersion, Kind: LogBatchKind, ResourceID: jobID,
	}); err != nil {
		t.Fatalf("执行批量作业：%v", err)
	}

	overview, err := queries.GetLogBatchJob(ctx, jobID)
	if err != nil {
		t.Fatalf("读作业：%v", err)
	}
	if overview.Status != LogBatchStatusFailed || overview.FailedCount != 2 || overview.SuccessCount != 0 {
		t.Fatalf("终态应为 failed/2 台失败，得到 %s success=%d failed=%d",
			overview.Status, overview.SuccessCount, overview.FailedCount)
	}
	if overview.StartedAt.Valid == false || overview.FinishedAt.Valid == false {
		t.Fatalf("开始/结束时间应落库，得到 started=%v finished=%v", overview.StartedAt, overview.FinishedAt)
	}
	items, err := queries.ListLogBatchJobItems(ctx, jobID)
	if err != nil {
		t.Fatalf("读明细：%v", err)
	}
	for _, item := range items {
		if item.Status != logBatchItemFailed || item.Message == "" {
			t.Fatalf("明细应落 failed 且带原因，得到 %+v", item)
		}
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("整批跑完不该重投，得到 %d 条", len(publisher.messages))
	}

	// 认领语义：作业已是终态时再投一次（消息重投）必须静默返回，不碰任何明细。
	if err := runner.run(ctx, jobID); err != nil {
		t.Fatalf("对已结束作业再跑一次应静默返回：%v", err)
	}

	// 失联对账：造一个心跳过期的 running 作业，断言被回落 + 重投。
	staleID, err := queries.CreateLogBatchJob(ctx, db.CreateLogBatchJobParams{
		CreateTime: now, UpdateTime: now, Action: LogBatchActionInstall, TotalCount: 1,
		Concurrency: 1, RequestedUsername: "smoke",
	})
	if err != nil {
		t.Fatalf("建失联作业：%v", err)
	}
	defer func() {
		if _, err := pool.ExecContext(context.Background(), "DELETE FROM monitor_log_batch_job_item WHERE batch_job_id = ?", staleID); err != nil {
			t.Errorf("清理失联明细失败：%v", err)
		}
		if _, err := pool.ExecContext(context.Background(), "DELETE FROM monitor_log_batch_job WHERE id = ?", staleID); err != nil {
			t.Errorf("清理失联作业失败：%v", err)
		}
	}()
	if err := queries.CreateLogBatchJobItem(ctx, db.CreateLogBatchJobItemParams{
		CreateTime: now, UpdateTime: now, BatchJobID: staleID,
		TargetID: 900000001, HostID: 1, HostName: "smoke-stale", HostIp: "10.255.255.252",
	}); err != nil {
		t.Fatalf("建失联明细：%v", err)
	}
	claimed, err := queries.ClaimLogBatchJob(ctx, db.ClaimLogBatchJobParams{
		StartedAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: staleID,
	})
	if err != nil || claimed != 1 {
		t.Fatalf("首次认领应成功，得到 claimed=%d err=%v", claimed, err)
	}
	// 已经 running 的行再认领一次拿 0 行（真库的"影响行数"语义，执行器据此走续跑分支）。
	claimedAgain, err := queries.ClaimLogBatchJob(ctx, db.ClaimLogBatchJobParams{
		StartedAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, ID: staleID,
	})
	if err != nil || claimedAgain != 0 {
		t.Fatalf("running 的作业不应被再次认领，得到 claimed=%d err=%v", claimedAgain, err)
	}
	// 把心跳推到过期，明细也置成 running（模拟执行进程中途消失）。
	if err := queries.UpdateLogBatchJobHeartbeat(ctx, db.UpdateLogBatchJobHeartbeatParams{
		UpdateTime: now.Add(-time.Hour), ID: staleID,
	}); err != nil {
		t.Fatalf("回拨心跳：%v", err)
	}
	if _, err := pool.ExecContext(ctx, "UPDATE monitor_log_batch_job_item SET status='running', update_time=? WHERE batch_job_id=?", now.Add(-time.Hour), staleID); err != nil {
		t.Fatalf("置明细 running：%v", err)
	}
	before := len(publisher.messages)
	runner.reap(ctx)
	overview, err = queries.GetLogBatchJob(ctx, staleID)
	if err != nil {
		t.Fatalf("读失联作业：%v", err)
	}
	if overview.Status != LogBatchStatusPending {
		t.Fatalf("失联作业应回落为 pending，得到 %s", overview.Status)
	}
	if len(publisher.messages) != before+1 || publisher.messages[len(publisher.messages)-1].ResourceID != staleID {
		t.Fatalf("失联作业应被重投，得到 %+v", publisher.messages)
	}
	items, err = queries.ListLogBatchJobItems(ctx, staleID)
	if err != nil {
		t.Fatalf("读失联明细：%v", err)
	}
	if items[0].Status != logBatchItemPending {
		t.Fatalf("失联明细应回落为 pending，得到 %s", items[0].Status)
	}
}
