package scheduler

import (
	"net/http"

	"autoadmin/internal/shared/apperror"
)

var (
	ErrTaskNotFound       = apperror.New(apperror.CodeNotFound, "定时任务不存在")
	ErrLogNotFound        = apperror.New(apperror.CodeNotFound, "任务日志不存在")
	ErrCronRequired       = apperror.New(apperror.CodeInvalidArgument, "cron 表达式不能为空")
	ErrCronInvalid        = apperror.New(apperror.CodeInvalidArgument, "cron 表达式无效")
	ErrTaskDisabled       = apperror.New(apperror.CodeInvalidArgument, "Task is disabled")
	ErrTaskRunning        = apperror.New(apperror.CodeInvalidArgument, "Task is already running")
	ErrWorkerUnavailable  = apperror.New(apperror.CodeInvalidArgument, "Scheduler worker is not running")
	ErrHandlerUnsupported = apperror.New(apperror.CodeInvalidArgument, "任务 handler 尚未迁移到 Go")

	ErrTaskQueryInternal = apperror.NewWithHTTP(apperror.CodeInternal, "查询定时任务失败", http.StatusInternalServerError)
	ErrTaskSaveInternal  = apperror.NewWithHTTP(apperror.CodeInternal, "保存定时任务失败", http.StatusInternalServerError)
	ErrDispatchInternal  = apperror.NewWithHTTP(apperror.CodeInternal, "任务提交失败", http.StatusInternalServerError)
	ErrLogQueryInternal  = apperror.NewWithHTTP(apperror.CodeInternal, "查询任务日志失败", http.StatusInternalServerError)
)
