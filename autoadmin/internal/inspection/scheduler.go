package inspection

import (
	"context"
	"log/slog"
	"strconv"
	"strings"
	"time"

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
	rows, err := handler.db.Query(`SELECT id,cron_expression FROM inspection_task WHERE enabled=TRUE AND cron_expression<>'' AND next_run_time IS NOT NULL AND next_run_time<=UTC_TIMESTAMP(6)`)
	if err != nil {
		slog.Error("list due inspection tasks", "error", err)
		return
	}
	dueTasks := make([]dueTask, 0)
	for rows.Next() {
		var task dueTask
		if err = rows.Scan(&task.id, &task.cronExpression); err != nil {
			rows.Close()
			slog.Error("scan due inspection task", "error", err)
			return
		}
		dueTasks = append(dueTasks, task)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		slog.Error("iterate due inspection tasks", "error", err)
		return
	}
	for _, task := range dueTasks {
		schedule, parseErr := cron.ParseStandard(task.cronExpression)
		if parseErr != nil {
			slog.Warn("skip inspection task with invalid cron", "task_id", task.id, "cron", task.cronExpression)
			continue
		}
		next := schedule.Next(time.Now().UTC())
		result, claimErr := handler.db.Exec(`UPDATE inspection_task SET next_run_time=?,update_time=NOW() WHERE id=? AND next_run_time<=UTC_TIMESTAMP(6)`, next, task.id)
		if claimErr != nil || rowsAffected(result) == 0 {
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
	var value string
	if err := handler.db.QueryRow(`SELECT value FROM sys_config WHERE `+"`key`"+`=?`, key).Scan(&value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
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
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	transaction, err := handler.db.Begin()
	if err != nil {
		slog.Error("begin inspection cleanup", "error", err)
		return
	}
	defer transaction.Rollback()
	if _, err = transaction.Exec(`DELETE r FROM inspection_result r JOIN inspection_target_execution t ON t.id=r.target_id JOIN inspection_execution e ON e.id=t.execution_id WHERE e.status<>'pending' AND e.status<>'running' AND e.end_time<?`, cutoff); err != nil {
		slog.Error("cleanup inspection results", "error", err)
		return
	}
	if _, err = transaction.Exec(`DELETE t FROM inspection_target_execution t JOIN inspection_execution e ON e.id=t.execution_id WHERE e.status<>'pending' AND e.status<>'running' AND e.end_time<?`, cutoff); err != nil {
		slog.Error("cleanup inspection target executions", "error", err)
		return
	}
	result, err := transaction.Exec(`DELETE FROM inspection_execution WHERE status<>'pending' AND status<>'running' AND end_time<?`, cutoff)
	if err != nil {
		slog.Error("cleanup inspection executions", "error", err)
		return
	}
	if err = transaction.Commit(); err != nil {
		slog.Error("commit inspection cleanup", "error", err)
		return
	}
	if deleted, _ := result.RowsAffected(); deleted > 0 {
		slog.Info("inspection retention cleanup done", "executions_deleted", deleted, "retention_days", days)
	}
}
