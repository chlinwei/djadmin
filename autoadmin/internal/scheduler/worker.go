package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"autoadmin/internal/job"
)

const scheduledTaskMessageKind = "scheduled_task"

var supportedTaskCodes = map[string]struct{}{
	"cleanup_login_audit_logs":     {},
	"cleanup_operation_audit_logs": {},
}

func IsSupportedTaskCode(code string) bool {
	_, supported := supportedTaskCodes[code]
	return supported
}

type Worker struct {
	repository *Repository
	handlers   map[string]func(context.Context) (string, error)
}

func NewWorker(repository *Repository) *Worker {
	worker := &Worker{repository: repository}
	worker.handlers = map[string]func(context.Context) (string, error){
		"cleanup_login_audit_logs":     worker.cleanupLoginAudits,
		"cleanup_operation_audit_logs": worker.cleanupOperationAudits,
	}
	return worker
}

func (worker *Worker) Handle(ctx context.Context, message job.Message) error {
	if message.Kind != scheduledTaskMessageKind || message.ResourceID < 1 {
		return fmt.Errorf("unsupported scheduler message kind %q", message.Kind)
	}
	startedAt := time.Now().UTC()
	claimed, err := worker.repository.Claim(ctx, message.ResourceID, startedAt)
	if err != nil || !claimed {
		return err
	}
	task, err := worker.repository.GetTask(ctx, message.ResourceID)
	if err != nil {
		return worker.repository.Complete(ctx, message.ResourceID, startedAt, "失败", "读取任务失败", "")
	}
	handler, supported := worker.handlers[task.Code]
	if !supported {
		return worker.repository.Complete(ctx, task.ID, startedAt, "失败", "任务 handler 尚未迁移到 Go", "")
	}
	output, executionErr := handler(ctx)
	status, resultMessage := "成功", "执行成功"
	if executionErr != nil {
		slog.Error("execute scheduled task", "task_id", task.ID, "task_code", task.Code, "error", executionErr)
		status, resultMessage = "失败", "执行失败"
	}
	return worker.repository.Complete(ctx, task.ID, startedAt, status, resultMessage, output)
}

func (worker *Worker) cleanupLoginAudits(ctx context.Context) (string, error) {
	days := worker.repository.RetentionDays(ctx, "sys.audit.login_logs.retention_days", 90)
	count, err := worker.repository.CleanupLoginAudits(ctx, time.Now().UTC().AddDate(0, 0, -days))
	return fmt.Sprintf("deleted %d login audit rows", count), err
}

func (worker *Worker) cleanupOperationAudits(ctx context.Context) (string, error) {
	days := worker.repository.RetentionDays(ctx, "sys.audit.operation_logs.retention_days", 90)
	count, err := worker.repository.CleanupOperationAudits(ctx, time.Now().UTC().AddDate(0, 0, -days))
	return fmt.Sprintf("deleted %d operation audit rows", count), err
}
