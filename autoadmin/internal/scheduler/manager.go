package scheduler

import (
	"context"
	"fmt"
	"time"

	"autoadmin/internal/job"

	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
)

type Publisher interface {
	Publish(context.Context, job.Message) error
}

type Definition struct {
	ID             int64
	Name           string
	Kind           string
	CronExpression string
}

type Manager struct {
	scheduler gocron.Scheduler
	publisher Publisher
}

func New(publisher Publisher) (*Manager, error) {
	// 显式指定解释 cron 的时区：与 showNextRunTime/nextRun 用的 ScheduleLocation 是同一个值。
	// 不指定的话 gocron 用 time.Local，而展示侧若换了时区就会出现"界面说 17:00、实际 9:00 触发"。
	schedulerInstance, err := gocron.NewScheduler(gocron.WithLocation(ScheduleLocation))
	if err != nil {
		return nil, fmt.Errorf("create gocron scheduler: %w", err)
	}
	return &Manager{scheduler: schedulerInstance, publisher: publisher}, nil
}

func (manager *Manager) Register(definition Definition) (gocron.Job, error) {
	return manager.scheduler.NewJob(
		gocron.CronJob(definition.CronExpression, false),
		gocron.NewTask(func(ctx context.Context) error {
			return manager.publisher.Publish(ctx, job.Message{
				SchemaVersion: job.SchemaVersion,
				ExecutionID:   uuid.NewString(),
				Kind:          definition.Kind,
				ResourceID:    definition.ID,
				TriggeredAt:   time.Now().UTC(),
			})
		}),
		gocron.WithName(definition.Name),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
}

func (manager *Manager) Start() {
	manager.scheduler.Start()
}

func (manager *Manager) Shutdown(ctx context.Context) error {
	return manager.scheduler.ShutdownWithContext(ctx)
}
