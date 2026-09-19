package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"autoadmin/internal/job"
)

const scheduledTaskMessageKind = "scheduled_task"

// supportedTaskCodes 已迁到 Go 的任务编码 → 给用户看的名字。
//
// **这张表就是"哪些定时任务真的会执行"的唯一答案**：Django 后端移出后，库里那些历史任务
// （WebSSH 会话日志清理、执行日志清理、告警清理、Prometheus 历史告警对账…）没有任何实现，
// 定时调度会跳过它们、手动执行也必然失败。名字放在这里是为了让报错能说清"能做什么"
// （见 UnsupportedTaskNote），而不是只丢一句"尚未迁移"。
var supportedTaskCodes = map[string]string{
	"cleanup_login_audit_logs":          "登录日志清理",
	"cleanup_operation_audit_logs":      "操作日志清理",
	"cleanup_webssh_session_logs":       "WebSSH 会话日志清理",
	"cleanup_ansible_execution_logs":    "自动化执行日志清理",
	"cleanup_monitor_install_histories": "监控安装历史清理",
	"cleanup_alert_histories":           "历史告警清理",
}

func IsSupportedTaskCode(code string) bool {
	_, supported := supportedTaskCodes[code]
	return supported
}

// SupportedTaskNames 已实现的任务名（按编码排序，输出稳定，便于出现在提示文案与文档里）。
func SupportedTaskNames() []string {
	names := make([]string, 0, len(supportedTaskCodes))
	for _, name := range supportedTaskCodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// UnsupportedTaskNote 描述"这个任务为什么不可执行、现在能执行什么"。
//
// 现场（2026-09-19）只看到 `任务提交失败: 任务 handler 尚未迁移到 Go`：既不知道是谁的问题、
// 也不知道是不是要等它、更不知道有没有能用的替代，只能来问。把这三件事一次写清。
func UnsupportedTaskNote(name, code string) string {
	label := name
	if label == "" {
		label = code
	}
	return fmt.Sprintf(
		"定时任务「%s」(%s) 的实现尚未迁移到 Go（Django 后端已移出）：定时调度会跳过它，手动执行也只会失败。"+
			"当前已实现的任务：%s。",
		label, code, strings.Join(SupportedTaskNames(), "、"))
}

type Worker struct {
	repository *Repository
	handlers   map[string]func(context.Context) (string, error)
}

func NewWorker(repository *Repository) *Worker {
	worker := &Worker{repository: repository}
	worker.handlers = map[string]func(context.Context) (string, error){
		"cleanup_login_audit_logs":          worker.cleanupLoginAudits,
		"cleanup_operation_audit_logs":      worker.cleanupOperationAudits,
		"cleanup_webssh_session_logs":       worker.cleanupWebSSHSessionLogs,
		"cleanup_ansible_execution_logs":    worker.cleanupAutomationExecutionLogs,
		"cleanup_monitor_install_histories": worker.cleanupMonitorInstallHistories,
		"cleanup_alert_histories":           worker.cleanupAlertHistories,
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
		// 派发侧（app.go）已经跳过未实现的任务，正常到不了这里；真到了也要让日志行说得清是谁。
		return worker.repository.Complete(ctx, task.ID, startedAt, "失败", UnsupportedTaskNote(task.Name, task.Code), "")
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

// ---- 四类保留期清理（2026-09-19：从 Django 时代的历史任务迁到 Go）----
//
// 保留期一律读 sys_config 的**同名键**（与 Django 时代同一个键，升级后行为不变），
// 键不存在或读不出来时退回默认值。输出里带上"清了多少 + 保留多少天"：
// 执行日志是这些清理唯一的证据（删掉的数据没有别的痕迹）。

// cleanupWebSSHSessionLogs 清理 WebSSH 会话记录（sys.audit.webssh.retention_days，默认 30 天）。
//
// 按 start_time 判年龄：这条配置的说明是"会话完整记录在数据库中的保留天数"，
// 指记录本身；未正常结束（end_time 为空）的记录也要能清掉，否则崩溃残留永远删不掉。
// 会话内容占了这张表的大头（input_content / output_content 两个 longtext），
// 不清理会一直涨。
func (worker *Worker) cleanupWebSSHSessionLogs(ctx context.Context) (string, error) {
	days := worker.repository.RetentionDays(ctx, "sys.audit.webssh.retention_days", 30)
	count, err := worker.repository.DeleteWebSSHSessionLogsBefore(ctx, time.Now().UTC().AddDate(0, 0, -days))
	return fmt.Sprintf("deleted %d webssh session log rows (retention %d days)", count, days), err
}

// cleanupAutomationExecutionLogs 清理自动化执行记录（sys.automation.logs.retention_days，默认 30 天）。
//
// 清的是"作业 + 主机明细 + 字节块"三张表（子表先删，外键无级联），**只清已结束的作业**：
// 运行中的作业正在被运行记录中心读，删了就凭空消失。作业字节块本来在收尾时就会删
// （见 AUTOMATION_JOB_EXECUTION.md），这里兜住"执行进程没来得及清"的那部分。
func (worker *Worker) cleanupAutomationExecutionLogs(ctx context.Context) (string, error) {
	days := worker.repository.RetentionDays(ctx, "sys.automation.logs.retention_days", 30)
	hostLogs, chunkLogs, jobs, err := worker.repository.DeleteAutomationExecutionLogsBefore(ctx, time.Now().UTC().AddDate(0, 0, -days))
	return fmt.Sprintf("deleted %d automation jobs, %d host results, %d log chunks (retention %d days)",
		jobs, hostLogs, chunkLogs, days), err
}

// cleanupMonitorInstallHistories 清理监控安装/卸载历史（sys.monitor.install_history.retention_days，默认 180 天）。
//
// "每个纳管目标至少保留最新一条"写在配置说明里，SQL 里也照这个来（见 DeleteMonitorInstallHistoriesBefore）：
// 清完还要能回答"这台机器最近一次装/卸是什么结果"。
func (worker *Worker) cleanupMonitorInstallHistories(ctx context.Context) (string, error) {
	days := worker.repository.RetentionDays(ctx, "sys.monitor.install_history.retention_days", 180)
	count, err := worker.repository.DeleteMonitorInstallHistoriesBefore(ctx, time.Now().UTC().AddDate(0, 0, -days))
	return fmt.Sprintf("deleted %d monitor install history rows (retention %d days)", count, days), err
}

// cleanupAlertHistories 清理历史告警（sys.monitor.alert_history.retention_days，默认 90 天）。
//
// **只清已恢复的记录**（配置说明原话）：仍在 firing 的行是"当前状态"，删掉会让告警页凭空少一条
// 正在发的告警。年龄取恢复时间（resolved_at，历史数据缺失时回落到 started_at）。
// 一并清掉这些告警的通知事件与投递记录（它们引用告警行，外键无级联；保留期跟父记录走——
// 否则"有通知记录的告警"永远清不掉，这条配置就形同虚设。见 DeleteResolvedAlertHistoriesBefore）。
func (worker *Worker) cleanupAlertHistories(ctx context.Context) (string, error) {
	days := worker.repository.RetentionDays(ctx, "sys.monitor.alert_history.retention_days", 90)
	deliveries, events, histories, err := worker.repository.DeleteResolvedAlertHistoriesBefore(ctx, time.Now().UTC().AddDate(0, 0, -days))
	return fmt.Sprintf("deleted %d resolved alert rows, %d notification events, %d deliveries (retention %d days)",
		histories, events, deliveries, days), err
}
