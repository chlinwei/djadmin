package inspection

import (
	"context"
	"database/sql"
	"log/slog"
	"strconv"
	"strings"
	"time"

	db "autoadmin/internal/platform/database/generated"

	"github.com/robfig/cron/v3"
)

const (
	schedulerTickInterval      = 30 * time.Second
	cleanupInterval            = 24 * time.Hour
	defaultResultRetentionDays = 180
	retentionConfigKey         = "sys.inspection.executions.retention_days"
)

func (handler *Handler) RunScheduler(ctx context.Context) {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()
	lastCleanup := time.Now().Add(-cleanupInterval + 5*time.Minute)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		handler.dispatchDueTasks(ctx)
		if time.Since(lastCleanup) >= cleanupInterval {
			lastCleanup = time.Now()
			handler.cleanupExpiredExecutions(ctx)
		}
	}
}

// dispatchDueTasks advances each schedule in the same transaction that records
// the run, so a failed insert or a restart cannot consume the trigger.
func (handler *Handler) dispatchDueTasks(ctx context.Context) {
	now := time.Now().UTC()
	rows, err := db.New(handler.db).ListDueInspectionTasks(ctx, sql.NullTime{Time: now, Valid: true})
	if err != nil {
		slog.Error("list due inspection tasks", "error", err)
		return
	}
	for _, row := range rows {
		schedule, parseErr := cron.ParseStandard(row.CronExpression)
		if parseErr != nil {
			slog.Warn("skip inspection task with invalid cron", "task_id", row.ID, "cron", row.CronExpression)
			continue
		}
		now := time.Now().UTC()
		next := schedule.Next(now)
		claim := db.ClaimDueInspectionTaskParams{
			NextRunTime: sql.NullTime{Time: next, Valid: true}, UpdateTime: now,
			ID: row.ID, Now: row.NextRunTime,
		}
		executionID, message, runErr := handler.startRunWithClaim(ctx, row.ID, "scheduled", 0, "scheduler", &claim)
		switch {
		case runErr != nil:
			slog.Error("dispatch scheduled inspection", "task_id", row.ID, "error", runErr)
		case message != "":
			slog.Warn("scheduled inspection rejected", "task_id", row.ID, "reason", message)
		case executionID != 0:
			slog.Info("scheduled inspection dispatched", "task_id", row.ID, "execution_id", executionID)
		}
	}
}

// configValue 读 sys_config 字符串值；key 不存在或读失败返回空串（调用方用缺省值）。
func (handler *Handler) configValue(ctx context.Context, key string) string {
	row, err := db.New(handler.db).GetConfigByKey(ctx, key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(row.Value)
}

// cleanupExpiredExecutions prunes finished executions older than the retention
// window (sys_config sys.inspection.executions.retention_days, default 180 days).
// 键名必须与 sys_config 里真实存在的键逐字一致：此前写的是 inspection.results.retention_days
// （只有 sys. 前缀与 executions/results 之别），于是**永远读不到用户配的 90 天**、一直按 180 天兜底
// （2026-09-19 对着配置表逐条核对时发现）。
// Child rows are removed before executions because only inspection_result
// carries a physical foreign key.
func (handler *Handler) cleanupExpiredExecutions(ctx context.Context) {
	days := defaultResultRetentionDays
	if parsed, parseErr := strconv.Atoi(handler.configValue(ctx, retentionConfigKey)); parseErr == nil && parsed > 0 {
		days = parsed
	}
	cutoff := sql.NullTime{Time: time.Now().UTC().AddDate(0, 0, -days), Valid: true}
	transaction, err := handler.db.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("begin inspection cleanup", "error", err)
		return
	}
	defer transaction.Rollback()
	queries := db.New(transaction)
	if _, err = queries.DeleteFinishedInspectionResults(ctx, cutoff); err != nil {
		slog.Error("cleanup inspection results", "error", err)
		return
	}
	if _, err = queries.DeleteFinishedInspectionTargetExecutions(ctx, cutoff); err != nil {
		slog.Error("cleanup inspection target executions", "error", err)
		return
	}
	deleted, err := queries.DeleteFinishedInspectionExecutions(ctx, cutoff)
	if err != nil {
		slog.Error("cleanup inspection executions", "error", err)
		return
	}
	if err = transaction.Commit(); err != nil {
		slog.Error("commit inspection cleanup", "error", err)
		return
	}
	if deleted > 0 {
		slog.Info("inspection retention cleanup done", "executions_deleted", deleted, "retention_days", days)
	}
}
