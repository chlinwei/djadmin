package scheduler

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"autoadmin/internal/job"
	db "autoadmin/internal/platform/database/generated"
	"autoadmin/internal/shared/apperror"
	"autoadmin/internal/shared/pagination"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

type Task struct {
	ID                      int64   `json:"id"`
	Name                    string  `json:"name"`
	Code                    string  `json:"code"`
	Description             *string `json:"description"`
	Menu                    *int32  `json:"menu"`
	MenuName                *string `json:"menu_name"`
	MenuPath                *string `json:"menu_path"`
	Enabled                 bool    `json:"enabled"`
	IsRunning               bool    `json:"is_running"`
	CronExpression          *string `json:"cron_expression"`
	EffectiveCronExpression string  `json:"effective_cron_expression"`
	IntervalMinutes         *int32  `json:"interval_minutes"`
	LastRunTime             *string `json:"last_run_time"`
	NextRunTime             *string `json:"next_run_time"`
	LastStatus              *string `json:"last_status"`
	LastMessage             *string `json:"last_message"`
	CreateTime              string  `json:"create_time"`
	UpdateTime              string  `json:"update_time"`
	Logs                    []any   `json:"logs"`
}

type TaskInput struct {
	Name, Code     string
	Description    *string
	Enabled        bool
	CronExpression string
}
type Service struct {
	repository *Repository
	publisher  Publisher
}

func NewService(repository *Repository) *Service              { return &Service{repository: repository} }
func (s *Service) WithPublisher(publisher Publisher) *Service { s.publisher = publisher; return s }

func (s *Service) ListTasks(ctx context.Context, filter TaskFilter, page pagination.Page) ([]Task, int64, error) {
	rows, count, err := s.repository.ListTasks(ctx, filter, page)
	if err != nil {
		return nil, 0, err
	}
	result := make([]Task, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapListTask(row))
	}
	return result, count, nil
}
func (s *Service) GetTask(ctx context.Context, id int64) (Task, error) {
	row, err := s.repository.GetTask(ctx, id)
	return mapTask(row), err
}
func (s *Service) UpdateTask(ctx context.Context, id int64, input TaskInput) (Task, error) {
	if strings.TrimSpace(input.CronExpression) == "" {
		return Task{}, ErrCronRequired
	}
	next, err := nextRun(input.CronExpression, input.Enabled)
	if err != nil {
		return Task{}, ErrCronInvalid
	}
	err = s.repository.UpdateTask(ctx, db.UpdateScheduledTaskParams{Name: input.Name, Code: input.Code, Description: nullString(input.Description), Enabled: input.Enabled, CronExpression: sql.NullString{String: input.CronExpression, Valid: true}, NextRunTime: next, UpdateTime: time.Now().UTC(), ID: id})
	if err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, id)
}
func (s *Service) SetEnabled(ctx context.Context, id int64, enabled bool) (Task, error) {
	task, err := s.repository.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	cron := ""
	if task.CronExpression.Valid {
		cron = task.CronExpression.String
	}
	next, err := nextRun(cron, enabled)
	if err != nil {
		return Task{}, ErrCronInvalid
	}
	err = s.repository.SetEnabled(ctx, db.SetScheduledTaskEnabledParams{Enabled: enabled, NextRunTime: next, UpdateTime: time.Now().UTC(), ID: id})
	if err != nil {
		return Task{}, err
	}
	return s.GetTask(ctx, id)
}
func (s *Service) Status(ctx context.Context, id int64) (map[string]any, error) {
	task, err := s.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	enabled, _ := s.repository.SchedulerEnabled(ctx)
	return map[string]any{"task_id": task.ID, "task_name": task.Name, "is_running": task.IsRunning, "scheduler_enabled": enabled, "last_status": task.LastStatus, "last_message": task.LastMessage, "last_run_time": task.LastRunTime, "next_run_time": task.NextRunTime}, nil
}
func (s *Service) RunNow(ctx context.Context, id int64) (Task, error) {
	task, err := s.repository.GetTask(ctx, id)
	if err != nil {
		return Task{}, err
	}
	if !task.Enabled {
		return Task{}, ErrTaskDisabled
	}
	if task.IsRunning {
		return Task{}, ErrTaskRunning
	}
	if !IsSupportedTaskCode(task.Code) {
		return Task{}, ErrHandlerUnsupported
	}
	if s.publisher == nil {
		return Task{}, ErrWorkerUnavailable
	}
	if err := s.publisher.Publish(ctx, job.Message{
		SchemaVersion: job.SchemaVersion, ExecutionID: uuid.NewString(), Kind: "scheduled_task",
		ResourceID: id, TriggeredAt: time.Now().UTC(),
	}); err != nil {
		return Task{}, apperror.WithCause(ErrDispatchInternal, err)
	}
	return mapTask(task), nil
}
func (s *Service) ListLogs(ctx context.Context, filter LogFilter, page pagination.Page) ([]db.ListScheduledTaskLogsRow, int64, error) {
	return s.repository.ListLogs(ctx, filter, page)
}
func (s *Service) GetLog(ctx context.Context, id int64) (db.GetScheduledTaskLogRow, error) {
	return s.repository.GetLog(ctx, id)
}

