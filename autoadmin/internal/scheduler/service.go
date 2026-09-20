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

	"github.com/robfig/cron/v3"
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
	// Supported 该任务的 handler 是否已实现（见 worker.go 的 supportedTaskCodes）。
	// false = 随 Django 后端一起移出的历史任务：定时调度会**跳过**它、手动执行也必然失败。
	// 页面据此把「立即执行」置灰并说明原因——不让人点一下再看报错（2026-09-19 现场）。
	Supported bool `json:"supported"`
	// SupportNote 不可执行时给用户看的一句说明（为什么、能执行什么）；可执行时为空串。
	SupportNote string `json:"support_note"`
	// ScheduleTimezone cron 的解释时区（= ScheduleLocation，如 "Local"/"Asia/Shanghai"）。
	// 界面用它写清"下次运行时间按哪个时区算"——cron 的钟点是服务器时区的钟点，不是用户所在时区的。
	ScheduleTimezone string `json:"schedule_timezone"`
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
	// 实现未迁移的任务：库里不留时刻（它不会被调度），cron 表达式本身仍照常校验。
	if !IsSupportedTaskCode(input.Code) {
		next = sql.NullTime{}
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
	if !IsSupportedTaskCode(task.Code) {
		next = sql.NullTime{}
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
		// 具体到任务名与编码，并给出"已实现的任务有哪些"：通用文案（ErrHandlerUnsupported）
		// 只说"未迁移"，用户既不知道是谁的问题、也不知道要不要等它。
		return Task{}, apperror.New(apperror.CodeInvalidArgument, UnsupportedTaskNote(task.Name, task.Code))
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

// ScheduleLocation 是 cron 表达式的**解释时区**，也是"界面显示的下次运行时间"与"真实触发时刻"
// 对齐的唯一依据：执行侧是进程内 gocron（`app.go` 注册任务），它按调度器自己的 location 解释
// 表达式；展示侧若用另一个时区算，就会变成"界面显示 17:00、实际 9:00 触发"——两边都自认有道理，
// 用户只能猜（2026-09-20 现场：下次运行时间看着不对）。
//
// 只此一处定义：`Manager` 用 `WithLocation` 拿它建调度器，`nextRun` 用它算下次时间。
// 要改成固定时区（例如统一按 Asia/Shanghai 解释 cron）只改这里。
var ScheduleLocation = time.Local

// nextRun 算下一次触发时刻（UTC），cron 按 ScheduleLocation 解释。
//
// 用 robfig/cron 的纯函数解析（巡检调度同一套，见 inspection/scheduler.go）：早先的实现是
// 临时起一个 gocron 调度器、轮询 100ms 取 `job.NextRun()`——慢、有副作用，而且 location 固定
// UTC 与执行侧（time.Local）不一致。
func nextRun(expression string, enabled bool) (sql.NullTime, error) {
	if !enabled {
		return sql.NullTime{}, nil
	}
	trimmed := strings.TrimSpace(expression)
	if len(strings.Fields(trimmed)) != 5 {
		return sql.NullTime{}, ErrCronInvalid
	}
	schedule, err := cron.ParseStandard(trimmed)
	if err != nil {
		return sql.NullTime{}, ErrCronInvalid
	}
	next := schedule.Next(time.Now().In(ScheduleLocation))
	if next.IsZero() {
		return sql.NullTime{}, ErrCronInvalid
	}
	return sql.NullTime{Time: next.UTC(), Valid: true}, nil
}

// displayNextRunTime 界面上「下次运行时间」的值：**实时算**，不信库里的 `next_run_time`。
//
// 库里那列只在保存/启停时写过一次，之后没有任何东西推进它（真正的触发由进程内 gocron 自己算），
// 所以它会越来越旧、最后显示成一个**过去的时间**——用户看到"下次运行时间比现在早"就是这个
// （2026-09-20 现场）。实时算顺带把另外两种情况说清（不给一个不会发生的时刻）：
//   - 任务已停用 → 没有下次；
//   - 实现未迁移（supported=false）→ 调度器根本不注册它，不会跑（见 worker.go 的 supportedTaskCodes）。
func displayNextRunTime(task Task) *string {
	if !task.Enabled || !task.Supported || strings.TrimSpace(task.EffectiveCronExpression) == "" {
		return nil
	}
	next, err := nextRun(task.EffectiveCronExpression, true)
	if err != nil || !next.Valid {
		return nil
	}
	text := next.Time.UTC().Format("2006-01-02T15:04:05.999999Z")
	return &text
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
	task := Task{ID: row.ID, Name: row.Name, Code: row.Code, Description: stringPtr(row.Description), Menu: intPtr(row.MenuID), MenuName: stringPtr(row.MenuName), MenuPath: stringPtr(row.MenuPath), Enabled: row.Enabled, IsRunning: row.IsRunning, CronExpression: stringPtr(row.CronExpression), EffectiveCronExpression: row.CronExpression.String, IntervalMinutes: intPtr(row.IntervalMinutes), LastRunTime: timePtr(row.LastRunTime), NextRunTime: timePtr(row.NextRunTime), LastStatus: stringPtr(row.LastStatus), LastMessage: stringPtr(row.LastMessage), CreateTime: row.CreateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), UpdateTime: row.UpdateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), Logs: []any{}}
	withTaskSupport(&task)
	return task
}

// withTaskSupport 给 DTO 补"这个任务的实现到底有没有"——列表与详情共用，避免两处口径不一致。
func withTaskSupport(task *Task) {
	task.Supported = IsSupportedTaskCode(task.Code)
	if !task.Supported {
		task.SupportNote = UnsupportedTaskNote(task.Name, task.Code)
	}
	// 「下次运行时间」以**实时算**为准，覆盖库里的快照（见 displayNextRunTime 的注释）。
	// 放在这里而不是各 mapper 里：列表与详情共用一处口径，不会再出现两处不一致。
	task.NextRunTime = displayNextRunTime(*task)
	task.ScheduleTimezone = ScheduleLocation.String()
}
func mapListTask(row db.ListScheduledTasksRow) Task {
	task := Task{ID: row.ID, Name: row.Name, Code: row.Code, Description: stringPtr(row.Description), Menu: intPtr(row.MenuID), MenuName: stringPtr(row.MenuName), MenuPath: stringPtr(row.MenuPath), Enabled: row.Enabled, IsRunning: row.IsRunning, CronExpression: stringPtr(row.CronExpression), EffectiveCronExpression: row.CronExpression.String, IntervalMinutes: intPtr(row.IntervalMinutes), LastRunTime: timePtr(row.LastRunTime), NextRunTime: timePtr(row.NextRunTime), LastStatus: stringPtr(row.LastStatus), LastMessage: stringPtr(row.LastMessage), CreateTime: row.CreateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), UpdateTime: row.UpdateTime.UTC().Format("2006-01-02T15:04:05.999999Z"), Logs: []any{}}
	withTaskSupport(&task)
	return task
}
