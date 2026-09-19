package scheduler

import (
	"context"
	"database/sql"
	"strconv"
	"time"

	platformdb "autoadmin/internal/platform/database"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/pagination"
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
}

func NewRepository(pool *sql.DB) *Repository { return &Repository{pool: pool, queries: db.New(pool)} }

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
	return repository.queries.UpdateScheduledTask(ctx, params)
}

func (repository *Repository) SetEnabled(ctx context.Context, params db.SetScheduledTaskEnabledParams) error {
	return repository.queries.SetScheduledTaskEnabled(ctx, params)
}

func (repository *Repository) SchedulerEnabled(ctx context.Context) (bool, error) {
	config, err := repository.queries.GetConfigByKey(ctx, "sys.scheduler.enabled")
	if err != nil {
		return false, err
	}
	return config.Value == "true" || config.Value == "1", nil
}

func (repository *Repository) SetSchedulerEnabled(ctx context.Context, enabled bool) error {
	config, err := repository.queries.GetConfigByKey(ctx, "sys.scheduler.enabled")
	if err != nil {
		return err
	}
	value := "false"
	if enabled {
		value = "true"
	}
	return repository.queries.UpdateConfigValue(ctx, db.UpdateConfigValueParams{
		Value: value, DefaultValue: config.DefaultValue, UpdateTime: time.Now().UTC(), ID: config.ID,
	})
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

func (repository *Repository) Claim(ctx context.Context, taskID int64, startedAt time.Time) (bool, error) {
	result, err := repository.queries.ClaimScheduledTask(ctx, db.ClaimScheduledTaskParams{
		LastRunTime: sql.NullTime{Time: startedAt, Valid: true}, UpdateTime: startedAt, ID: taskID,
	})
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected == 1, err
}

func (repository *Repository) Complete(ctx context.Context, taskID int64, startedAt time.Time, status string, message string, output string) error {
	finishedAt := time.Now().UTC()
	return platformdb.InTransaction(ctx, repository.pool, func(queries *db.Queries) error {
		if err := queries.CompleteScheduledTask(ctx, db.CompleteScheduledTaskParams{
			LastStatus: sql.NullString{String: status, Valid: true}, LastMessage: sql.NullString{String: message, Valid: true},
			LastRunTime: sql.NullTime{Time: startedAt, Valid: true}, UpdateTime: finishedAt, ID: taskID,
		}); err != nil {
			return err
		}
		return queries.CreateScheduledTaskLog(ctx, db.CreateScheduledTaskLogParams{
			CreateTime: finishedAt, UpdateTime: finishedAt, RunTime: startedAt, Status: status,
			Message:         sql.NullString{String: message, Valid: true},
			DurationSeconds: sql.NullFloat64{Float64: finishedAt.Sub(startedAt).Seconds(), Valid: true},
			TaskID:          taskID, Output: sql.NullString{String: output, Valid: output != ""},
		})
	})
}

func (repository *Repository) RetentionDays(ctx context.Context, key string, fallback int) int {
	config, err := repository.queries.GetConfigByKey(ctx, key)
	if err != nil {
		return fallback
	}
	days, err := strconv.Atoi(config.Value)
	if err != nil || days < 0 {
		return fallback
	}
	return days
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
	hostLogRows, _ := hostLogs.RowsAffected()
	chunkLogs, err := repository.queries.DeleteAutomationExecutionJobLogsBefore(ctx, cutoff)
	if err != nil {
		return hostLogRows, 0, 0, err
	}
	chunkLogRows, _ := chunkLogs.RowsAffected()
	jobs, err := repository.queries.DeleteAutomationExecutionJobsBefore(ctx, cutoff)
	if err != nil {
		return hostLogRows, chunkLogRows, 0, err
	}
	jobRows, _ := jobs.RowsAffected()
	return hostLogRows, chunkLogRows, jobRows, nil
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
	deliveryRows, _ := deliveries.RowsAffected()
	events, err := repository.queries.DeleteAlertNotificationEventsForHistoriesBefore(ctx, cutoff)
	if err != nil {
		return deliveryRows, 0, 0, err
	}
	eventRows, _ := events.RowsAffected()
	histories, err := repository.queries.DeleteResolvedAlertHistoriesBefore(ctx, cutoff)
	if err != nil {
		return deliveryRows, eventRows, 0, err
	}
	historyRows, _ := histories.RowsAffected()
	return deliveryRows, eventRows, historyRows, nil
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
