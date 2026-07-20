package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

const defaultTimeout = 30 * time.Second

// Executor 负责执行单个任务并采集结果。
type Executor struct {
	DefaultTimeout time.Duration
}

// New 创建一个新的 Executor 实例
func New(defaultJobTimeout time.Duration) *Executor {
	return &Executor{DefaultTimeout: defaultJobTimeout}
}

// Run 执行一个任务，返回结果或错误
func (e *Executor) Run(ctx context.Context, job protocol.Job) (protocol.JobResult, error) {
	// 校验必需字段
	if job.JobID == "" {
		return protocol.JobResult{}, fmt.Errorf("job_id is required")
	}
	if job.Type == "" {
		return protocol.JobResult{}, fmt.Errorf("type is required")
	}
	if job.Action == "" {
		return protocol.JobResult{}, fmt.Errorf("action is required")
	}

	// 按类型校验参数
	if err := validateJobByType(job); err != nil {
		return protocol.JobResult{}, err
	}

	// 确定实际超时时间
	timeout := e.resolveTimeout(job.Timeout)
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 尝试作为内置操作执行
	if result, handled := e.runBuiltinAction(runCtx, job); handled {
		return result, nil
	}

	// 作为普通命令执行
	cmd := exec.CommandContext(runCtx, job.Command, job.Args...)
	if job.WorkDir != "" {
		cmd.Dir = job.WorkDir
	}
	if len(job.Env) > 0 {
		cmd.Env = append(os.Environ(), mapToEnv(job.Env)...)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	started := time.Now()
	err := cmd.Run()
	finished := time.Now()

	result := protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		StartedAt:  started,
		FinishedAt: finished,
		CostMS:     finished.Sub(started).Milliseconds(),
		ExitCode:   readExitCode(err),
	}

	// 判断执行结果状态
	switch {
	case err == nil:
		result.Status = protocol.StatusSuccess
		return result, nil
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		result.Status = protocol.StatusTimeout
		result.Error = runCtx.Err().Error()
		return result, err
	case errors.Is(runCtx.Err(), context.Canceled):
		result.Status = protocol.StatusCanceled
		result.Error = runCtx.Err().Error()
		return result, err
	default:
		result.Status = protocol.StatusFailed
		result.Error = err.Error()
		return result, err
	}
}

// validateJobByType 根据任务类型校验所需参数
func validateJobByType(job protocol.Job) error {
	switch job.Type {
	case protocol.TaskTypeCommand, protocol.TaskTypeAutomation:
		if job.Command == "" {
			return fmt.Errorf("command is required for type=%s", job.Type)
		}
	case protocol.TaskTypeInventory, protocol.TaskTypeCustom:
		// 内置操作不需要 params，其他情况需要
		if job.Action != actionGetAgentVersion &&
			job.Action != actionGetHostInfo &&
			job.Action != actionInstallNodeExporter &&
			job.Action != actionUninstallNodeExporter &&
			len(job.Params) == 0 {
			return fmt.Errorf("params is required for type=%s", job.Type)
		}
	default:
		return fmt.Errorf("unsupported type: %s", job.Type)
	}

	return nil
}

// resolveTimeout 解析任务超时时间，按优先级返回：job超时 > executor默认 > 全局默认
func (e *Executor) resolveTimeout(jobTimeout time.Duration) time.Duration {
	if jobTimeout > 0 {
		return jobTimeout
	}
	if e.DefaultTimeout > 0 {
		return e.DefaultTimeout
	}
	return defaultTimeout
}
