package protocol

import "time"

// TaskType 表示任务类型，便于一个 agent 承载多类任务。
type TaskType string

const (
	TaskTypeCommand    TaskType = "command"
	TaskTypeAutomation TaskType = "automation"
	TaskTypeInventory  TaskType = "inventory"
	TaskTypeCustom     TaskType = "custom"
)

// Job 描述一次可执行任务。
type Job struct {
	JobID string `json:"job_id"`
	// Type 为必填，必须显式声明任务类型。
	Type TaskType `json:"type"`
	// Action 用于标识具体动作，例如 collect_hosts / run_playbook / shell。
	Action string `json:"action"`

	// 命令型任务字段（Type=command/automation 时常用）。
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	WorkDir string            `json:"work_dir,omitempty"`
	Env     map[string]string `json:"env,omitempty"`

	// 扩展参数，支持非命令型任务（例如资产采集、配置下发、脚本编排）。
	Params map[string]any `json:"params,omitempty"`

	Timeout time.Duration `json:"timeout,omitempty"`
}

// JobStatus 统一任务生命周期状态。
type JobStatus string

const (
	StatusQueued   JobStatus = "queued"
	StatusRunning  JobStatus = "running"
	StatusSuccess  JobStatus = "success"
	StatusFailed   JobStatus = "failed"
	StatusCanceled JobStatus = "canceled"
	StatusTimeout  JobStatus = "timeout"
)

// JobResult 记录任务执行结果，便于上报与审计。
type JobResult struct {
	JobID  string   `json:"job_id"`
	Type   TaskType `json:"type"`
	Action string   `json:"action"`

	Status   JobStatus `json:"status"`
	ExitCode int       `json:"exit_code"`
	Stdout   string    `json:"stdout,omitempty"`
	Stderr   string    `json:"stderr,omitempty"`

	// Data 用于回传结构化结果，例如采集到的主机/指标/执行摘要。
	Data map[string]any `json:"data,omitempty"`

	StartedAt  time.Time `json:"started_at,omitempty"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
	CostMS     int64     `json:"cost_ms,omitempty"`
	Error      string    `json:"error,omitempty"`
}