func nextRun(expression string, enabled bool) (sql.NullTime, error) {
	if !enabled {
		return sql.NullTime{}, nil
	}
	if len(strings.Fields(expression)) != 5 {
		return sql.NullTime{}, ErrCronInvalid
	}
	schedulerInstance, err := gocron.NewScheduler(gocron.WithLocation(time.UTC))
	if err != nil {
		return sql.NullTime{}, err
	}
	job, err := schedulerInstance.NewJob(gocron.CronJob(strings.TrimSpace(expression), false), gocron.NewTask(func() {}))
	if err != nil {
		return sql.NullTime{}, err
	}
	schedulerInstance.Start()
	defer schedulerInstance.Shutdown()
	deadline := time.Now().Add(100 * time.Millisecond)
	for time.Now().Before(deadline) {
		next, err := job.NextRun()
		if err == nil && !next.IsZero() {
			return sql.NullTime{Time: next.UTC(), Valid: true}, nil
		}
		time.Sleep(time.Millisecond)
	}
	return sql.NullTime{}, ErrCronInvalid
}
func nullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}
func stringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
func intPtr(value sql.NullInt32) *int32 {
	if !value.Valid {
		return nil
	}
	return &value.Int32
}
func timePtr(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	text := value.Time.UTC().Format("2006-01-02T15:04:05.999999Z")
	return &text
}
func mapTask(row db.GetScheduledTaskRow) Task {
	return Task{ID: row.ID, Name: row.Name, Code: row.Code, Description: stringPtr(row.Description), Menu: intPtr(row.MenuID), MenuName: stringPtr(row.MenuName), MenuPath: stringPtr(row.MenuPath), Enabled: row.Enabled, IsRunning: row.IsRunning, CronExpression: stringPtr(row.CronExpression), EffectiveCronExpression: row.CronExpression.String, IntervalMinutes: intPtr(row.IntervalMinutes), LastRunTime: timePtr(row.LastRunTime), NextRunTime: timePtr(row.NextRunTime), LastStatus: stringPtr(row.LastStatus), LastMessage: stringPtr(row.LastMessage), CreateTime: row.CreateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), UpdateTime: row.UpdateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), Logs: []any{}}
}
func mapListTask(row db.ListScheduledTasksRow) Task {
	return Task{ID: row.ID, Name: row.Name, Code: row.Code, Description: stringPtr(row.Description), Menu: intPtr(row.MenuID), MenuName: stringPtr(row.MenuName), MenuPath: stringPtr(row.MenuPath), Enabled: row.Enabled, IsRunning: row.IsRunning, CronExpression: stringPtr(row.CronExpression), EffectiveCronExpression: row.CronExpression.String, IntervalMinutes: intPtr(row.IntervalMinutes), LastRunTime: timePtr(row.LastRunTime), NextRunTime: timePtr(row.NextRunTime), LastStatus: stringPtr(row.LastStatus), LastMessage: stringPtr(row.LastMessage), CreateTime: row.CreateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), UpdateTime: row.UpdateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), Logs: []any{}}
}
