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
	retentionConfigKey         = "inspection.results.retention_days"
)

// StartScheduler launches the background loop that fires cron-configured
// inspection tasks and prunes expired execution history. It must run in the API
// process because executions dispatch through the in-process Agent gateway.
func (handler *Handler) StartScheduler() {
	go handler.runScheduler()
}

func (handler *Handler) runScheduler() {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()
	lastCleanup := time.Now().Add(-cleanupInterval + 5*time.Minute)
	for range ticker.C {
		handler.dispatchDueTasks()
		if time.Since(lastCleanup) >= cleanupInterval {
			lastCleanup = time.Now()
			handler.cleanupExpiredExecutions()
		}
	}
}

// dispatchDueTasks claims every due task atomically (next_run_time moved forward
// in the same UPDATE) before dispatching, so multiple API replicas or overlapping
// ticks cannot double-fire the same schedule.
func (handler *Handler) dispatchDueTasks() {
	type dueTask struct {
		id             int64
		cronExpression string
	}
	now := time.Now().UTC()
	rows, err := db.New(handler.db).ListDueInspectionTasks(context.Background(), sql.NullTime{Time: now, Valid: true})
	if err != nil {
		slog.Error("list due inspection tasks", "error", err)
		return
	}
	dueTasks := make([]dueTask, 0, len(rows))
	for _, row := range rows {
		dueTasks = append(dueTasks, dueTask{id: row.ID, cronExpression: row.CronExpression})
	}
	for _, task := range dueTasks {
		schedule, parseErr := cron.ParseStandard(task.cronExpression)
		if parseErr != nil {
			slog.Warn("skip inspection task with invalid cron", "task_id", task.id, "cron", task.cronExpression)
			continue
		}
		now := time.Now().UTC()
		next := schedule.Next(now)
		claimed, claimErr := db.New(handler.db).ClaimDueInspectionTask(context.Background(), db.ClaimDueInspectionTaskParams{
			NextRunTime: sql.NullTime{Time: next, Valid: true}, UpdateTime: now,
			ID: task.id, Now: sql.NullTime{Time: now, Valid: true},
		})
		if claimErr != nil || claimed == 0 {
			continue
		}
		executionID, message, runErr := handler.startRun(context.Background(), task.id, "scheduled", 0, "scheduler")
		switch {
		case runErr != nil:
			slog.Error("dispatch scheduled inspection", "task_id", task.id, "error", runErr)
		case message != "":
			slog.Warn("scheduled inspection rejected", "task_id", task.id, "reason", message)
		default:
			slog.Info("scheduled inspection dispatched", "task_id", task.id, "execution_id", executionID)
		}
	}
}

// configValue 读 sys_config 字符串值；key 不存在或读失败返回空串（调用方用缺省值）。
func (handler *Handler) configValue(key string) string {
	row, err := db.New(handler.db).GetConfigByKey(context.Background(), key)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(row.Value)
}

// cleanupExpiredExecutions prunes finished executions older than the retention
// window (sys_config inspection.results.retention_days, default 180 days).
// Child rows are removed before executions because only inspection_result
// carries a physical foreign key.
func (handler *Handler) cleanupExpiredExecutions() {
	days := defaultResultRetentionDays
	if parsed, parseErr := strconv.Atoi(handler.configValue(retentionConfigKey)); parseErr == nil && parsed > 0 {
		days = parsed
	}
	cutoff := sql.NullTime{Time: time.Now().UTC().AddDate(0, 0, -days), Valid: true}
	transaction, err := handler.db.Begin()
	if err != nil {
		slog.Error("begin inspection cleanup", "error", err)
		return
	}
	defer transaction.Rollback()
	queries := db.New(transaction)
	if _, err = queries.DeleteFinishedInspectionResults(context.Background(), cutoff); err != nil {
		slog.Error("cleanup inspection results", "error", err)
		return
	}
	if _, err = queries.DeleteFinishedInspectionTargetExecutions(context.Background(), cutoff); err != nil {
		slog.Error("cleanup inspection target executions", "error", err)
		return
	}
	deleted, err := queries.DeleteFinishedInspectionExecutions(context.Background(), cutoff)
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
