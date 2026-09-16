package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	db "autoadmin/internal/platform/database/generated"
)

// Agent 安装/更新的作业与主机日志生命周期写入。
//
// install 与 update 两条流程用的是同一组语句（历史上这两份裸 SQL 是重复的），统一收敛到这里，
// 免得参数顺序在四处各写一遍。语句定义见 db/queries/mysql/assets.sql 的 agent 流程段。
//
// 时长（duration_seconds）由应用层算：原实现是 MySQL 的 TIMESTAMPDIFF(MICROSECOND,…)/1000000，
// PG 没有对应写法（分叉清单 SQL_DESIGN §4.2），所以先读回 start_time 再算。

func (handler *Handler) markAgentJobRunning(ctx context.Context, jobID string, logID int64, now time.Time) {
	queries := db.New(handler.service.repository.pool)
	_ = queries.MarkAgentJobRunning(ctx, db.MarkAgentJobRunningParams{
		PickedAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, JobID: jobID,
	})
	_ = queries.MarkAgentJobHostLogRunning(ctx, db.MarkAgentJobHostLogRunningParams{UpdateTime: now, ID: logID})
}

// failAgentJob 把作业与主机日志同时置为终态失败。jobStatus 允许是 'timeout'（作业行区分超时），
// 主机日志侧统一记 'failed'（与迁移前一致）。
func (handler *Handler) failAgentJob(ctx context.Context, host agentUpdateHost, jobStatus, message string, exitCode int64, stdout, stderr string) {
	now := time.Now().UTC()
	queries := db.New(handler.service.repository.pool)
	_ = queries.FailAgentJob(ctx, db.FailAgentJobParams{
		Status: jobStatus, ErrorMessage: message, ExitCode: int32(exitCode), Stdout: stdout, Stderr: stderr,
		FinishedAt: sql.NullTime{Time: now, Valid: true}, UpdateTime: now, JobID: host.AgentJobID,
	})
	_ = queries.FailAgentJobHostLog(ctx, db.FailAgentJobHostLogParams{
		Status: "failed", ErrorMessage: message, ExitCode: sql.NullInt32{Int32: int32(exitCode), Valid: true},
		Stdout: stdout, Stderr: stderr, UpdateTime: now, ID: host.LogID,
	})
}

func (handler *Handler) updateAgentJobStdout(ctx context.Context, jobID string, logID int64, stdout string, now time.Time) {
	queries := db.New(handler.service.repository.pool)
	_ = queries.UpdateAgentJobStdout(ctx, db.UpdateAgentJobStdoutParams{Stdout: stdout, UpdateTime: now, JobID: jobID})
	_ = queries.UpdateAgentJobHostLogStdout(ctx, db.UpdateAgentJobHostLogStdoutParams{Stdout: stdout, UpdateTime: now, ID: logID})
}

// finishAgentJob 收尾单台主机的作业。logResultData 传 nil 表示主机日志的 result_data 保持原值
// （更新流程不写它，与迁移前一致）。
func (handler *Handler) finishAgentJob(ctx context.Context, host agentUpdateHost, status string, exitCode int64, message string, resultData string, logResultData json.RawMessage, now time.Time) {
	queries := db.New(handler.service.repository.pool)
	_ = queries.FinishAgentJob(ctx, db.FinishAgentJobParams{
		Status: status, ExitCode: int32(exitCode), ErrorMessage: message,
		ResultData: json.RawMessage(resultData), FinishedAt: sql.NullTime{Time: now, Valid: true},
		UpdateTime: now, JobID: host.AgentJobID,
	})
	_ = queries.FinishAgentJobHostLog(ctx, db.FinishAgentJobHostLogParams{
		Status: status, ExitCode: sql.NullInt32{Int32: int32(exitCode), Valid: true}, ErrorMessage: message,
		ResultData: logResultData, UpdateTime: now, ID: host.LogID,
	})
}

func (handler *Handler) finishAgentExecutionJob(ctx context.Context, executionID int64, status, summary string, now time.Time) {
	queries := db.New(handler.service.repository.pool)
	duration := sql.NullFloat64{}
	if startTime, err := queries.GetAutomationJobStartTime(ctx, executionID); err == nil && startTime.Valid {
		duration = sql.NullFloat64{Float64: now.Sub(startTime.Time).Seconds(), Valid: true}
	}
	_ = queries.FinishAgentExecutionJob(ctx, db.FinishAgentExecutionJobParams{
		Status: status, EndTime: sql.NullTime{Time: now, Valid: true}, DurationSeconds: duration,
		ResultSummary: json.RawMessage(summary), UpdateTime: now, ID: executionID,
	})
}
