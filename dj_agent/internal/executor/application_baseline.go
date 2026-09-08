package executor

import (
	"context"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

type applicationCheckResult struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
	Message  string `json:"message,omitempty"`
}

func (e *Executor) checkApplicationBaseline(ctx context.Context, job protocol.Job) protocol.JobResult {
	started := time.Now()
	// 巡检中心模式：只执行 check_plan 下发的 OPA 策略检查项。
	// 早期的应用控制状态、端口、路径、日志内置检查，以及 goss / schema_validate
	// 执行器已全部下线，统一收敛到 opa。
	checks := make([]applicationCheckResult, 0)

	if ctx.Err() != nil {
		return canceledApplicationBaselineResult(job, started, ctx.Err())
	}
	checks = append(checks, e.checkApplicationPlanForState(ctx, job.Params, true)...)
	if ctx.Err() != nil {
		return canceledApplicationBaselineResult(job, started, ctx.Err())
	}

	passed := true
	for _, check := range checks {
		if check.Status != "pass" && check.Status != "skipped" {
			passed = false
			break
		}
	}

	finished := time.Now()
	return protocol.JobResult{
		JobID: job.JobID, Type: job.Type, Action: job.Action,
		Status: protocol.StatusSuccess, ExitCode: 0,
		StartedAt: started, FinishedAt: finished, CostMS: finished.Sub(started).Milliseconds(),
		Data: map[string]any{"passed": passed, "checks": checks},
	}
}

func canceledApplicationBaselineResult(job protocol.Job, started time.Time, err error) protocol.JobResult {
	return canceledJobResult(job, started, err, -1)
}

var applicationCheckCapabilities = map[string]struct{}{
	"opa:v1": {},
}

func (e *Executor) checkApplicationPlanForState(ctx context.Context, params map[string]any, applicationRunning bool) []applicationCheckResult {
	plan, ok := params["check_plan"].(map[string]any)
	if !ok {
		return nil
	}
	version, versionErr := numberToInt(plan["schema_version"])
	if versionErr != nil || version != 1 {
		return []applicationCheckResult{{Key: "check_plan", Type: "plan", Name: "应用检查计划", Status: "error", Message: "不支持的检查计划版本"}}
	}
	for _, capability := range stringSliceFromAny(anyToSlice(plan["required_capabilities"])) {
		if _, supported := applicationCheckCapabilities[capability]; !supported {
			return []applicationCheckResult{{Key: "check_plan", Type: "plan", Name: "应用检查计划", Status: "error", Actual: capability, Message: "dj-agent 不支持检查计划要求的能力"}}
		}
	}

	rawChecks, _ := plan["checks"].([]any)
	results := make([]applicationCheckResult, 0, len(rawChecks))
	for _, rawCheck := range rawChecks {
		check, valid := rawCheck.(map[string]any)
		if !valid {
			results = append(results, applicationCheckResult{Key: "invalid", Type: "plan", Name: "无效检查项", Status: "error", Message: "检查项必须是对象"})
			continue
		}
		if requiresRunning, _ := check["requires_running"].(bool); requiresRunning && !applicationRunning {
			results = append(results, newPlanCheckResult(check, "skipped", nil, "应用未运行，已跳过该检查项"))
			continue
		}
		// 唯一执行器：OPA 策略。check_plan 不再携带 executor 字段。
		results = append(results, e.checkOpa(ctx, check))
	}
	return results
}

func newPlanCheckResult(check map[string]any, status string, actual any, message string) applicationCheckResult {
	return applicationCheckResult{
		Key: valueString(check["key"]), Type: valueString(check["type"]), Name: valueString(check["name"]),
		Status: status, Expected: check["expected"], Actual: actual, Message: message,
	}
}

