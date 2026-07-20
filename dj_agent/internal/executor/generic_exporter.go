package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/chlinwei/djadmin/dj_agent/internal/protocol"
)

// 通用 exporter 生命周期执行器：安装/卸载已彻底交给 backend 的“自动化任务”
// （Ansible Playbook，见 automation.go 的 run_automation_task action）执行，
// dj-agent 不再解释/运行任何 manage_script，也不再需要针对脚本执行的 Landlock 沙箱。
// 这里只保留“日常启停/状态查询”这三个轻量动作，直接调用 systemd（sudo systemctl），
// 调用面收窄到固定的三条 systemctl 子命令 + 一个经过严格格式校验的服务名，
// 配合主机侧 sudoers 精确限定到 "systemctl start|stop|status <name>.service" 这一条命令，
// 攻击面比"任意 shell 脚本 + sudo"小得多。

// serviceNamePattern 严格限定服务名格式为 <合法字符>.service，避免异常输入被传给
// sudo systemctl 时匹配到非预期的系统单元，也便于主机侧 sudoers 精确到具体命令行做白名单限制。
var serviceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.@-]+\.service$`)

// resolveServiceName 从 job.Params 中取出并校验 systemd 服务名（形如 node_exporter.service，
// 由 backend 按 SoftwarePackage.service_unit_name 下发，等同 <exporter_name>.service）。
func resolveServiceName(params map[string]any) (string, error) {
	raw, ok := params["service_name"]
	if !ok || raw == nil {
		return "", fmt.Errorf("service_name is required")
	}
	name := strings.TrimSpace(fmt.Sprintf("%v", raw))
	if !serviceNamePattern.MatchString(name) {
		return "", fmt.Errorf("invalid service_name: %q (expected format like node_exporter.service)", name)
	}
	return name, nil
}

// runSystemctlAction 以 sudo -n（非交互：主机侧 sudoers 未免密时立即报错，而不是卡住等待密码）
// 执行 systemctl <action> <serviceName>，供 start/stop/status 三个动作复用。
func runSystemctlAction(ctx context.Context, job protocol.Job, action string) protocol.JobResult {
	serviceName, err := resolveServiceName(job.Params)
	if err != nil {
		return builtinFailedResult(job, err)
	}

	started := time.Now()
	cmd := exec.CommandContext(ctx, "sudo", "-n", "systemctl", action, serviceName)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
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
		ExitCode:   readExitCode(runErr),
	}

	switch {
	case runErr == nil:
		result.Status = protocol.StatusSuccess
	case ctx.Err() != nil && strings.Contains(ctx.Err().Error(), "deadline"):
		result.Status = protocol.StatusTimeout
		result.Error = ctx.Err().Error()
	case ctx.Err() != nil && strings.Contains(ctx.Err().Error(), "cancel"):
		result.Status = protocol.StatusCanceled
		result.Error = ctx.Err().Error()
	default:
		// systemctl status 在服务 inactive/failed 时也会以非零退出码返回，这里不做特殊豁免，
		// 统一标记为 failed，退出码/stdout/stderr 原样透传，由 backend 依据这些信息判断具体状态。
		result.Status = protocol.StatusFailed
		result.Error = runErr.Error()
	}

	return result
}

// startExporter 启动 exporter：sudo systemctl start <name>.service
func (e *Executor) startExporter(ctx context.Context, job protocol.Job) protocol.JobResult {
	return runSystemctlAction(ctx, job, "start")
}

// stopExporter 停止 exporter：sudo systemctl stop <name>.service
func (e *Executor) stopExporter(ctx context.Context, job protocol.Job) protocol.JobResult {
	return runSystemctlAction(ctx, job, "stop")
}

// checkExporterStatus 查询 exporter 状态：sudo systemctl status <name>.service
func (e *Executor) checkExporterStatus(ctx context.Context, job protocol.Job) protocol.JobResult {
	return runSystemctlAction(ctx, job, "status")
}

func builtinFailedResult(job protocol.Job, runErr error) protocol.JobResult {
	now := time.Now()
	return protocol.JobResult{
		JobID:      job.JobID,
		Type:       job.Type,
		Action:     job.Action,
		Status:     protocol.StatusFailed,
		Error:      runErr.Error(),
		StartedAt:  now,
		FinishedAt: now,
		ExitCode:   -1,
	}
}
