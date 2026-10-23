package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"

	"github.com/google/uuid"
)

type TaskFilter struct {
	Search    string
	Enabled   *bool
	IsRunning *bool
}

type LogFilter struct {
	TaskID      *int64
	Status      string
	DurationMin *float64
	DurationMax *float64
	Content     string
}

type Repository struct {
	pool    *sql.DB
	queries *db.Queries
	tx      *sql.Tx
}

func NewRepository(pool *sql.DB) *Repository { return &Repository{pool: pool, queries: db.New(pool)} }

func (repository *Repository) transaction(ctx context.Context, fn func(*Repository) error) error {
	tx, err := repository.pool.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Cleanup helpers receive only transaction-scoped queries, never a pool.
	if err := fn(&Repository{queries: db.New(tx), tx: tx}); err != nil {
		return err
	}
	return tx.Commit()
}

func (repository *Repository) ListTasks(ctx context.Context, filter TaskFilter, page pagination.Page) ([]db.ListScheduledTasksRow, int64, error) {
	params := taskParams(filter)
	count, err := repository.queries.CountScheduledTasks(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	rows, err := repository.queries.ListScheduledTasks(ctx, db.ListScheduledTasksParams{
		NamePattern: params.NamePattern, CodePattern: params.CodePattern,
		Enabled: params.Enabled, IsRunning: params.IsRunning, Limit: page.Size, Offset: page.Offset,
	})
	return rows, count, err
}

func taskParams(filter TaskFilter) db.CountScheduledTasksParams {
	pattern := sql.NullString{}
	if filter.Search != "" {
		pattern = sql.NullString{String: "%" + filter.Search + "%", Valid: true}
	}
	enabled, running := sql.NullBool{}, sql.NullBool{}
	if filter.Enabled != nil {
		enabled = sql.NullBool{Bool: *filter.Enabled, Valid: true}
	}
	if filter.IsRunning != nil {
		running = sql.NullBool{Bool: *filter.IsRunning, Valid: true}
	}
	return db.CountScheduledTasksParams{NamePattern: pattern, CodePattern: pattern, Enabled: enabled, IsRunning: running}
}

func (repository *Repository) GetTask(ctx context.Context, id int64) (db.GetScheduledTaskRow, error) {
	return repository.queries.GetScheduledTask(ctx, id)
}

func (repository *Repository) UpdateTask(ctx context.Context, params db.UpdateScheduledTaskParams) error {
	return repository.transaction(ctx, func(tx *Repository) error {
		task, err := tx.queries.LockSchedulerTask(ctx, params.ID)
		if err != nil {
			return err
		}
		params.NextRunTime = task.NextRunTime
		if task.Enabled != params.Enabled || task.Code != params.Code || task.CronExpression != params.CronExpression {
			next, err := tx.resetTask(ctx, task.ID, params.Code, params.CronExpression.String, task.ScheduleTimezone, params.Enabled)
			if err != nil {
				return err
			}
			params.NextRunTime = next
		}
		return tx.queries.UpdateScheduledTask(ctx, params)
	})
}

func (repository *Repository) SetEnabled(ctx context.Context, params db.SetScheduledTaskEnabledParams) error {
	return repository.transaction(ctx, func(tx *Repository) error {
		task, err := tx.queries.LockSchedulerTask(ctx, params.ID)
		if err != nil {
			return err
		}
		params.NextRunTime = task.NextRunTime
		if task.Enabled != params.Enabled {
			params.NextRunTime, err = tx.resetTask(ctx, task.ID, task.Code, task.CronExpression.String, task.ScheduleTimezone, params.Enabled)
			if err != nil {
				return err
			}
		}
		return tx.queries.SetScheduledTaskEnabled(ctx, params)
	})
}

func schedulerEnabled(value string) (bool, error) {
	switch value {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid sys.scheduler.enabled value %q", value)
	}
}

func (repository *Repository) SchedulerEnabled(ctx context.Context) (bool, error) {
	config, err := repository.queries.GetConfigByKey(ctx, "sys.scheduler.enabled")
	if err != nil {
		return false, err
	}
	return schedulerEnabled(config.Value)
}

func (repository *Repository) SetSchedulerEnabled(ctx context.Context, enabled bool) error {
	return repository.updateSchedulerConfig(ctx, strconv.FormatBool(enabled), nil)
}

// UpdateSchedulerConfig is the generic config-write entry point for the scheduler
// switch. Preserve its default value in the same transaction as cursor reset and
// pending cancellation, so generic update/reset cannot bypass scheduler semantics.
func (repository *Repository) UpdateSchedulerConfig(ctx context.Context, params db.UpdateConfigValueParams) error {
	return repository.updateSchedulerConfig(ctx, params.Value, &params)
}

func (repository *Repository) updateSchedulerConfig(ctx context.Context, value string, params *db.UpdateConfigValueParams) error {
	enabled, err := schedulerEnabled(value)
	if err != nil {
		return err
	}
	return repository.transaction(ctx, func(tx *Repository) error {
		// All paths lock tasks in ascending order before the global switch. Waiting
		// here lets a running cleanup finish before disable becomes committed.
		ids, err := tx.queries.ListSchedulerTaskIDs(ctx)
		if err != nil {
			return err
		}
		tasks := make([]db.SchedulerScheduledtask, 0, len(ids))
		for _, id := range ids {
			task, err := tx.queries.LockSchedulerTask(ctx, id)
			if err != nil {
				return err
			}
			tasks = append(tasks, task)
		}
		config, err := tx.queries.LockSchedulerConfig(ctx)
		if err != nil {
			return err
		}
		if params != nil && params.ID != config.ID {
			return fmt.Errorf("scheduler config ID mismatch")
		}
		previous, err := schedulerEnabled(config.Value)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if previous != enabled {
			for _, task := range tasks {
				next := sql.NullTime{}
				if enabled && task.Enabled && IsSupportedTaskCode(task.Code) {
					next, err = nextRunAt(task.CronExpression.String, task.ScheduleTimezone, now)
					if err != nil {
						return err
					}
				}
				if err := tx.queries.CancelAutomaticSchedulerExecutions(ctx, db.CancelAutomaticSchedulerExecutionsParams{
					FinishedAt: sql.NullTime{Time: now, Valid: true}, TaskID: task.ID,
				}); err != nil {
					return err
				}
				if err := tx.queries.SetSchedulerCursor(ctx, db.SetSchedulerCursorParams{NextRunTime: next, ID: task.ID}); err != nil {
					return err
				}
			}
		}
		defaultValue := config.DefaultValue
		if params != nil {
			defaultValue = params.DefaultValue
			now = params.UpdateTime
		}
		return tx.queries.UpdateConfigValue(ctx, db.UpdateConfigValueParams{
			Value: value, DefaultValue: defaultValue, UpdateTime: now, ID: config.ID,
		})
	})
}

func (repository *Repository) resetTask(ctx context.Context, id int64, code, expression, timezone string, enabled bool) (sql.NullTime, error) {
	now := time.Now().UTC()
	if err := repository.queries.CancelPendingSchedulerExecutions(ctx, db.CancelPendingSchedulerExecutionsParams{
		FinishedAt: sql.NullTime{Time: now, Valid: true}, TaskID: id,
	}); err != nil {
		return sql.NullTime{}, err
	}
	config, err := repository.queries.LockSchedulerConfig(ctx)
	if err != nil {
		return sql.NullTime{}, err
	}
	globalEnabled, err := schedulerEnabled(config.Value)
	if err != nil || !enabled || !globalEnabled || !IsSupportedTaskCode(code) {
		return sql.NullTime{}, err
	}
	return nextRunAt(expression, timezone, now)
}

func (repository *Repository) ListLogs(ctx context.Context, filter LogFilter, page pagination.Page) ([]db.ListScheduledTaskLogsRow, int64, error) {
	params := logParams(filter)
	count, err := repository.queries.CountScheduledTaskLogs(ctx, params)
	if err != nil {
		return nil, 0, err
	}
	rows, err := repository.queries.ListScheduledTaskLogs(ctx, db.ListScheduledTaskLogsParams{
		TaskID: params.TaskID, ExactStatus: params.ExactStatus,
		DurationMin: params.DurationMin, DurationMax: params.DurationMax,
		MessagePattern: params.MessagePattern, OutputPattern: params.OutputPattern,
		Limit: page.Size, Offset: page.Offset,
	})
	return rows, count, err
}

func (repository *Repository) GetLog(ctx context.Context, id int64) (db.GetScheduledTaskLogRow, error) {
	return repository.queries.GetScheduledTaskLog(ctx, id)
}

func (repository *Repository) complete(ctx context.Context, taskID int64, startedAt time.Time, status string, message string, output string) error {
	finishedAt := time.Now().UTC()
	if err := repository.queries.CompleteScheduledTask(ctx, db.CompleteScheduledTaskParams{
		LastStatus: sql.NullString{String: status, Valid: true}, LastMessage: sql.NullString{String: message, Valid: true},
		LastRunTime: sql.NullTime{Time: startedAt, Valid: true}, UpdateTime: finishedAt, ID: taskID,
	}); err != nil {
		return err
	}
	return repository.queries.CreateScheduledTaskLog(ctx, db.CreateScheduledTaskLogParams{
		CreateTime: finishedAt, UpdateTime: finishedAt, RunTime: startedAt, Status: status,
		Message:         sql.NullString{String: message, Valid: true},
		DurationSeconds: sql.NullFloat64{Float64: finishedAt.Sub(startedAt).Seconds(), Valid: true},
		TaskID:          taskID, Output: sql.NullString{String: output, Valid: output != ""},
	})
}

func (repository *Repository) RetentionDays(ctx context.Context, key string) (int, error) {
	config, err := repository.queries.GetConfigByKey(ctx, key)
	if err != nil {
		return 0, fmt.Errorf("read retention %s: %w", key, err)
	}
	days, err := strconv.Atoi(config.Value)
	if err != nil || days < 0 {
		return 0, fmt.Errorf("invalid retention %s: %q", key, config.Value)
	}
	return days, nil
}

func (repository *Repository) enqueue(ctx context.Context, taskID int64, trigger string, scheduledAt, now time.Time) error {
	return repository.queries.CreateSchedulerExecution(ctx, db.CreateSchedulerExecutionParams{
		ID: uuid.NewString(), TaskID: taskID, TriggerType: trigger, ScheduledAt: scheduledAt, CreatedAt: now,
	})
}

func (repository *Repository) EnqueueManual(ctx context.Context, id int64) error {
	return repository.transaction(ctx, func(tx *Repository) error {
		task, err := tx.queries.TryLockSchedulerTask(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			// A separate non-locking read distinguishes a missing task from an
			// executing task without waiting for the cleanup to finish.
			if _, readErr := tx.GetTask(ctx, id); readErr != nil {
				return readErr
			}
			return ErrTaskRunning
		}
		if err != nil {
			return err
		}
		if !task.Enabled {
			return ErrTaskDisabled
		}
		if !IsSupportedTaskCode(task.Code) {
			return ErrHandlerUnsupported
		}
		if _, err := tx.queries.GetPendingSchedulerExecution(ctx, id); err == nil {
			return ErrTaskRunning
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := time.Now().UTC()
		return tx.enqueue(ctx, id, "manual", now, now)
	})
}

func (repository *Repository) CleanupLoginAudits(ctx context.Context, before time.Time) (int64, error) {
	result, err := repository.queries.DeleteLoginAuditsBefore(ctx, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (repository *Repository) CleanupOperationAudits(ctx context.Context, before time.Time) (int64, error) {
	result, err := repository.queries.DeleteOperationAuditsBefore(ctx, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ---- 四类保留期清理（2026-09-19：从 Django 时代的历史任务迁到 Go）----
//
// 都是"删早于 cutoff 的行"：cutoff 由 handler 按 sys_config 的保留天数算好传进来（时间一律由应用层给，
// 与巡检清理、审计清理同一口径），仓储层只负责跑语句并回影响行数——影响行数会写进任务执行日志，
// 是"这次到底清了多少"的唯一证据。

func (repository *Repository) DeleteWebSSHSessionLogsBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := repository.queries.DeleteWebSSHSessionLogsBefore(ctx, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteAutomationExecutionLogsBefore 清理自动化执行记录：**先子后父**（外键无级联），
// 顺序错了会直接撞外键。返回 (主机明细, 作业字节块, 作业行) 三个影响行数。
func (repository *Repository) DeleteAutomationExecutionLogsBefore(ctx context.Context, before time.Time) (int64, int64, int64, error) {
	// end_time / resolved_at 是可空列，sqlc 把参数推成 sql.NullTime（"早于 cutoff"的 cutoff 本身非空）。
	cutoff := sql.NullTime{Time: before, Valid: true}
	hostLogs, err := repository.queries.DeleteAutomationExecutionHostLogsBefore(ctx, cutoff)
	if err != nil {
		return 0, 0, 0, err
	}
	hostLogRows, err := hostLogs.RowsAffected()
	if err != nil {
		return 0, 0, 0, err
	}
	chunkLogs, err := repository.queries.DeleteAutomationExecutionJobLogsBefore(ctx, cutoff)
	if err != nil {
		return hostLogRows, 0, 0, err
	}
	chunkLogRows, err := chunkLogs.RowsAffected()
	if err != nil {
		return hostLogRows, 0, 0, err
	}
	jobs, err := repository.queries.DeleteAutomationExecutionJobsBefore(ctx, cutoff)
	if err != nil {
		return hostLogRows, chunkLogRows, 0, err
	}
	jobRows, err := jobs.RowsAffected()
	return hostLogRows, chunkLogRows, jobRows, err
}

func (repository *Repository) DeleteMonitorInstallHistoriesBefore(ctx context.Context, before time.Time) (int64, error) {
	result, err := repository.queries.DeleteMonitorInstallHistoriesBefore(ctx, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteResolvedAlertHistoriesBefore 清理已恢复的历史告警：**先投递记录、再通知事件、最后告警行**
// （外键无级联，顺序错了会撞外键）。返回 (投递记录, 通知事件, 告警行) 三个影响行数。
func (repository *Repository) DeleteResolvedAlertHistoriesBefore(ctx context.Context, before time.Time) (int64, int64, int64, error) {
	cutoff := sql.NullTime{Time: before, Valid: true}
	deliveries, err := repository.queries.DeleteAlertNotificationDeliveriesForHistoriesBefore(ctx, cutoff)
	if err != nil {
		return 0, 0, 0, err
	}
	deliveryRows, err := deliveries.RowsAffected()
	if err != nil {
		return 0, 0, 0, err
	}
	events, err := repository.queries.DeleteAlertNotificationEventsForHistoriesBefore(ctx, cutoff)
	if err != nil {
		return deliveryRows, 0, 0, err
	}
	eventRows, err := events.RowsAffected()
	if err != nil {
		return deliveryRows, 0, 0, err
	}
	histories, err := repository.queries.DeleteResolvedAlertHistoriesBefore(ctx, cutoff)
	if err != nil {
		return deliveryRows, eventRows, 0, err
	}
	historyRows, err := histories.RowsAffected()
	return deliveryRows, eventRows, historyRows, err
}

func logParams(filter LogFilter) db.CountScheduledTaskLogsParams {
	params := db.CountScheduledTaskLogsParams{}
	if filter.TaskID != nil {
		params.TaskID = sql.NullInt64{Int64: *filter.TaskID, Valid: true}
	}
	if filter.Status != "" {
		params.ExactStatus = sql.NullString{String: filter.Status, Valid: true}
	}
	if filter.DurationMin != nil {
		params.DurationMin = sql.NullFloat64{Float64: *filter.DurationMin, Valid: true}
	}
	if filter.DurationMax != nil {
		params.DurationMax = sql.NullFloat64{Float64: *filter.DurationMax, Valid: true}
	}
	if filter.Content != "" {
		pattern := sql.NullString{String: "%" + filter.Content + "%", Valid: true}
		params.MessagePattern, params.OutputPattern = pattern, pattern
	}
	return params
}
