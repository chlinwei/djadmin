package automation

import (
	"context"
	"database/sql"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// 失联作业对账。
//
// 作业的执行超时由**执行进程内的 context** 强制（见 executeLocalAnsible）：一旦执行进程本身
// 消失（被 kill、崩溃、worker 被重启），那个 context 就永远不会触发超时，作业会永久停在
// `running` —— 界面上「查看日志」就会一直显示"等待新输出"，而且没有任何兜底把它收敛。
// 2026-09-18 现场：作业 #831 的执行进程消失后，留下 8 个孤儿 ansible 进程和一个永不结束的作业。
//
// 判定口径：`start_time` 超过"作业自身超时 + 余量"。余量给足是刻意的——正常路径下进程内的
// 超时会先把作业置为 failed，对账只在执行进程确实不在了的情况下命中，不会误伤跑得慢的作业。
const (
	staleJobGrace    = 2 * time.Minute
	staleJobSweepGap = time.Minute
)

// defaultExecutionTimeout 与 executeLocalAnsible 的兜底一致（任务未配置超时按 10 分钟）。
const defaultExecutionTimeout = 10 * time.Minute

// staleJobExpired 判断一个 running 作业是否已越过"自身超时 + 余量"。
func staleJobExpired(start time.Time, timeoutSeconds uint32, now time.Time) bool {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = defaultExecutionTimeout
	}
	return now.Sub(start) > timeout+staleJobGrace
}

// ReapStaleJobs 扫一遍 running 作业，把已失联的置为 failed，返回收敛条数。
//
// 置失败走 FailStaleAutomationJob（带 `status='running'` 守卫）：正常收尾可能同时在写，
// 没有守卫会把刚成功的作业改写成失败。
func ReapStaleJobs(ctx context.Context, database *sql.DB) (int, error) {
	rows, err := db.New(database).ListRunningAutomationJobs(ctx)
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	reaped := 0
	for _, row := range rows {
		if !row.StartTime.Valid || !staleJobExpired(row.StartTime.Time, row.ExecutionTimeoutSeconds, now) {
			continue
		}
		affected, err := db.New(database).FailStaleAutomationJob(ctx, db.FailStaleAutomationJobParams{
			EndTime:         sql.NullTime{Time: now, Valid: true},
			DurationSeconds: sql.NullFloat64{Float64: now.Sub(row.StartTime.Time).Seconds(), Valid: true},
			ResultSummary: marshalJSON(map[string]any{
				"message":        "执行超时或执行进程已失联，已由对账置为失败；该作业的实际输出可能已丢失（执行进程不在时无法回收其输出）",
				"execution_mode": "stale_reaper",
			}),
			UpdateTime: now,
			ID:         row.ID,
		})
		if err != nil {
			return reaped, err
		}
		if affected > 0 {
			// 顺带清掉实时输出块：执行进程不在时没人会清，留着就是无主的运行中输出。
			_ = db.New(database).DeleteAutomationJobLogChunks(ctx, row.ID)
			reaped += int(affected)
		}
	}
	return reaped, nil
}

// StartStaleJobReaper 启动对账循环。单实例部署，进程内唯一 goroutine，随进程退出终止
// （与 monitor 的失联告警对账同一范式）。由 worker 模式启动——作业执行归它管。
func StartStaleJobReaper(ctx context.Context, database *sql.DB) {
	go func() {
		ticker := time.NewTicker(staleJobSweepGap)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 单轮失败不终止循环：下一轮重试即可（数据库抖动不该让对账永久停摆）。
				_, _ = ReapStaleJobs(ctx, database)
			}
		}
	}()
}
